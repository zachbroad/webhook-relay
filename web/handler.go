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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zachbroad/webhook-relay/internal/model"
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

func (h *Handler) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates[page].ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("template render error", "page", page, "error", err)
	}
}

func (h *Handler) renderFragment(w http.ResponseWriter, page string, fragment string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates[page].ExecuteTemplate(w, fragment, data); err != nil {
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
	Subscriptions []model.Subscription
	WebhookURL    string
	Error         string
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

func (h *Handler) Sources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.Sources.List(r.Context())
	if err != nil {
		slog.Error("failed to list sources", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.render(w, "sources", sourcesData{
		Nav:     "sources",
		Sources: sources,
	})
}

func (h *Handler) SourceDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	source, err := h.store.Sources.GetBySlug(r.Context(), slug)
	if err != nil {
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}
	subs, err := h.store.Subscriptions.List(r.Context(), source.ID)
	if err != nil {
		slog.Error("failed to list subscriptions", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.render(w, "source", sourceData{
		Nav:           "sources",
		Source:        source,
		Subscriptions: subs,
		WebhookURL:    webhookURL(r, source.Slug),
	})
}

func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.Sources.List(r.Context())
	if err != nil {
		slog.Error("failed to list sources", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	sourceFilter := r.URL.Query().Get("source")
	var sourceSlug *string
	if sourceFilter != "" {
		sourceSlug = &sourceFilter
	}
	deliveries, err := h.store.Deliveries.List(r.Context(), sourceSlug, 50)
	if err != nil {
		slog.Error("failed to list deliveries", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.render(w, "deliveries", deliveriesData{
		Nav:          "deliveries",
		Sources:      sources,
		Deliveries:   deliveries,
		SourceFilter: sourceFilter,
	})
}

func (h *Handler) DeliveryDetail(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid delivery ID", http.StatusBadRequest)
		return
	}
	delivery, err := h.store.Deliveries.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Delivery not found", http.StatusNotFound)
		return
	}
	attempts, err := h.store.Deliveries.ListAttemptsByDelivery(r.Context(), id)
	if err != nil {
		slog.Error("failed to list attempts", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	h.render(w, "delivery", deliveryData{
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

func webhookURL(r *http.Request, slug string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/webhooks/%s", scheme, r.Host, slug)
}

func (h *Handler) CreateSource(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		sources, _ := h.store.Sources.List(r.Context())
		h.render(w, "sources", sourcesData{
			Nav:     "sources",
			Sources: sources,
			Error:   "Name is required",
		})
		return
	}
	slug := generateSlug(name)
	if slug == "" {
		sources, _ := h.store.Sources.List(r.Context())
		h.render(w, "sources", sourcesData{
			Nav:     "sources",
			Sources: sources,
			Error:   "Could not generate slug from name",
		})
		return
	}
	_, err := h.store.Sources.Create(r.Context(), name, slug)
	if err != nil {
		sources, _ := h.store.Sources.List(r.Context())
		errMsg := "Failed to create source"
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			errMsg = "Source with this slug already exists"
		}
		h.render(w, "sources", sourcesData{
			Nav:     "sources",
			Sources: sources,
			Error:   errMsg,
		})
		return
	}
	http.Redirect(w, r, "/sources/"+slug, http.StatusSeeOther)
}

func (h *Handler) UpdateSource(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	name := strings.TrimSpace(r.FormValue("name"))
	if name != "" {
		if _, err := h.store.Sources.Update(r.Context(), slug, &name); err != nil {
			slog.Error("failed to update source", "error", err)
		}
	}
	http.Redirect(w, r, "/sources/"+slug, http.StatusSeeOther)
}

func (h *Handler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := h.store.Sources.Delete(r.Context(), slug); err != nil {
		slog.Error("failed to delete source", "error", err)
		http.Error(w, "Failed to delete source", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Redirect", "/sources")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	source, err := h.store.Sources.GetBySlug(r.Context(), slug)
	if err != nil {
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}
	targetURL := strings.TrimSpace(r.FormValue("target_url"))
	if targetURL != "" {
		var signingSecret *string
		if s := strings.TrimSpace(r.FormValue("signing_secret")); s != "" {
			signingSecret = &s
		}
		if _, err := h.store.Subscriptions.Create(r.Context(), source.ID, targetURL, signingSecret); err != nil {
			slog.Error("failed to create subscription", "error", err)
		}
	}
	subs, _ := h.store.Subscriptions.List(r.Context(), source.ID)
	h.renderFragment(w, "source", "subscriptions-card", sourceData{
		Source:        source,
		Subscriptions: subs,
	})
}

func (h *Handler) ToggleSubscription(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid subscription ID", http.StatusBadRequest)
		return
	}
	source, err := h.store.Sources.GetBySlug(r.Context(), slug)
	if err != nil {
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}
	isActive := r.FormValue("is_active") == "on"
	if _, err := h.store.Subscriptions.Update(r.Context(), id, nil, nil, &isActive); err != nil {
		slog.Error("failed to toggle subscription", "error", err)
	}
	subs, _ := h.store.Subscriptions.List(r.Context(), source.ID)
	h.renderFragment(w, "source", "subscriptions-card", sourceData{
		Source:        source,
		Subscriptions: subs,
	})
}

func (h *Handler) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid subscription ID", http.StatusBadRequest)
		return
	}
	source, err := h.store.Sources.GetBySlug(r.Context(), slug)
	if err != nil {
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}
	if err := h.store.Subscriptions.Delete(r.Context(), id); err != nil {
		slog.Error("failed to delete subscription", "error", err)
	}
	subs, _ := h.store.Subscriptions.List(r.Context(), source.ID)
	h.renderFragment(w, "source", "subscriptions-card", sourceData{
		Source:        source,
		Subscriptions: subs,
	})
}
