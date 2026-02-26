package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zachbroad/webhook-relay/internal/model"
	"github.com/zachbroad/webhook-relay/internal/script"
	"github.com/zachbroad/webhook-relay/internal/store"
)

var funcMap = template.FuncMap{
	"shortID": func(id uuid.UUID) string {
		s := id.String()
		if len(s) >= 8 {
			return s[:8]
		}
		return s
	},
	"formatTime": func(t time.Time) string {
		return t.Format("Jan 2, 2006 3:04 PM")
	},
	"formatJSON": func(data json.RawMessage) template.HTML {
		if data == nil {
			return "-"
		}
		var out bytes.Buffer
		if err := json.Indent(&out, data, "", "  "); err != nil {
			return template.HTML(template.HTMLEscapeString(string(data)))
		}
		return template.HTML(template.HTMLEscapeString(out.String()))
	},
	"derefInt": func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	},
	"derefStr": func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	},
	"marshalJSON": func(v any) template.HTML {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return template.HTML(template.HTMLEscapeString(fmt.Sprintf("%v", v)))
		}
		return template.HTML(template.HTMLEscapeString(string(b)))
	},
	"truncateJSON": func(data json.RawMessage, maxLen int) string {
		if data == nil {
			return "(empty)"
		}
		var out bytes.Buffer
		if err := json.Compact(&out, data); err != nil {
			s := string(data)
			if len(s) > maxLen {
				return s[:maxLen] + "…"
			}
			return s
		}
		s := out.String()
		if len(s) > maxLen {
			return s[:maxLen] + "…"
		}
		return s
	},
}

type Handler struct {
	store     *store.Store
	templates map[string]*template.Template
}

func NewHandler(s *store.Store) *Handler {
	h := &Handler{
		store:     s,
		templates: make(map[string]*template.Template),
	}
	for _, page := range []string{"sources", "source", "deliveries", "delivery"} {
		h.templates[page] = template.Must(
			template.New("").Funcs(funcMap).ParseFS(templateFS,
				"templates/layout.html",
				"templates/"+page+".html",
			),
		)
	}
	return h
}

func (h *Handler) render(c *gin.Context, page string, data any) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.templates[page].ExecuteTemplate(c.Writer, "layout", data); err != nil {
		slog.Error("template render error", "page", page, "error", err)
	}
}

func (h *Handler) renderFragment(c *gin.Context, page string, fragment string, data any) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.templates[page].ExecuteTemplate(c.Writer, fragment, data); err != nil {
		slog.Error("template render error", "page", page, "fragment", fragment, "error", err)
	}
}

// Page data types

type sourcesData struct {
	Nav     string
	Sources []model.Source
	Error   string
}

type sourceData struct {
	Nav           string
	Source        *model.Source
	Actions       []model.Action
	Deliveries    []model.Delivery
	WebhookURL    string
	Error         string
	ScriptError   string
	ScriptSuccess string
	EditAction    *model.Action
	ActionError   string
	ActionSuccess string
}

type scriptTestData struct {
	Result *script.TransformResult
	Error  string
}

type deliveriesData struct {
	Nav          string
	Sources      []model.Source
	Deliveries   []model.Delivery
	SourceFilter string
}

type deliveryData struct {
	Nav      string
	Delivery *model.Delivery
	Attempts []model.DeliveryAttempt
}

// Page handlers

func (h *Handler) Sources(c *gin.Context) {
	sources, err := h.store.Sources.List(c.Request.Context())
	if err != nil {
		slog.Error("failed to list sources", "error", err)
		c.String(http.StatusInternalServerError, "Internal server error")
		return
	}
	h.render(c, "sources", sourcesData{
		Nav:     "sources",
		Sources: sources,
	})
}

func (h *Handler) SourceDetail(c *gin.Context) {
	slug := c.Param("slug")
	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}
	actions, err := h.store.Actions.List(c.Request.Context(), source.ID)
	if err != nil {
		slog.Error("failed to list actions", "error", err)
		c.String(http.StatusInternalServerError, "Internal server error")
		return
	}
	deliveries, _ := h.store.Deliveries.List(c.Request.Context(), &slug, 10)
	h.render(c, "source", sourceData{
		Nav:        "sources",
		Source:     source,
		Actions:    actions,
		Deliveries: deliveries,
		WebhookURL: webhookURL(c, source.Slug),
	})
}

