package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRespondError_LogsInternallyAndReturnsGeneric(t *testing.T) {
	// Capture log output to verify the real error is logged server-side.
	var buf strings.Builder
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	rec := httptest.NewRecorder()
	RespondError(rec, http.StatusBadRequest, "validation failed", errSentinel("database column 'foo' not found"))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if got.Error != "validation failed" {
		t.Errorf("Error = %q, want %q", got.Error, "validation failed")
	}
	// The internal error must NOT leak to the client.
	if strings.Contains(rec.Body.String(), "foo") {
		t.Errorf("internal error leaked to client: %s", rec.Body.String())
	}
	// The internal error must be logged server-side.
	if !strings.Contains(buf.String(), "foo") {
		t.Errorf("internal error not logged: %q", buf.String())
	}
}

func TestRespondError_NilError(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondError(rec, http.StatusInternalServerError, "boom", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	var got ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if got.Error != "boom" {
		t.Errorf("Error = %q, want %q", got.Error, "boom")
	}
}

// errSentinel is a tiny helper that returns a string-shaped error without
// pulling in fmt/errors for this test file.
type stringErr string

func (e stringErr) Error() string { return string(e) }

func errSentinel(s string) error { return stringErr(s) }
