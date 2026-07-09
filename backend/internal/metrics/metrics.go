package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// --- HTTP metrics ---

	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"method", "path"},
	)

	RequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed.",
		},
	)

	// --- OpenAI metrics ---

	OpenAIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watchgpt_openai_requests_total",
			Help: "Total requests to OpenAI by status (success/error).",
		},
		[]string{"status"},
	)

	OpenAIRequestDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "watchgpt_openai_request_duration_seconds",
			Help:    "Time spent waiting for OpenAI response.",
			Buckets: []float64{0.5, 1, 2, 5, 10, 15, 30, 60},
		},
	)

	// --- Cache metrics ---

	CacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "watchgpt_cache_hits_total",
			Help: "Total cache hits.",
		},
	)

	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "watchgpt_cache_misses_total",
			Help: "Total cache misses.",
		},
	)

	// --- Business metrics ---

	ImageSizeBytes = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "watchgpt_image_size_bytes",
			Help:    "Size of images sent for analysis.",
			Buckets: []float64{10000, 50000, 100000, 250000, 500000, 1000000, 5000000},
		},
	)

	ActiveConversations = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "watchgpt_active_conversations",
			Help: "Number of chat requests currently in-flight.",
		},
	)

	// --- Circuit breaker metrics ---

	CircuitBreakerState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "watchgpt_circuit_breaker_state",
			Help: "Circuit breaker state: 0=closed, 1=half-open, 2=open.",
		},
	)
)
