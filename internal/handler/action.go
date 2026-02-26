package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zachbroad/webhook-relay/internal/model"
	"github.com/zachbroad/webhook-relay/internal/script"
	"github.com/zachbroad/webhook-relay/internal/store"
)

type ActionHandler struct {
	store *store.Store
}

func NewActionHandler(s *store.Store) *ActionHandler {
	return &ActionHandler{store: s}
}

type createActionRequest struct {
	Type          string  `json:"type"`
	TargetURL     *string `json:"target_url,omitempty"`
	SigningSecret *string `json:"signing_secret,omitempty"`
	ScriptBody    *string `json:"script_body,omitempty"`
}

type updateActionRequest struct {
	TargetURL     *string `json:"target_url,omitempty"`
	SigningSecret *string `json:"signing_secret,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

func (h *ActionHandler) Create(c *gin.Context) {
	sourceSlug := c.Param("sourceSlug")

	src, err := h.store.Sources.GetBySlug(c.Request.Context(), sourceSlug)
	if err != nil {
		c.String(http.StatusNotFound, "source not found")
		return
	}

	var req createActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid request body")
		return
	}

	actionType := model.ActionType(req.Type)
	if actionType == "" {
		actionType = model.ActionTypeWebhook
	}

	switch actionType {
	case model.ActionTypeWebhook:
		if req.TargetURL == nil || *req.TargetURL == "" {
			c.String(http.StatusBadRequest, "target_url is required for webhook actions")
			return
		}
	case model.ActionTypeJavascript:
		if req.ScriptBody == nil || *req.ScriptBody == "" {
			c.String(http.StatusBadRequest, "script_body is required for javascript actions")
			return
		}
		if err := script.ValidateAction(*req.ScriptBody); err != nil {
			c.String(http.StatusBadRequest, "invalid script: %s", err.Error())
			return
		}
	default:
		c.String(http.StatusBadRequest, "invalid action type: must be 'webhook' or 'javascript'")
		return
	}

	action, err := h.store.Actions.Create(c.Request.Context(), src.ID, actionType, req.TargetURL, req.SigningSecret, req.ScriptBody)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to create action")
		return
	}

	c.JSON(http.StatusCreated, action)
}

func (h *ActionHandler) List(c *gin.Context) {
	sourceSlug := c.Param("sourceSlug")

	src, err := h.store.Sources.GetBySlug(c.Request.Context(), sourceSlug)
	if err != nil {
		c.String(http.StatusNotFound, "source not found")
		return
	}

	actions, err := h.store.Actions.List(c.Request.Context(), src.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list actions")
		return
	}
	if actions == nil {
		c.Data(http.StatusOK, "application/json", []byte("[]"))
		return
	}
	c.JSON(http.StatusOK, actions)
}

func (h *ActionHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid action id")
		return
	}

	action, err := h.store.Actions.GetByID(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusNotFound, "action not found")
		return
	}

	c.JSON(http.StatusOK, action)
}

func (h *ActionHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid action id")
		return
	}

	var req updateActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "invalid request body")
		return
	}

	action, err := h.store.Actions.Update(c.Request.Context(), id, req.TargetURL, req.SigningSecret, req.IsActive, nil)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to update action")
		return
	}

	c.JSON(http.StatusOK, action)
}

func (h *ActionHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid action id")
		return
	}

	if err := h.store.Actions.Delete(c.Request.Context(), id); err != nil {
		c.String(http.StatusInternalServerError, "failed to delete action")
		return
	}

	c.Status(http.StatusNoContent)
}
