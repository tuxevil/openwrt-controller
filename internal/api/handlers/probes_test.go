package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// HealthzHandler is the container liveness probe. It must succeed as long
// as the HTTP server is up; it does not depend on the database.
func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	HealthzHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if body := rr.Body.String(); body != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", body)
	}
	if ct := rr.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("expected Content-Type to be set")
	}
}

// ReadyzHandler must return 503 when the DB is not initialised
// (database.DB is nil in the test process) and we expect pingDB to
// return sql.ErrConnDone. We don't try to start a real Postgres here.
func TestReadyzHandler_NotReady(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	ReadyzHandler(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
}

// Compile-time check that the readiness timeout is positive. The
// liveness/readiness contract breaks silently if it is misconfigured.
func TestLivenessProbeTimeoutIsPositive(t *testing.T) {
	if LivenessProbeTimeout <= 0 {
		t.Fatalf("LivenessProbeTimeout must be > 0, got %v", LivenessProbeTimeout)
	}
}

// Make sure pingDB returns a non-nil error when the DB is nil (the
// state of a fresh test process). This is the function that
// ReadyzHandler relies on.
func TestPingDB_NoDB(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pingDB(ctx); err == nil {
		t.Fatalf("expected pingDB to fail when DB is nil")
	}
}