func (h *Handler) Deliveries(c *gin.Context) {
	sources, err := h.store.Sources.List(c.Request.Context())
	if err != nil {
		slog.Error("failed to list sources", "error", err)
		c.String(http.StatusInternalServerError, "Internal server error")
		return
	}
	sourceFilter := c.Query("source")
	var sourceSlug *string
	if sourceFilter != "" {
		sourceSlug = &sourceFilter
	}
	deliveries, err := h.store.Deliveries.List(c.Request.Context(), sourceSlug, 50)
	if err != nil {
		slog.Error("failed to list deliveries", "error", err)
		c.String(http.StatusInternalServerError, "Internal server error")
		return
	}
	h.render(c, "deliveries", deliveriesData{
		Nav:          "deliveries",
		Sources:      sources,
		Deliveries:   deliveries,
		SourceFilter: sourceFilter,
	})
}

func (h *Handler) DeliveryDetail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid delivery ID")
		return
	}
	delivery, err := h.store.Deliveries.GetByID(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusNotFound, "Delivery not found")
		return
	}
	attempts, err := h.store.Deliveries.ListAttemptsByDelivery(c.Request.Context(), id)
	if err != nil {
		slog.Error("failed to list attempts", "error", err)
		c.String(http.StatusInternalServerError, "Internal server error")
		return
	}
	h.render(c, "delivery", deliveryData{
		Nav:      "deliveries",
		Delivery: delivery,
		Attempts: attempts,
	})
}

