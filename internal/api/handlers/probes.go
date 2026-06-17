package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
)

// LivenessProbeTimeout is the per-attempt timeout for the readiness
// probe when pinging the database.
const LivenessProbeTimeout = 2 * time.Second

// HealthzHandler is the container liveness probe. It returns 200 OK as
// long as the HTTP server is up and the goroutine scheduler is
// responsive. It does NOT touch the database — that is what /readyz is
// for. This endpoint is intentionally anonymous and unauthenticated so
// the orchestrator can poll it cheaply.
func HealthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// ReadyzHandler is the container readiness probe. It returns 200 only
// when the application is fully initialised AND can reach the
// database. Use this as the readiness gate in Kubernetes / Docker
// Swarm / Coolify. Anonymous for the same reason as /healthz.
func ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), LivenessProbeTimeout)
	defer cancel()

	if err := pingDB(ctx); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "not-ready",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func pingDB(ctx context.Context) error {
	if database.DB == nil {
		return sql.ErrConnDone
	}
	return database.DB.PingContext(ctx)
}
