package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zachbroad/webhook-relay/internal/store"
)

type WebhookHandler struct {
	store *store.Store
	rdb   *redis.Client
}

func NewWebhookHandler(s *store.Store, rdb *redis.Client) *WebhookHandler {
	return &WebhookHandler{store: s, rdb: rdb}
}

func (h *WebhookHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	sourceSlug := chi.URLParam(r, "sourceSlug")

	src, err := h.store.Sources.GetBySlug(r.Context(), sourceSlug)
	if err != nil {
		http.Error(w, "source not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if !json.Valid(body) {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Extract relevant headers
	headerMap := map[string]string{}
	for _, key := range []string{"Content-Type", "X-Request-ID", "X-Webhook-ID"} {
		if v := r.Header.Get(key); v != "" {
			headerMap[key] = v
		}
	}
	headersJSON, _ := json.Marshal(headerMap)

	// Use X-Idempotency-Key header or generate one
	idempotencyKey := r.Header.Get("X-Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = uuid.New().String()
	}

	delivery, err := h.store.Deliveries.Create(r.Context(), src.ID, idempotencyKey, headersJSON, body)
	if err != nil {
		slog.Error("failed to create delivery", "error", err)
		http.Error(w, "failed to store delivery", http.StatusInternalServerError)
		return
	}

	// Publish to Redis Stream for fan-out
	if err := h.publishToStream(r.Context(), delivery.ID); err != nil {
		slog.Error("failed to publish to redis stream", "error", err, "delivery_id", delivery.ID)
		// Delivery is in Postgres with status=pending, catch-up poll will handle it
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"delivery_id": delivery.ID,
		"status":      delivery.Status,
	})
}

func (h *WebhookHandler) publishToStream(ctx context.Context, deliveryID uuid.UUID) error {
	return h.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "deliveries",
		Values: map[string]any{"delivery_id": deliveryID.String()},
	}).Err()
}