// Mutation handlers

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]+`)
var multiDash = regexp.MustCompile(`-{2,}`)

func generateSlug(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumDash.ReplaceAllString(s, "")
	s = multiDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func webhookURL(c *gin.Context, slug string) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/webhooks/%s", scheme, c.Request.Host, slug)
}

func (h *Handler) CreateSource(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		sources, _ := h.store.Sources.List(c.Request.Context())
		h.render(c, "sources", sourcesData{
			Nav:     "sources",
			Sources: sources,
			Error:   "Name is required",
		})
		return
	}
	slug := generateSlug(name)
	if slug == "" {
		sources, _ := h.store.Sources.List(c.Request.Context())
		h.render(c, "sources", sourcesData{
			Nav:     "sources",
			Sources: sources,
			Error:   "Could not generate slug from name",
		})
		return
	}
	_, err := h.store.Sources.Create(c.Request.Context(), name, slug, "record", nil)
	if err != nil {
		sources, _ := h.store.Sources.List(c.Request.Context())
		errMsg := "Failed to create source"
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			errMsg = "Source with this slug already exists"
		}
		h.render(c, "sources", sourcesData{
			Nav:     "sources",
			Sources: sources,
			Error:   errMsg,
		})
		return
	}
	c.Redirect(http.StatusSeeOther, "/sources/"+slug)
}

func (h *Handler) UpdateSource(c *gin.Context) {
	slug := c.Param("slug")
	name := strings.TrimSpace(c.PostForm("name"))
	if name != "" {
		if _, err := h.store.Sources.Update(c.Request.Context(), slug, &name, nil, nil, false); err != nil {
			slog.Error("failed to update source", "error", err)
		}
	}
	c.Redirect(http.StatusSeeOther, "/sources/"+slug)
}

func (h *Handler) DeleteSource(c *gin.Context) {
	slug := c.Param("slug")
	if err := h.store.Sources.Delete(c.Request.Context(), slug); err != nil {
		slog.Error("failed to delete source", "error", err)
		c.String(http.StatusInternalServerError, "Failed to delete source")
		return
	}
	c.Header("HX-Redirect", "/sources")
	c.Status(http.StatusOK)
}

func (h *Handler) UpdateSourceMode(c *gin.Context) {
	slug := c.Param("slug")
	mode := c.PostForm("mode")
	if mode != "record" && mode != "active" {
		c.String(http.StatusBadRequest, "Invalid mode")
		return
	}
	source, err := h.store.Sources.Update(c.Request.Context(), slug, nil, &mode, nil, false)
	if err != nil {
		slog.Error("failed to update source mode", "error", err)
		c.String(http.StatusInternalServerError, "Failed to update mode")
		return
	}
	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	h.renderFragment(c, "source", "mode-card", sourceData{
		Source:  source,
		Actions: actions,
	})
}

func (h *Handler) UpdateSourceScript(c *gin.Context) {
	slug := c.Param("slug")
	scriptBody := c.PostForm("script_body")

	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}

	var scriptError, scriptSuccess string
	if strings.TrimSpace(scriptBody) == "" {
		// Clear the script
		source, err = h.store.Sources.Update(c.Request.Context(), slug, nil, nil, nil, true)
		if err != nil {
			slog.Error("failed to clear script", "error", err)
			scriptError = "Failed to clear script"
		} else {
			scriptSuccess = "Script cleared"
		}
	} else {
		// Validate the script first
		if err := script.Validate(scriptBody); err != nil {
			scriptError = "Invalid script: " + err.Error()
		} else {
			source, err = h.store.Sources.Update(c.Request.Context(), slug, nil, nil, &scriptBody, false)
			if err != nil {
				slog.Error("failed to save script", "error", err)
				scriptError = "Failed to save script"
			} else {
				scriptSuccess = "Script saved"
			}
		}
	}

	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	deliveries, _ := h.store.Deliveries.List(c.Request.Context(), &slug, 10)
	h.renderFragment(c, "source", "script-card", sourceData{
		Source:        source,
		Actions:       actions,
		Deliveries:    deliveries,
		ScriptError:   scriptError,
		ScriptSuccess: scriptSuccess,
	})
}

func (h *Handler) ClearSourceScript(c *gin.Context) {
	slug := c.Param("slug")
	source, err := h.store.Sources.Update(c.Request.Context(), slug, nil, nil, nil, true)
	if err != nil {
		slog.Error("failed to clear script", "error", err)
		c.String(http.StatusInternalServerError, "Failed to clear script")
		return
	}
	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	deliveries, _ := h.store.Deliveries.List(c.Request.Context(), &slug, 10)
	h.renderFragment(c, "source", "script-card", sourceData{
		Source:        source,
		Actions:       actions,
		Deliveries:    deliveries,
		ScriptSuccess: "Script cleared",
	})
}

func (h *Handler) CreateAction(c *gin.Context) {
	slug := c.Param("slug")
	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}

	actionType := model.ActionType(c.PostForm("type"))
	if actionType == "" {
		actionType = model.ActionTypeWebhook
	}

	switch actionType {
	case model.ActionTypeWebhook:
		targetURL := strings.TrimSpace(c.PostForm("target_url"))
		if targetURL != "" {
			var signingSecret *string
			if s := strings.TrimSpace(c.PostForm("signing_secret")); s != "" {
				signingSecret = &s
			}
			if _, err := h.store.Actions.Create(c.Request.Context(), source.ID, actionType, &targetURL, signingSecret, nil); err != nil {
				slog.Error("failed to create action", "error", err)
			}
		}
	case model.ActionTypeJavascript:
		scriptBody := strings.TrimSpace(c.PostForm("script_body"))
		if scriptBody != "" {
			if err := script.ValidateAction(scriptBody); err != nil {
				slog.Error("invalid action script", "error", err)
			} else {
				if _, err := h.store.Actions.Create(c.Request.Context(), source.ID, actionType, nil, nil, &scriptBody); err != nil {
					slog.Error("failed to create action", "error", err)
				}
			}
		}
	}

	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	h.renderFragment(c, "source", "actions-card", sourceData{
		Source:  source,
		Actions: actions,
	})
}

func (h *Handler) ToggleAction(c *gin.Context) {
	slug := c.Param("slug")
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid action ID")
		return
	}
	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}
	isActive := c.PostForm("is_active") == "on"
	if _, err := h.store.Actions.Update(c.Request.Context(), id, nil, nil, &isActive, nil); err != nil {
		slog.Error("failed to toggle action", "error", err)
	}
	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	h.renderFragment(c, "source", "actions-card", sourceData{
		Source:  source,
		Actions: actions,
	})
}

func (h *Handler) TestSourceScript(c *gin.Context) {
	slug := c.Param("slug")
	scriptBody := c.PostForm("script_body")
	deliveryID := c.PostForm("delivery_id")

	if strings.TrimSpace(deliveryID) == "" {
		h.renderFragment(c, "source", "script-test-result", scriptTestData{
			Error: "Select a payload to test against",
		})
		return
	}

	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		h.renderFragment(c, "source", "script-test-result", scriptTestData{
			Error: "Source not found",
		})
		return
	}

	did, err := uuid.Parse(deliveryID)
	if err != nil {
		h.renderFragment(c, "source", "script-test-result", scriptTestData{
			Error: "Invalid delivery ID",
		})
		return
	}

	delivery, err := h.store.Deliveries.GetByID(c.Request.Context(), did)
	if err != nil || delivery.SourceID != source.ID {
		h.renderFragment(c, "source", "script-test-result", scriptTestData{
			Error: "Delivery not found for this source",
		})
		return
	}

	if strings.TrimSpace(scriptBody) == "" {
		h.renderFragment(c, "source", "script-test-result", scriptTestData{
			Error: "Script body is empty",
		})
		return
	}

	// Build transform input
	var payload map[string]any
	if delivery.Payload != nil {
		if err := json.Unmarshal(delivery.Payload, &payload); err != nil {
			payload = map[string]any{"_raw": string(delivery.Payload)}
		}
	}
	var headers map[string]string
	if delivery.Headers != nil {
		if err := json.Unmarshal(delivery.Headers, &headers); err != nil {
			headers = map[string]string{}
		}
	}

	actions, _ := h.store.Actions.ListActiveBySource(c.Request.Context(), source.ID)
	actionRefs := make([]script.ActionRef, len(actions))
	for i, a := range actions {
		targetURL := ""
		if a.TargetURL != nil {
			targetURL = *a.TargetURL
		}
		actionRefs[i] = script.ActionRef{ID: a.ID, TargetURL: targetURL}
	}

	input := script.TransformInput{
		Payload: payload,
		Headers: headers,
		Actions: actionRefs,
	}

	result, err := script.Run(scriptBody, input)
	if err != nil {
		h.renderFragment(c, "source", "script-test-result", scriptTestData{
			Error: err.Error(),
		})
		return
	}

	h.renderFragment(c, "source", "script-test-result", scriptTestData{
		Result: result,
	})
}

func (h *Handler) EditAction(c *gin.Context) {
	slug := c.Param("slug")

	// Get the action ID from the URL
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid action ID")
		return
	}

	// Get the source
	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}

	// Get the action
	action, err := h.store.Actions.GetByID(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusNotFound, "Action not found")
		return
	}

	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	h.renderFragment(c, "source", "action-edit-card", sourceData{
		Source:     source,
		Actions:    actions,
		EditAction: action,
	})
}

func (h *Handler) UpdateAction(c *gin.Context) {
	slug := c.Param("slug")
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid action ID")
		return
	}
	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}
	action, err := h.store.Actions.GetByID(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusNotFound, "Action not found")
		return
	}

	var actionError string
	switch action.Type {
	case model.ActionTypeWebhook:
		targetURL := strings.TrimSpace(c.PostForm("target_url"))
		if targetURL == "" {
			actionError = "Target URL is required for webhook actions"
		} else {
			var signingSecret *string
			if s := strings.TrimSpace(c.PostForm("signing_secret")); s != "" {
				signingSecret = &s
			}
			if _, err := h.store.Actions.Update(c.Request.Context(), id, &targetURL, signingSecret, nil, nil); err != nil {
				slog.Error("failed to update action", "error", err)
				actionError = "Failed to update action"
			}
		}
	case model.ActionTypeJavascript:
		scriptBody := strings.TrimSpace(c.PostForm("script_body"))
		if scriptBody == "" {
			actionError = "Script body is required for javascript actions"
		} else if err := script.ValidateAction(scriptBody); err != nil {
			actionError = "Invalid script: " + err.Error()
		} else {
			if _, err := h.store.Actions.Update(c.Request.Context(), id, nil, nil, nil, &scriptBody); err != nil {
				slog.Error("failed to update action", "error", err)
				actionError = "Failed to update action"
			}
		}
	}

	if actionError != "" {
		// Re-fetch action for edit form
		action, _ = h.store.Actions.GetByID(c.Request.Context(), id)
		actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
		h.renderFragment(c, "source", "action-edit-card", sourceData{
			Source:      source,
			Actions:     actions,
			EditAction:  action,
			ActionError: actionError,
		})
		return
	}

	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	h.renderFragment(c, "source", "actions-card", sourceData{
		Source:        source,
		Actions:       actions,
		ActionSuccess: "Action updated",
	})
}

func (h *Handler) DeleteAction(c *gin.Context) {
	slug := c.Param("slug")
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid action ID")
		return
	}
	source, err := h.store.Sources.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.String(http.StatusNotFound, "Source not found")
		return
	}
	if err := h.store.Actions.Delete(c.Request.Context(), id); err != nil {
		slog.Error("failed to delete action", "error", err)
	}
	actions, _ := h.store.Actions.List(c.Request.Context(), source.ID)
	h.renderFragment(c, "source", "actions-card", sourceData{
		Source:  source,
		Actions: actions,
	})
}
