package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusOutput(t *testing.T) {
	m := New()
	m.IncLinksCreated()
	m.IncRedirects()
	m.ObserveCache("hit")
	m.ObserveRequest(http.MethodGet, "GET /{code}", "302", 20*time.Millisecond)

	response := httptest.NewRecorder()
	m.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{
		"url_shortener_links_created_total 1",
		"url_shortener_redirects_total 1",
		`url_shortener_cache_requests_total{result="hit"} 1`,
		`url_shortener_http_requests_total{method="GET",route="GET /{code}",status="302"} 1`,
		`url_shortener_http_request_duration_seconds_count{method="GET",route="GET /{code}"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics output does not contain %q:\n%s", expected, body)
		}
	}
}
