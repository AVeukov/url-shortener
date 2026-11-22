package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AVeukov/url-shortener/internal/metrics"
	"github.com/AVeukov/url-shortener/internal/model"
	"github.com/AVeukov/url-shortener/internal/repository"
	"github.com/AVeukov/url-shortener/internal/service"
)

const maxBodySize = 1 << 20

type LinkService interface {
	Create(ctx context.Context, rawURL, customAlias string, lifetime time.Duration) (model.Link, error)
	Resolve(ctx context.Context, code string) (model.Link, error)
	Get(ctx context.Context, code string) (model.Link, error)
	RecordRedirect(ctx context.Context, code string) error
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	service LinkService
	db      HealthChecker
	cache   HealthChecker
	metrics *metrics.Metrics
	logger  *slog.Logger
}

func New(svc LinkService, db, linkCache HealthChecker, m *metrics.Metrics, logger *slog.Logger) http.Handler {
	h := &Handler{service: svc, db: db, cache: linkCache, metrics: m, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/urls", h.createLink)
	mux.HandleFunc("GET /api/v1/urls/{code}", h.getLink)
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.Handle("GET /metrics", m.Handler())
	mux.HandleFunc("GET /{code}", h.redirect)
	return h.observe(mux)
}

type createRequest struct {
	URL              string `json:"url"`
	CustomAlias      string `json:"custom_alias"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type linkResponse struct {
	model.Link
	ShortURL string `json:"short_url"`
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request createRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if request.ExpiresInSeconds < 0 || request.ExpiresInSeconds > int64((365*24*time.Hour)/time.Second) {
		writeError(w, http.StatusBadRequest, "expires_in_seconds must be between 0 and 31536000")
		return
	}

	link, err := h.service.Create(r.Context(), request.URL, request.CustomAlias, time.Duration(request.ExpiresInSeconds)*time.Second)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "url must be an absolute HTTP or HTTPS URL")
		case errors.Is(err, service.ErrInvalidAlias):
			writeError(w, http.StatusBadRequest, "custom_alias must contain 4-32 letters, digits, hyphens or underscores")
		case errors.Is(err, service.ErrAliasTaken):
			writeError(w, http.StatusConflict, "custom alias is already taken")
		default:
			h.logger.Error("create short link", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, linkResponse{Link: link, ShortURL: baseURL(r) + "/" + link.Code})
}

func (h *Handler) getLink(w http.ResponseWriter, r *http.Request) {
	link, err := h.service.Get(r.Context(), r.PathValue("code"))
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "short link not found")
		return
	}
	if err != nil {
		h.logger.Error("get link statistics", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, linkResponse{Link: link, ShortURL: baseURL(r) + "/" + link.Code})
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := h.service.Resolve(r.Context(), code)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "short link not found")
		return
	}
	if err != nil {
		h.logger.Error("resolve short link", "error", err, "code", code)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if err := h.service.RecordRedirect(r.Context(), code); err != nil {
		h.logger.Error("record redirect", "error", err, "code", code)
	}
	http.Redirect(w, r, link.OriginalURL, http.StatusFound)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "postgres is unavailable")
		return
	}
	if err := h.cache.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "redis is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		h.metrics.ObserveRequest(r.Method, route, strconv.Itoa(wrapped.status), time.Since(started))
		h.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "status", wrapped.status, "duration", time.Since(started))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
