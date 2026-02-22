package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zachbroad/webhook-relay/internal/model"
	"github.com/zachbroad/webhook-relay/internal/script"
	"github.com/zachbroad/webhook-relay/internal/signing"
	"github.com/zachbroad/webhook-relay/internal/store"
)

const (
	streamName    = "deliveries"
	consumerGroup = "fanout-workers"
	maxBodyLen    = 4096
)

type FanoutWorker struct {
	store          *store.Store
	rdb            *redis.Client
	httpClient     *http.Client
	concurrency    int
	maxRetries     int
	retryBaseDelay time.Duration
	pollInterval   time.Duration
}

func New(s *store.Store, rdb *redis.Client, concurrency, maxRetries int, retryBaseDelay, deliveryTimeout, pollInterval time.Duration) *FanoutWorker {
	return &FanoutWorker{
		store:          s,
		rdb:            rdb,
		httpClient:     &http.Client{Timeout: deliveryTimeout},
		concurrency:    concurrency,
		maxRetries:     maxRetries,
		retryBaseDelay: retryBaseDelay,
		pollInterval:   pollInterval,
	}
}

func (w *FanoutWorker) Start(ctx context.Context) error {
	// Ensure consumer group exists
	err := w.rdb.XGroupCreateMkStream(ctx, streamName, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("create consumer group: %w", err)
	}

	// Start stream consumers
	for i := range w.concurrency {
		consumer := fmt.Sprintf("worker-%d", i)
		go w.consumeStream(ctx, consumer)
	}

	// Start catch-up poll for pending deliveries
	go w.pollPending(ctx)

	// Start retry poll
	go w.pollRetries(ctx)

	return nil
}

func (w *FanoutWorker) consumeStream(ctx context.Context, consumer string) {
	for {
		if ctx.Err() != nil {
			return
		}

		streams, err := w.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumer,
			Streams:  []string{streamName, ">"},
			Count:    1,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				continue
			}
			slog.Error("xreadgroup error", "error", err, "consumer", consumer)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				deliveryIDStr, ok := msg.Values["delivery_id"].(string)
				if !ok {
					slog.Error("invalid delivery_id in stream message", "msg_id", msg.ID)
					w.rdb.XAck(ctx, streamName, consumerGroup, msg.ID)
					continue
				}

				deliveryID, err := uuid.Parse(deliveryIDStr)
				if err != nil {
					slog.Error("failed to parse delivery_id", "error", err, "value", deliveryIDStr)
					w.rdb.XAck(ctx, streamName, consumerGroup, msg.ID)
					continue
				}

				w.processDelivery(ctx, deliveryID)
				w.rdb.XAck(ctx, streamName, consumerGroup, msg.ID)
				w.rdb.XDel(ctx, streamName, msg.ID)
			}
		}
	}
}

