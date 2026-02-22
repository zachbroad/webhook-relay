package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

func (h *SubscriptionHandler) Create(c *gin.Context) {
	sourceSlug := c.Param("sourceSlug")

	src, err := h.store.Sources.GetBySlug(c.Request.Context(), sourceSlug)
	if err != nil {
		c.String(http.StatusNotFound, "source not found")
		return
	}

	var req createSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TargetURL == "" {
		c.String(http.StatusBadRequest, "target_url is required")
		return
	}

	sub, err := h.store.Subscriptions.Create(c.Request.Context(), src.ID, req.TargetURL, req.SigningSecret)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to create subscription")
		return
	}

	c.JSON(http.StatusCreated, sub)
}

func (h *SubscriptionHandler) List(c *gin.Context) {
	sourceSlug := c.Param("sourceSlug")

	src, err := h.store.Sources.GetBySlug(c.Request.Context(), sourceSlug)
	if err != nil {
		c.String(http.StatusNotFound, "source not found")
		return
	}

	subs, err := h.store.Subscriptions.List(c.Request.Context(), src.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	if subs == nil {
		c.Data(http.StatusOK, "application/json", []byte("[]"))
		return
	}
	c.JSON(http.StatusOK, subs)
}

func (h *SubscriptionHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid subscription id")
		return
	}

	sub, err := h.store.Subscriptions.GetByID(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusNotFound, "subscription not found")
		return
	}

	c.JSON(http.StatusOK, sub)
}

func (h *SubscriptionHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid subscription id")
		return
	}

	var req updateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid request body")
		return
	}

	sub, err := h.store.Subscriptions.Update(c.Request.Context(), id, req.TargetURL, req.SigningSecret, req.IsActive)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to update subscription")
		return
	}

	c.JSON(http.StatusOK, sub)
}

func (h *SubscriptionHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid subscription id")
		return
	}

	if err := h.store.Subscriptions.Delete(c.Request.Context(), id); err != nil {
		c.String(http.StatusInternalServerError, "failed to delete subscription")
		return
	}

	c.Status(http.StatusNoContent)
}
