package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL       string
	RedisURL          string
	Port              string
	WorkerConcurrency int
	MaxRetries        int
	RetryBaseDelay    time.Duration
	DeliveryTimeout   time.Duration
	PollInterval      time.Duration
}

func Load() Config {
	return Config{
		DatabaseURL:       envOrDefault("DATABASE_URL", "postgres://relay:relay@localhost:5432/webhook_relay?sslmode=disable"),
		RedisURL:          envOrDefault("REDIS_URL", "redis://localhost:6379"),
		Port:              envOrDefault("PORT", "8080"),
		WorkerConcurrency: envOrDefaultInt("WORKER_CONCURRENCY", 4),
		MaxRetries:        envOrDefaultInt("MAX_RETRIES", 5),
		RetryBaseDelay:    envOrDefaultDuration("RETRY_BASE_DELAY", 5*time.Second),
		DeliveryTimeout:   envOrDefaultDuration("DELIVERY_TIMEOUT", 10*time.Second),
		PollInterval:      envOrDefaultDuration("POLL_INTERVAL", 30*time.Second),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefaultDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
