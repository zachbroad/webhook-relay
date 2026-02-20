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

	// Connect to Redis (needed for XADD on webhook ingest)
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
	subscriptionH := handler.NewSubscriptionHandler(s)
	deliveryH := handler.NewDeliveryHandler(s)

	// Routes
	r := chi.NewRouter()
	r.Use(middleware.Heartbeat("/healthz"))
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Post("/webhooks/{sourceSlug}", webhookH.Ingest)

	r.Route("/sources/{sourceSlug}/subscriptions", func(r chi.Router) {
		r.Post("/", subscriptionH.Create)
		r.Get("/", subscriptionH.List)
		r.Get("/{id}", subscriptionH.Get)
		r.Patch("/{id}", subscriptionH.Update)
		r.Delete("/{id}", subscriptionH.Delete)
	})

	r.Get("/deliveries", deliveryH.List)

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		slog.Info("api server listening", "port", cfg.Port)
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
	slog.Info("api server stopped")
}
