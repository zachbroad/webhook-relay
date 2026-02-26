package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
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
	withWorker := flag.Bool("worker", false, "also run the fan-out worker in-process")
	flag.Parse()

	_ = godotenv.Load()  // Load .env file
	cfg := config.Load() // Load config from environment variables

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
	sourceH := handler.NewSourceHandler(s)
	actionH := handler.NewActionHandler(s)
	deliveryH := handler.NewDeliveryHandler(s)
	webH := web.NewHandler(s)

	// Routes
	r := gin.Default()
	r.RedirectFixedPath = true
	r.RedirectTrailingSlash = true

	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, ".")
	})

	// Web UI
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/sources")
	})
	r.GET("/sources", webH.Sources)
	r.POST("/sources", webH.CreateSource)
	r.GET("/sources/:slug", webH.SourceDetail)
	r.POST("/sources/:slug/update", webH.UpdateSource)
	r.DELETE("/sources/:slug", webH.DeleteSource)
	r.POST("/sources/:slug/mode", webH.UpdateSourceMode)
	r.POST("/sources/:slug/script", webH.UpdateSourceScript)
	r.POST("/sources/:slug/script/clear", webH.ClearSourceScript)
	r.POST("/sources/:slug/script/test", webH.TestSourceScript)
	r.POST("/sources/:slug/actions", webH.CreateAction)
	r.GET("/sources/:slug/actions/:id/edit", webH.EditAction)
	r.POST("/sources/:slug/actions/:id/update", webH.UpdateAction)
	r.POST("/sources/:slug/actions/:id/toggle", webH.ToggleAction)
	r.DELETE("/sources/:slug/actions/:id", webH.DeleteAction)
	r.GET("/deliveries", webH.Deliveries)
	r.GET("/deliveries/:id", webH.DeliveryDetail)

	// Webhook ingest
	r.POST("/webhooks/:sourceSlug", webhookH.Ingest)

	// JSON API
	api := r.Group("/api")
	{
		sources := api.Group("/sources")
		{
			sources.GET("", sourceH.List)
			sources.POST("", sourceH.Create)
			srcGroup := sources.Group("/:sourceSlug")
			{
				srcGroup.GET("", sourceH.Get)
				srcGroup.PATCH("", sourceH.Update)
				srcGroup.DELETE("", sourceH.Delete)
				actions := srcGroup.Group("/actions")
				{
					actions.POST("", actionH.Create)
					actions.GET("", actionH.List)
					actions.GET("/:id", actionH.Get)
					actions.PATCH("/:id", actionH.Update)
					actions.DELETE("/:id", actionH.Delete)
				}
			}
		}
		deliveries := api.Group("/deliveries")
		{
			deliveries.GET("", deliveryH.List)
			deliveries.GET("/:id", deliveryH.Get)
			deliveries.GET("/:id/attempts", deliveryH.ListAttempts)
		}
	}

	// Optionally start fan-out worker in-process for local development
	if *withWorker {
		w := worker.New(s, rdb, cfg.WorkerConcurrency, cfg.MaxRetries, cfg.RetryBaseDelay, cfg.DeliveryTimeout, cfg.PollInterval)
		if err := w.Start(ctx); err != nil {
			slog.Error("failed to start worker", "error", err)
			os.Exit(1)
		}
		slog.Info("fan-out worker started", "concurrency", cfg.WorkerConcurrency)
	}

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
