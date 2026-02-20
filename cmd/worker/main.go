package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/zachbroad/webhook-relay/internal/config"
	"github.com/zachbroad/webhook-relay/internal/database"
	"github.com/zachbroad/webhook-relay/internal/store"
	"github.com/zachbroad/webhook-relay/internal/worker"
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

	// Initialize store and start fan-out worker
	s := store.New(pool)
	w := worker.New(s, rdb, cfg.WorkerConcurrency, cfg.MaxRetries, cfg.RetryBaseDelay, cfg.DeliveryTimeout, cfg.PollInterval)
	if err := w.Start(ctx); err != nil {
		slog.Error("failed to start worker", "error", err)
		os.Exit(1)
	}
	slog.Info("fan-out worker started", "concurrency", cfg.WorkerConcurrency)

	// Minimal health endpoint for k8s liveness probes
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	healthSrv := &http.Server{
		Addr:    ":8081",
		Handler: healthMux,
	}

	go func() {
		slog.Info("worker health server listening", "port", "8081")
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down worker...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("health server shutdown error", "error", err)
	}
	slog.Info("worker stopped")
}
