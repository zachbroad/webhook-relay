package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zachbroad/webhook-relay/internal/store"
)

type DeliveryHandler struct {
	store *store.Store
}

func NewDeliveryHandler(s *store.Store) *DeliveryHandler {
	return &DeliveryHandler{store: s}
}

func (h *DeliveryHandler) List(c *gin.Context) {
	var sourceSlug *string
	if s := c.Query("source"); s != "" {
		sourceSlug = &s
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	deliveries, err := h.store.Deliveries.List(c.Request.Context(), sourceSlug, limit)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list deliveries")
		return
	}

	if deliveries == nil {
		c.Data(http.StatusOK, "application/json", []byte("[]"))
		return
	}
	c.JSON(http.StatusOK, deliveries)
}

func (h *DeliveryHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid delivery id")
		return
	}

	delivery, err := h.store.Deliveries.GetByID(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusNotFound, "delivery not found")
		return
	}

	c.JSON(http.StatusOK, delivery)
}

func (h *DeliveryHandler) ListAttempts(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid delivery id")
		return
	}

	// Verify delivery exists
	if _, err := h.store.Deliveries.GetByID(c.Request.Context(), id); err != nil {
		c.String(http.StatusNotFound, "delivery not found")
		return
	}

	attempts, err := h.store.Deliveries.ListAttemptsByDelivery(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list attempts")
		return
	}

	if attempts == nil {
		c.Data(http.StatusOK, "application/json", []byte("[]"))
		return
	}
	c.JSON(http.StatusOK, attempts)
}
