package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zachbroad/webhook-relay/internal/store"
)

type SubscriptionHandler struct {
	store *store.Store
}

func NewSubscriptionHandler(s *store.Store) *SubscriptionHandler {
	return &SubscriptionHandler{store: s}
}

type createSubscriptionRequest struct {
	TargetURL     string  `json:"target_url"`
	SigningSecret *string `json:"signing_secret,omitempty"`
}

type updateSubscriptionRequest struct {
	TargetURL     *string `json:"target_url,omitempty"`
	SigningSecret *string `json:"signing_secret,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

func (h *SubscriptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	sourceSlug := chi.URLParam(r, "sourceSlug")

	src, err := h.store.Sources.GetBySlug(r.Context(), sourceSlug)
	if err != nil {
		http.Error(w, "source not found", http.StatusNotFound)
		return
	}

	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.TargetURL == "" {
		http.Error(w, "target_url is required", http.StatusBadRequest)
		return
	}

	sub, err := h.store.Subscriptions.Create(r.Context(), src.ID, req.TargetURL, req.SigningSecret)
	if err != nil {
		http.Error(w, "failed to create subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sub)
}

func (h *SubscriptionHandler) List(w http.ResponseWriter, r *http.Request) {
	sourceSlug := chi.URLParam(r, "sourceSlug")

	src, err := h.store.Sources.GetBySlug(r.Context(), sourceSlug)
	if err != nil {
		http.Error(w, "source not found", http.StatusNotFound)
		return
	}

	subs, err := h.store.Subscriptions.List(r.Context(), src.ID)
	if err != nil {
		http.Error(w, "failed to list subscriptions", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if subs == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(subs)
}

func (h *SubscriptionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid subscription id", http.StatusBadRequest)
		return
	}

	sub, err := h.store.Subscriptions.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

func (h *SubscriptionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid subscription id", http.StatusBadRequest)
		return
	}

	var req updateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	sub, err := h.store.Subscriptions.Update(r.Context(), id, req.TargetURL, req.SigningSecret, req.IsActive)
	if err != nil {
		http.Error(w, "failed to update subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

func (h *SubscriptionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid subscription id", http.StatusBadRequest)
		return
	}

	if err := h.store.Subscriptions.Delete(r.Context(), id); err != nil {
		http.Error(w, "failed to delete subscription", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
