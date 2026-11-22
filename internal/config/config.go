package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	CacheTTL        time.Duration
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	redisDB, err := strconv.Atoi(env("REDIS_DB", "0"))
	if err != nil {
		return Config{}, fmt.Errorf("parse REDIS_DB: %w", err)
	}

	cacheTTL, err := time.ParseDuration(env("CACHE_TTL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("parse CACHE_TTL: %w", err)
	}

	shutdownTimeout, err := time.ParseDuration(env("SHUTDOWN_TIMEOUT", "10s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
	}

	return Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://shortener:shortener@localhost:5432/shortener?sslmode=disable"),
		RedisAddr:       env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         redisDB,
		CacheTTL:        cacheTTL,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
