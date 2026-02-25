package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/athena-dhcpd/athena-dhcpd/internal/metrics"
)

// metricsMiddleware wraps an http.Handler to record request metrics.
type metricsMiddleware struct {
	next http.Handler
}

// newMetricsMiddleware wraps a handler with Prometheus metrics instrumentation.
func newMetricsMiddleware(next http.Handler) http.Handler {
	return &metricsMiddleware{next: next}
}

func (m *metricsMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

	m.next.ServeHTTP(sw, r)

	duration := time.Since(start).Seconds()
	path := normalizePath(r.URL.Path)

	metrics.APIRequests.WithLabelValues(r.Method, path, strconv.Itoa(sw.status)).Inc()
	metrics.APIRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
}

// statusWriter captures the HTTP status code.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// normalizePath reduces cardinality by collapsing dynamic path segments.
func normalizePath(path string) string {
	switch {
	case len(path) > 17 && path[:17] == "/api/v1/leases/ex":
		return "/api/v1/leases/export"
	case len(path) > 15 && path[:15] == "/api/v1/leases/":
		return "/api/v1/leases/{ip}"
	case len(path) > 22 && path[:22] == "/api/v1/reservations/e":
		return "/api/v1/reservations/export"
	case len(path) > 22 && path[:22] == "/api/v1/reservations/i":
		return "/api/v1/reservations/import"
	case len(path) > 21 && path[:21] == "/api/v1/reservations/":
		return "/api/v1/reservations/{id}"
	case len(path) > 22 && path[:22] == "/api/v1/conflicts/his":
		return "/api/v1/conflicts/history"
	case len(path) > 22 && path[:22] == "/api/v1/conflicts/sta":
		return "/api/v1/conflicts/stats"
	case len(path) > 18 && path[:18] == "/api/v1/conflicts/":
		return "/api/v1/conflicts/{ip}"
	case len(path) > 23 && path[:23] == "/api/v1/config/backups/":
		return "/api/v1/config/backups/{timestamp}"
	default:
		return path
	}
}
