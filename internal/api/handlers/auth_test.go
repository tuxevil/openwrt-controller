package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
