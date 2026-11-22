package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AVeukov/url-shortener/internal/metrics"
	"github.com/AVeukov/url-shortener/internal/model"
)

type serviceStub struct {
	link       model.Link
	err        error
	recorded   bool
	createdURL string
}

func (s *serviceStub) Create(_ context.Context, rawURL, _ string, _ time.Duration) (model.Link, error) {
	s.createdURL = rawURL
	return s.link, s.err
}
func (s *serviceStub) Resolve(_ context.Context, _ string) (model.Link, error) {
	return s.link, s.err
}
func (s *serviceStub) Get(_ context.Context, _ string) (model.Link, error) {
	return s.link, s.err
}
func (s *serviceStub) RecordRedirect(_ context.Context, _ string) error {
	s.recorded = true
	return nil
}

type healthStub struct{ err error }

func (h healthStub) Ping(context.Context) error { return h.err }

func testHandler(svc *serviceStub) http.Handler {
	return New(svc, healthStub{}, healthStub{}, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCreateLink(t *testing.T) {
	svc := &serviceStub{link: model.Link{Code: "abcd1234", OriginalURL: "https://example.com"}}
	request := httptest.NewRequest(http.MethodPost, "http://short.test/api/v1/urls", strings.NewReader(`{"url":"https://example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testHandler(svc).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if svc.createdURL != "https://example.com" || !strings.Contains(response.Body.String(), `"short_url":"http://short.test/abcd1234"`) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestRedirect(t *testing.T) {
	svc := &serviceStub{link: model.Link{Code: "abcd1234", OriginalURL: "https://example.com/path"}}
	response := httptest.NewRecorder()
	testHandler(svc).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://short.test/abcd1234", nil))

	if response.Code != http.StatusFound || response.Header().Get("Location") != "https://example.com/path" {
		t.Fatalf("status = %d, location = %q", response.Code, response.Header().Get("Location"))
	}
	if !svc.recorded {
		t.Fatal("redirect was not recorded")
	}
}

func TestReadinessFailure(t *testing.T) {
	svc := &serviceStub{}
	handler := New(svc, healthStub{err: errors.New("down")}, healthStub{}, metrics.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://short.test/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	response := httptest.NewRecorder()
	testHandler(&serviceStub{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://short.test/metrics", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "url_shortener_links_created_total") {
		t.Fatalf("unexpected metrics response: %s", response.Body.String())
	}
}
