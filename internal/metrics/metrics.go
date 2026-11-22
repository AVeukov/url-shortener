package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var durationBuckets = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type requestKey struct{ method, route, status string }
type routeKey struct{ method, route string }

type histogram struct {
	buckets [len(durationBuckets)]uint64
	count   uint64
	sum     float64
}

type Metrics struct {
	mu            sync.Mutex
	requests      map[requestKey]uint64
	latency       map[routeKey]*histogram
	cacheRequests map[string]uint64
	linksCreated  uint64
	redirects     uint64
}

func New() *Metrics {
	return &Metrics{
		requests:      make(map[requestKey]uint64),
		latency:       make(map[routeKey]*histogram),
		cacheRequests: make(map[string]uint64),
	}
}

func (m *Metrics) ObserveRequest(method, route, status string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[requestKey{method, route, status}]++
	key := routeKey{method, route}
	h := m.latency[key]
	if h == nil {
		h = &histogram{}
		m.latency[key] = h
	}
	seconds := duration.Seconds()
	h.count++
	h.sum += seconds
	for i, upperBound := range durationBuckets {
		if seconds <= upperBound {
			h.buckets[i]++
			break
		}
	}
}

func (m *Metrics) IncLinksCreated() {
	m.mu.Lock()
	m.linksCreated++
	m.mu.Unlock()
}

func (m *Metrics) IncRedirects() {
	m.mu.Lock()
	m.redirects++
	m.mu.Unlock()
}

func (m *Metrics) ObserveCache(result string) {
	m.mu.Lock()
	m.cacheRequests[result]++
	m.mu.Unlock()
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(m.render()))
	})
}

func (m *Metrics) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var output strings.Builder
	output.WriteString("# HELP url_shortener_links_created_total Total number of short links created.\n")
	output.WriteString("# TYPE url_shortener_links_created_total counter\n")
	fmt.Fprintf(&output, "url_shortener_links_created_total %d\n", m.linksCreated)
	output.WriteString("# HELP url_shortener_redirects_total Total number of successful redirects.\n")
	output.WriteString("# TYPE url_shortener_redirects_total counter\n")
	fmt.Fprintf(&output, "url_shortener_redirects_total %d\n", m.redirects)

	output.WriteString("# HELP url_shortener_cache_requests_total Cache lookups split by result.\n")
	output.WriteString("# TYPE url_shortener_cache_requests_total counter\n")
	for _, result := range sortedKeys(m.cacheRequests) {
		fmt.Fprintf(&output, "url_shortener_cache_requests_total{result=%q} %d\n", escape(result), m.cacheRequests[result])
	}

	output.WriteString("# HELP url_shortener_http_requests_total Total number of HTTP requests.\n")
	output.WriteString("# TYPE url_shortener_http_requests_total counter\n")
	requestKeys := make([]requestKey, 0, len(m.requests))
	for key := range m.requests {
		requestKeys = append(requestKeys, key)
	}
	sort.Slice(requestKeys, func(i, j int) bool {
		return requestKeys[i].method+requestKeys[i].route+requestKeys[i].status < requestKeys[j].method+requestKeys[j].route+requestKeys[j].status
	})
	for _, key := range requestKeys {
		fmt.Fprintf(&output, "url_shortener_http_requests_total{method=%q,route=%q,status=%q} %d\n", escape(key.method), escape(key.route), escape(key.status), m.requests[key])
	}

	output.WriteString("# HELP url_shortener_http_request_duration_seconds HTTP request duration in seconds.\n")
	output.WriteString("# TYPE url_shortener_http_request_duration_seconds histogram\n")
	routeKeys := make([]routeKey, 0, len(m.latency))
	for key := range m.latency {
		routeKeys = append(routeKeys, key)
	}
	sort.Slice(routeKeys, func(i, j int) bool {
		return routeKeys[i].method+routeKeys[i].route < routeKeys[j].method+routeKeys[j].route
	})
	for _, key := range routeKeys {
		h := m.latency[key]
		var cumulative uint64
		for i, upperBound := range durationBuckets {
			cumulative += h.buckets[i]
			fmt.Fprintf(&output, "url_shortener_http_request_duration_seconds_bucket{method=%q,route=%q,le=%q} %d\n", escape(key.method), escape(key.route), fmt.Sprint(upperBound), cumulative)
		}
		fmt.Fprintf(&output, "url_shortener_http_request_duration_seconds_bucket{method=%q,route=%q,le=\"+Inf\"} %d\n", escape(key.method), escape(key.route), h.count)
		fmt.Fprintf(&output, "url_shortener_http_request_duration_seconds_sum{method=%q,route=%q} %g\n", escape(key.method), escape(key.route), h.sum)
		fmt.Fprintf(&output, "url_shortener_http_request_duration_seconds_count{method=%q,route=%q} %d\n", escape(key.method), escape(key.route), h.count)
	}
	return output.String()
}

func sortedKeys(values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
