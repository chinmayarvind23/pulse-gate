package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr     string
	RedisAddr      string
	HMACSecret     string
	Stream         string
	ResultStream   string
	IdempotencyTTL time.Duration
	MaxBodyBytes   int64
	MaxQueue       int64
}

// Load converts environment text into one validated configuration value. Keeping
// parsing at the process boundary prevents repeated conversions and makes bad
// production configuration fail fast instead of becoming a request-time surprise.
func Load() (Config, error) {
	ttlSeconds, err := intEnv("PULSEGATE_IDEMPOTENCY_TTL_SECONDS", 86400)
	if err != nil {
		return Config{}, err
	}
	maxBody, err := intEnv("PULSEGATE_MAX_BODY_BYTES", 65536)
	if err != nil {
		return Config{}, err
	}
	maxQueue, err := intEnv("PULSEGATE_MAX_QUEUE", 1000000)
	if err != nil || maxQueue <= 0 {
		return Config{}, fmt.Errorf("PULSEGATE_MAX_QUEUE must be positive")
	}

	cfg := Config{
		ListenAddr:     env("PULSEGATE_LISTEN_ADDR", ":8080"),
		RedisAddr:      env("PULSEGATE_REDIS_ADDR", "localhost:6379"),
		HMACSecret:     env("PULSEGATE_HMAC_SECRET", ""),
		Stream:         env("PULSEGATE_STREAM", "payment_events"),
		ResultStream:   env("PULSEGATE_RESULT_STREAM", "risk_decisions"),
		IdempotencyTTL: time.Duration(ttlSeconds) * time.Second,
		MaxBodyBytes:   int64(maxBody),
		MaxQueue:       int64(maxQueue),
	}
	if cfg.Stream == cfg.ResultStream {
		return Config{}, fmt.Errorf("input and result streams must differ")
	}
	if cfg.HMACSecret == "" {
		return Config{}, fmt.Errorf("PULSEGATE_HMAC_SECRET is required")
	}
	if ttlSeconds <= 0 || ttlSeconds > 31536000 {
		return Config{}, fmt.Errorf("idempotency TTL must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return Config{}, fmt.Errorf("max body bytes must be positive")
	}
	return cfg, nil
}

// env applies defaults only to unset or empty settings.
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// intEnv reports configuration mistakes before the server admits traffic.
func intEnv(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return value, nil
}
