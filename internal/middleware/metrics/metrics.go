package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP request duration histogram
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	// HTTP request counter
	httpRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "route", "status"},
	)

	// HTTP request size
	httpRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_size_bytes",
			Help:    "Size of HTTP requests in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000},
		},
		[]string{"method", "route"},
	)

	// HTTP response size
	httpResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "Size of HTTP responses in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000},
		},
		[]string{"method", "route"},
	)
)

// Middleware returns a metrics middleware
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status and size
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Record request size
		if r.ContentLength > 0 {
			httpRequestSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(r.ContentLength))
		}

		// Process request
		next.ServeHTTP(rw, r)

		// Calculate duration
		duration := time.Since(start).Seconds()

		// Record metrics
		status := strconv.Itoa(rw.statusCode)
		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, status).Observe(duration)
		httpRequestTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()

		// Record response size if available
		if rw.size > 0 {
			httpResponseSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(rw.size))
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// Handler returns the Prometheus metrics handler
func Handler() http.Handler {
	return promhttp.Handler()
}