func (w *FanoutWorker) processDelivery(ctx context.Context, deliveryID uuid.UUID) {
	delivery, err := w.store.Deliveries.GetByID(ctx, deliveryID)
	if err != nil {
		slog.Error("failed to get delivery", "error", err, "delivery_id", deliveryID)
		return
	}

	if delivery.Status != model.DeliveryPending {
		return
	}

	// Fetch the source to check mode and get script
	src, err := w.store.Sources.GetByID(ctx, delivery.SourceID)
	if err != nil {
		slog.Error("failed to get source for delivery", "error", err, "delivery_id", deliveryID)
		return
	}

	// Guard against race: if source switched to record mode after webhook was accepted
	if src.Mode == "record" {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryRecorded)
		return
	}

	if err := w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryProcessing); err != nil {
		slog.Error("failed to update delivery status", "error", err, "delivery_id", deliveryID)
		return
	}

	subs, err := w.store.Subscriptions.ListActiveBySource(ctx, delivery.SourceID)
	if err != nil {
		slog.Error("failed to list subscriptions", "error", err, "delivery_id", deliveryID)
		return
	}

	if len(subs) == 0 {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
		return
	}

	// Determine payload and headers to use for dispatch
	payload := delivery.Payload
	headers := delivery.Headers
	activeSubs := subs

	// Run transform script if source has one
	if src.ScriptBody != nil && *src.ScriptBody != "" {
		transformResult, err := w.runTransform(*src.ScriptBody, delivery, subs)
		if err != nil {
			slog.Error("script execution failed", "error", err, "delivery_id", deliveryID)
			w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryFailed)
			return
		}

		if transformResult.Dropped {
			slog.Info("script dropped delivery", "delivery_id", deliveryID)
			w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
			return
		}

		// Marshal transformed data
		transformedPayload, err := json.Marshal(transformResult.Payload)
		if err != nil {
			slog.Error("failed to marshal transformed payload", "error", err, "delivery_id", deliveryID)
			w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryFailed)
			return
		}
		transformedHeaders, err := json.Marshal(transformResult.Headers)
		if err != nil {
			slog.Error("failed to marshal transformed headers", "error", err, "delivery_id", deliveryID)
			w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryFailed)
			return
		}

		// Persist transformed data for retries
		if err := w.store.Deliveries.SetTransformed(ctx, deliveryID, transformedPayload, transformedHeaders); err != nil {
			slog.Error("failed to persist transformed data", "error", err, "delivery_id", deliveryID)
		}

		payload = transformedPayload
		headers = transformedHeaders

		// Filter subscriptions to only those the script kept
		if len(transformResult.Subscriptions) > 0 {
			activeSubs = filterSubscriptions(subs, transformResult.Subscriptions)
		} else {
			// Script filtered all subscriptions out
			w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
			return
		}
	}

	if len(activeSubs) == 0 {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
		return
	}

	allSuccess := true
	for _, sub := range activeSubs {
		success := w.dispatchToSubscriptionWithPayload(ctx, delivery, &sub, 1, payload, headers)
		if !success {
			allSuccess = false
		}
	}

	if allSuccess {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
	}
}

// runTransform executes the source's JS transform script against the delivery.
func (w *FanoutWorker) runTransform(scriptBody string, delivery *model.Delivery, subs []model.Subscription) (*script.TransformResult, error) {
	// Parse payload into a map
	var payloadMap map[string]any
	if err := json.Unmarshal(delivery.Payload, &payloadMap); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	// Parse headers into a map
	var headersMap map[string]string
	if err := json.Unmarshal(delivery.Headers, &headersMap); err != nil {
		return nil, fmt.Errorf("unmarshal headers: %w", err)
	}

	// Build subscription refs
	subRefs := make([]script.SubscriptionRef, len(subs))
	for i, s := range subs {
		subRefs[i] = script.SubscriptionRef{ID: s.ID, TargetURL: s.TargetURL}
	}

	input := script.TransformInput{
		Payload:       payloadMap,
		Headers:       headersMap,
		Subscriptions: subRefs,
	}

	return script.Run(scriptBody, input)
}

// filterSubscriptions returns only the subscriptions whose IDs appear in the script result.
func filterSubscriptions(all []model.Subscription, kept []script.SubscriptionRef) []model.Subscription {
	keptIDs := make(map[uuid.UUID]bool, len(kept))
	for _, s := range kept {
		keptIDs[s.ID] = true
	}

	var filtered []model.Subscription
	for _, s := range all {
		if keptIDs[s.ID] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func (w *FanoutWorker) dispatchToSubscription(ctx context.Context, delivery *model.Delivery, sub *model.Subscription, attemptNumber int) bool {
	// Use transformed payload/headers if available, otherwise originals
	payload := delivery.Payload
	headers := delivery.Headers
	if delivery.TransformedPayload != nil {
		payload = delivery.TransformedPayload
	}
	if delivery.TransformedHeaders != nil {
		headers = delivery.TransformedHeaders
	}
	return w.dispatchToSubscriptionWithPayload(ctx, delivery, sub, attemptNumber, payload, headers)
}

func (w *FanoutWorker) dispatchToSubscriptionWithPayload(ctx context.Context, delivery *model.Delivery, sub *model.Subscription, attemptNumber int, payload, headers json.RawMessage) bool {
	attempt, err := w.store.Deliveries.CreateAttempt(ctx, delivery.ID, sub.ID, attemptNumber)
	if err != nil {
		slog.Error("failed to create attempt", "error", err)
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.TargetURL, bytes.NewReader(payload))
	if err != nil {
		errMsg := err.Error()
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, nil, nil, &errMsg, nil)
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Delivery-ID", delivery.ID.String())

	// Apply any headers from the (potentially transformed) headers JSON
	var headerMap map[string]string
	if err := json.Unmarshal(headers, &headerMap); err == nil {
		for k, v := range headerMap {
			if k != "Content-Type" { // Don't override Content-Type
				req.Header.Set(k, v)
			}
		}
	}

	// Signing uses the payload that the subscriber actually receives
	if sub.SigningSecret != nil {
		sig := signing.Sign(payload, *sub.SigningSecret)
		req.Header.Set("X-Webhook-Signature-256", sig)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		errMsg := err.Error()
		nextRetry := w.nextRetryTime(attemptNumber)
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, nil, nil, &errMsg, nextRetry)
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyLen))
	bodyStr := string(body)
	statusCode := resp.StatusCode

	if statusCode >= 200 && statusCode < 300 {
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptSuccess, &statusCode, &bodyStr, nil, nil)
		return true
	}

	errMsg := fmt.Sprintf("HTTP %d", statusCode)
	nextRetry := w.nextRetryTime(attemptNumber)
	w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, &statusCode, &bodyStr, &errMsg, nextRetry)
	return false
}

