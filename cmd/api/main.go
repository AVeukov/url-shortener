package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AVeukov/url-shortener/internal/cache"
	"github.com/AVeukov/url-shortener/internal/config"
	"github.com/AVeukov/url-shortener/internal/httpapi"
	appmetrics "github.com/AVeukov/url-shortener/internal/metrics"
	"github.com/AVeukov/url-shortener/internal/repository"
	"github.com/AVeukov/url-shortener/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := repository.NewPostgres(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	linkCache, err := cache.NewRedis(startupCtx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.CacheTTL)
	if err != nil {
		return err
	}
	defer linkCache.Close()

	m := appmetrics.New()
	svc := service.New(db, linkCache, m)
	handler := httpapi.New(svc, db, linkCache, m, logger)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case signal := <-signals:
		logger.Info("shutdown signal received", "signal", signal.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}
