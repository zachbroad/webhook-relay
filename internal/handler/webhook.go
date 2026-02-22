package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zachbroad/webhook-relay/internal/model"
	"github.com/zachbroad/webhook-relay/internal/store"
)

type WebhookHandler struct {
	store *store.Store
	rdb   *redis.Client
}

func NewWebhookHandler(s *store.Store, rdb *redis.Client) *WebhookHandler {
	return &WebhookHandler{store: s, rdb: rdb}
}

func (h *WebhookHandler) Ingest(c *gin.Context) {
	sourceSlug := c.Param("sourceSlug")

	src, err := h.store.Sources.GetBySlug(c.Request.Context(), sourceSlug)
	if err != nil {
		c.String(http.StatusNotFound, "source not found")
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, "failed to read body")
		return
	}

	if !json.Valid(body) {
		c.String(http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Extract relevant headers
	headerMap := map[string]string{}
	for _, key := range []string{"Content-Type", "X-Request-ID", "X-Webhook-ID"} {
		if v := c.GetHeader(key); v != "" {
			headerMap[key] = v
		}
	}
	headersJSON, _ := json.Marshal(headerMap)

	// Use X-Idempotency-Key header or generate one
	idempotencyKey := c.GetHeader("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = uuid.New().String()
	}

	delivery, err := h.store.Deliveries.Create(c.Request.Context(), src.ID, idempotencyKey, headersJSON, body)
	if err != nil {
		slog.Error("failed to create delivery", "error", err)
		c.String(http.StatusInternalServerError, "failed to store delivery")
		return
	}

	// Record mode: store only, no fanout
	if src.Mode == "record" {
		if err := h.store.Deliveries.UpdateStatus(c.Request.Context(), delivery.ID, model.DeliveryRecorded); err != nil {
			slog.Error("failed to update delivery status to recorded", "error", err, "delivery_id", delivery.ID)
		}
		c.JSON(http.StatusAccepted, gin.H{
			"delivery_id": delivery.ID,
			"status":      "recorded",
		})
		return
	}

	// Active mode: publish to Redis Stream for fan-out
	if err := h.publishToStream(c.Request.Context(), delivery.ID); err != nil {
		slog.Error("failed to publish to redis stream", "error", err, "delivery_id", delivery.ID)
		// Delivery is in Postgres with status=pending, catch-up poll will handle it
	}

	c.JSON(http.StatusAccepted, gin.H{
		"delivery_id": delivery.ID,
		"status":      delivery.Status,
	})
}

func (h *WebhookHandler) publishToStream(ctx context.Context, deliveryID uuid.UUID) error {
	return h.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "deliveries",
		MaxLen: 10000,
		Approx: true,
		Values: map[string]any{"delivery_id": deliveryID.String()},
	}).Err()
}