func (w *FanoutWorker) nextRetryTime(attemptNumber int) *time.Time {
	if attemptNumber >= w.maxRetries {
		return nil // exhausted retries
	}
	delay := w.retryBaseDelay * time.Duration(math.Pow(2, float64(attemptNumber-1)))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	// Add jitter: +-25%
	jitter := time.Duration(float64(delay) * (0.75 + rand.Float64()*0.5))
	t := time.Now().Add(jitter)
	return &t
}

func (w *FanoutWorker) pollPending(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deliveries, err := w.store.Deliveries.ListPending(ctx, 100)
			if err != nil {
				slog.Error("poll pending error", "error", err)
				continue
			}
			for _, d := range deliveries {
				slog.Info("catch-up: processing pending delivery", "delivery_id", d.ID)
				w.processDelivery(ctx, d.ID)
			}
		}
	}
}

func (w *FanoutWorker) pollRetries(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			attempts, err := w.store.Deliveries.ListRetryableAttempts(ctx, 100)
			if err != nil {
				slog.Error("poll retries error", "error", err)
				continue
			}
			for _, a := range attempts {
				w.retryAttempt(ctx, &a)
			}
		}
	}
}

func (w *FanoutWorker) retryAttempt(ctx context.Context, prev *model.DeliveryAttempt) {
	delivery, err := w.store.Deliveries.GetByID(ctx, prev.DeliveryID)
	if err != nil {
		slog.Error("retry: failed to get delivery", "error", err)
		return
	}

	sub, err := w.store.Subscriptions.GetByID(ctx, prev.SubscriptionID)
	if err != nil {
		slog.Error("retry: failed to get subscription", "error", err)
		return
	}

	nextAttempt := prev.AttemptNumber + 1
	success := w.dispatchToSubscription(ctx, delivery, sub, nextAttempt)

	// Clear the retry marker on the old attempt so it's not picked up again
	w.store.Deliveries.UpdateAttempt(ctx, prev.ID, model.AttemptFailed, prev.ResponseStatus, prev.ResponseBody, prev.ErrorMessage, nil)

	// Roll up delivery status if this was the last subscription or all succeeded
	if success {
		w.rollUpDeliveryStatus(ctx, delivery.ID)
	} else if nextAttempt >= w.maxRetries {
		w.store.Deliveries.UpdateStatus(ctx, delivery.ID, model.DeliveryFailed)
	}
}

func (w *FanoutWorker) rollUpDeliveryStatus(ctx context.Context, deliveryID uuid.UUID) {
	delivery, err := w.store.Deliveries.GetByID(ctx, deliveryID)
	if err != nil {
		return
	}

	subs, err := w.store.Subscriptions.ListActiveBySource(ctx, delivery.SourceID)
	if err != nil {
		return
	}

	allDone := true
	for _, sub := range subs {
		maxAttempt, err := w.store.Deliveries.GetMaxAttemptNumber(ctx, deliveryID, sub.ID)
		if err != nil || maxAttempt == 0 {
			allDone = false
			continue
		}
		_ = maxAttempt
	}

	if allDone {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
	}
}
