package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"openwrt-controller/internal/database"
)

// TestLoginHandler_RejectsInvalidJSON exercises the request decoding
// path without touching the database (the DB call comes after JSON
// validation so a 400 should fire first).
func TestLoginHandler_RejectsInvalidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("not json"))
	LoginHandler(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid json") {
		t.Errorf("body should mention 'invalid json', got %q", rec.Body.String())
	}
}

// TestLoginHandler_RespectsCanceledContext is the regression for the
// Jun 17 2026 incident: when Postgres went unhealthy, the login
// handler blocked indefinitely on QueryRow because nothing bounded
// the request. With an already-canceled context the handler must
// return promptly (a 401 or 503) instead of hanging.
//
// We use a context that's ALREADY canceled so the database/sql driver
// fails the QueryRow immediately, surfacing the timeout behavior of
// the handler without depending on a live DB.
//
// This test requires a real DB pool to exercise the cancellation
// path — when database.DB is nil the handler panics in the
// database/sql driver before any of our code runs. We skip it
// when the test env didn't init a DB (e.g. the basic `go test ./...`
// run with no docker-compose). The CI pipeline that wires the test
// DB should set DATABASE_URL before invoking `go test`.
func TestLoginHandler_RespectsCanceledContext(t *testing.T) {
	if database.DB == nil {
		t.Skip("database.DB is nil — set DATABASE_URL in the test env to exercise the cancellation path")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate a client disconnect / DB-timeout propagation

	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"admin","password":"admin"}`))

	done := make(chan struct{})
	var rec *httptest.ResponseRecorder
	go func() {
		rec = httptest.NewRecorder()
		LoginHandler(rec, r)
		close(done)
	}()

	select {
	case <-done:
		// got a response in time
	case <-time.After(2 * time.Second):
		t.Fatal("LoginHandler hung for 2s with a canceled context (regression: no per-request DB timeout)")
	}

	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusUnauthorized && rec.Code != http.StatusInternalServerError && rec.Code != http.StatusBadGateway {
		t.Errorf("expected 503/401/500/502 for canceled context, got %d", rec.Code)
	}
}

// TestGetUsernameFromReq_FallsBackToSystem verifies that a request with
// no Authorization header and no token query returns "system" rather
// than panicking on a nil map claims.
func TestGetUsernameFromReq_FallsBackToSystem(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	if got := GetUsernameFromReq(r); got != "system" {
		t.Errorf("GetUsernameFromReq on no-auth request = %q, want 'system'", got)
	}
}

// TestGetUsernameFromReq_ValidBearerToken verifies that a signed JWT
// with a "sub" claim yields that subject.
func TestGetUsernameFromReq_ValidBearerToken(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	signed, err := tok.SignedString(getJWTSecret())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	if got := GetUsernameFromReq(r); got != "alice@example.com" {
		t.Errorf("GetUsernameFromReq = %q, want alice@example.com", got)
	}
}

// TestGetUsernameFromReq_InvalidBearerToken verifies that a garbage
// token yields "system" (the audit-log fallback) rather than panicking
// or returning an empty string.
func TestGetUsernameFromReq_InvalidBearerToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	r.Header.Set("Authorization", "Bearer this-is-not-a-jwt")
	if got := GetUsernameFromReq(r); got != "system" {
		t.Errorf("GetUsernameFromReq on garbage token = %q, want 'system'", got)
	}
}
