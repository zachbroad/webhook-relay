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

	actions, err := w.store.Actions.ListActiveBySource(ctx, delivery.SourceID)
	if err != nil {
		slog.Error("failed to list actions", "error", err, "delivery_id", deliveryID)
		return
	}

	if len(actions) == 0 {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
		return
	}

	// Determine payload and headers to use for dispatch
	payload := delivery.Payload
	headers := delivery.Headers
	activeActions := actions

	// Run transform script if source has one
	if src.ScriptBody != nil && *src.ScriptBody != "" {
		transformResult, err := w.runTransform(*src.ScriptBody, delivery, actions)
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

		// Filter actions to only those the script kept
		if len(transformResult.Actions) > 0 {
			activeActions = filterActions(actions, transformResult.Actions)
		} else {
			// Script filtered all actions out
			w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
			return
		}
	}

	if len(activeActions) == 0 {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
		return
	}

	allSuccess := true
	for _, action := range activeActions {
		var success bool
		switch action.Type {
		case model.ActionTypeJavascript:
			success = w.dispatchJavascriptAction(ctx, delivery, &action, 1, payload, headers)
		default:
			success = w.dispatchWebhookAction(ctx, delivery, &action, 1, payload, headers)
		}
		if !success {
			allSuccess = false
		}
	}

	if allSuccess {
		w.store.Deliveries.UpdateStatus(ctx, deliveryID, model.DeliveryCompleted)
	}
}

// runTransform executes the source's JS transform script against the delivery.
func (w *FanoutWorker) runTransform(scriptBody string, delivery *model.Delivery, actions []model.Action) (*script.TransformResult, error) {
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

	// Build action refs
	actionRefs := make([]script.ActionRef, len(actions))
	for i, a := range actions {
		targetURL := ""
		if a.TargetURL != nil {
			targetURL = *a.TargetURL
		}
		actionRefs[i] = script.ActionRef{ID: a.ID, TargetURL: targetURL}
	}

	input := script.TransformInput{
		Payload: payloadMap,
		Headers: headersMap,
		Actions: actionRefs,
	}

	return script.Run(scriptBody, input)
}

// filterActions returns only the actions whose IDs appear in the script result.
func filterActions(all []model.Action, kept []script.ActionRef) []model.Action {
	keptIDs := make(map[uuid.UUID]bool, len(kept))
	for _, a := range kept {
		keptIDs[a.ID] = true
	}

	var filtered []model.Action
	for _, a := range all {
		if keptIDs[a.ID] {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func (w *FanoutWorker) dispatchToAction(ctx context.Context, delivery *model.Delivery, action *model.Action, attemptNumber int) bool {
	// Use transformed payload/headers if available, otherwise originals
	payload := delivery.Payload
	headers := delivery.Headers
	if delivery.TransformedPayload != nil {
		payload = delivery.TransformedPayload
	}
	if delivery.TransformedHeaders != nil {
		headers = delivery.TransformedHeaders
	}
	switch action.Type {
	case model.ActionTypeJavascript:
		return w.dispatchJavascriptAction(ctx, delivery, action, attemptNumber, payload, headers)
	default:
		return w.dispatchWebhookAction(ctx, delivery, action, attemptNumber, payload, headers)
	}
}

func (w *FanoutWorker) dispatchWebhookAction(ctx context.Context, delivery *model.Delivery, action *model.Action, attemptNumber int, payload, headers json.RawMessage) bool {
	attempt, err := w.store.Deliveries.CreateAttempt(ctx, delivery.ID, action.ID, attemptNumber)
	if err != nil {
		slog.Error("failed to create attempt", "error", err)
		return false
	}

	targetURL := ""
	if action.TargetURL != nil {
		targetURL = *action.TargetURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
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
	if action.SigningSecret != nil {
		sig := signing.Sign(payload, *action.SigningSecret)
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

func (w *FanoutWorker) dispatchJavascriptAction(ctx context.Context, delivery *model.Delivery, action *model.Action, attemptNumber int, payload, headers json.RawMessage) bool {
	attempt, err := w.store.Deliveries.CreateAttempt(ctx, delivery.ID, action.ID, attemptNumber)
	if err != nil {
		slog.Error("failed to create attempt", "error", err)
		return false
	}

	if action.ScriptBody == nil || *action.ScriptBody == "" {
		errMsg := "javascript action has no script_body"
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, nil, nil, &errMsg, nil)
		return false
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		errMsg := fmt.Sprintf("failed to unmarshal payload: %v", err)
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, nil, nil, &errMsg, nil)
		return false
	}

	var headersMap map[string]string
	if err := json.Unmarshal(headers, &headersMap); err != nil {
		errMsg := fmt.Sprintf("failed to unmarshal headers: %v", err)
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, nil, nil, &errMsg, nil)
		return false
	}

	result, err := script.RunAction(*action.ScriptBody, payloadMap, headersMap)
	if err != nil {
		errMsg := err.Error()
		nextRetry := w.nextRetryTime(attemptNumber)
		w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptFailed, nil, nil, &errMsg, nextRetry)
		return false
	}

	w.store.Deliveries.UpdateAttempt(ctx, attempt.ID, model.AttemptSuccess, nil, &result, nil, nil)
	return true
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

	action, err := w.store.Actions.GetByID(ctx, prev.ActionID)
	if err != nil {
		slog.Error("retry: failed to get action", "error", err)
		return
	}

	nextAttempt := prev.AttemptNumber + 1
	success := w.dispatchToAction(ctx, delivery, action, nextAttempt)

	// Clear the retry marker on the old attempt so it's not picked up again
	w.store.Deliveries.UpdateAttempt(ctx, prev.ID, model.AttemptFailed, prev.ResponseStatus, prev.ResponseBody, prev.ErrorMessage, nil)

	// Roll up delivery status if this was the last action or all succeeded
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

	actions, err := w.store.Actions.ListActiveBySource(ctx, delivery.SourceID)
	if err != nil {
		return
	}

	allDone := true
	for _, action := range actions {
		maxAttempt, err := w.store.Deliveries.GetMaxAttemptNumber(ctx, deliveryID, action.ID)
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
