package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/zachbroad/webhook-relay/internal/config"
	"github.com/zachbroad/webhook-relay/internal/database"
	"github.com/zachbroad/webhook-relay/internal/handler"
	"github.com/zachbroad/webhook-relay/internal/store"
	"github.com/zachbroad/webhook-relay/internal/worker"
	"github.com/zachbroad/webhook-relay/web"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Connect to Postgres
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to postgres")

	// Connect to Redis
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to parse redis URL", "error", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()
	slog.Info("connected to redis")

	// Initialize store and handlers
	s := store.New(pool)
	webhookH := handler.NewWebhookHandler(s, rdb)
	sourceH := handler.NewSourceHandler(s)
	subscriptionH := handler.NewSubscriptionHandler(s)
	deliveryH := handler.NewDeliveryHandler(s)
	webH := web.NewHandler(s)

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.Heartbeat("/healthz"))
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Web UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/sources", http.StatusFound)
	})
	r.Get("/sources", webH.Sources)
	r.Post("/sources", webH.CreateSource)
	r.Get("/sources/{slug}", webH.SourceDetail)
	r.Post("/sources/{slug}/update", webH.UpdateSource)
	r.Delete("/sources/{slug}", webH.DeleteSource)
	r.Post("/sources/{slug}/subscriptions", webH.CreateSubscription)
	r.Post("/sources/{slug}/subscriptions/{id}/toggle", webH.ToggleSubscription)
	r.Delete("/sources/{slug}/subscriptions/{id}", webH.DeleteSubscription)
	r.Get("/deliveries", webH.Deliveries)
	r.Get("/deliveries/{id}", webH.DeliveryDetail)

	// Webhook ingest
	r.Post("/webhooks/{sourceSlug}", webhookH.Ingest)

	// JSON API
	r.Route("/api", func(r chi.Router) {
		r.Route("/sources", func(r chi.Router) {
			r.Get("/", sourceH.List)
			r.Post("/", sourceH.Create)
			r.Route("/{sourceSlug}", func(r chi.Router) {
				r.Get("/", sourceH.Get)
				r.Patch("/", sourceH.Update)
				r.Delete("/", sourceH.Delete)
				r.Route("/subscriptions", func(r chi.Router) {
					r.Post("/", subscriptionH.Create)
					r.Get("/", subscriptionH.List)
					r.Get("/{id}", subscriptionH.Get)
					r.Patch("/{id}", subscriptionH.Update)
					r.Delete("/{id}", subscriptionH.Delete)
				})
			})
		})
		r.Route("/deliveries", func(r chi.Router) {
			r.Get("/", deliveryH.List)
			r.Get("/{id}", deliveryH.Get)
			r.Get("/{id}/attempts", deliveryH.ListAttempts)
		})
	})

	// Start fan-out worker
	w := worker.New(s, rdb, cfg.WorkerConcurrency, cfg.MaxRetries, cfg.RetryBaseDelay, cfg.DeliveryTimeout, cfg.PollInterval)
	if err := w.Start(ctx); err != nil {
		slog.Error("failed to start worker", "error", err)
		os.Exit(1)
	}
	slog.Info("fan-out worker started", "concurrency", cfg.WorkerConcurrency)

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		slog.Info("server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("server stopped")
}
