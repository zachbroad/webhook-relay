package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/zachbroad/webhook-relay/internal/store"
)

type DeliveryHandler struct {
	store *store.Store
}

func NewDeliveryHandler(s *store.Store) *DeliveryHandler {
	return &DeliveryHandler{store: s}
}

func (h *DeliveryHandler) List(w http.ResponseWriter, r *http.Request) {
	var sourceSlug *string
	if s := r.URL.Query().Get("source"); s != "" {
		sourceSlug = &s
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	deliveries, err := h.store.Deliveries.List(r.Context(), sourceSlug, limit)
	if err != nil {
		http.Error(w, "failed to list deliveries", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if deliveries == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(deliveries)
}
