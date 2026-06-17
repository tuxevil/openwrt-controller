package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// statusClass maps a numeric HTTP status code to its class label
// (1xx, 2xx, 3xx, 4xx, 5xx, error). Used to keep the cardinality of
// the status label bounded.
func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "error"
	}
}

// responseRecorder wraps http.ResponseWriter so we can capture the
// status code written by the downstream handler. The default
// ResponseWriter only exposes status via http.ResponseController in
// Go 1.20+, and even then only when the handler hasn't already
// called WriteHeader.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.status = http.StatusOK
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// Middleware returns an http.Middleware-style function that records
// request count and duration for the wrapped handler. routeLabel is a
// function that maps the current request to a low-cardinality route
// pattern (e.g. "/api/sites/{site_id}"). If you do not have routing
// info available, pass func(*http.Request) string { return "" } and
// the route label will be empty.
func (r *Registry) Middleware(routeLabel func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()

			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, req)

			route := routeLabel(req)
			status := strconv.Itoa(rec.status) // e.g. "200"
			_ = status                            // not used as a label; we use class
			class := statusClass(rec.status)

			r.HTTPRequestsTotal.WithLabelValues(req.Method, route, class).Inc()
			r.HTTPRequestDuration.WithLabelValues(req.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}
