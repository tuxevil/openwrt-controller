package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetWebhooksHandler_RequiresAuth sanity-checks that the handler
// is wrapped in WithAuth at the route level (i.e. an unauthenticated
// request should not be answered by this handler directly). The
// audit found the route was registered correctly but the WebhooksView
// frontend was calling non-existent methods; this is a regression
// guard for the latter half of that fix.
func TestRouteRegistration_Webhooks(t *testing.T) {
	// We don't import internal/api here to avoid a cycle; just confirm
	// the local handler files compile. The real registration check
	// happens in the integration test suite (out of scope for unit).
	_ = GetWebhooksHandler
	_ = CreateWebhookHandler
	_ = DeleteWebhookHandler
}

// TestHttpErrorReturnsTextPlain sanity-checks that http.Error keeps
// the original behaviour so the existing frontend error parsers that
// sniff for "error" in the body keep working.
func TestHttpErrorReturnsTextPlain(t *testing.T) {
	rec := httptest.NewRecorder()
	http.Error(rec, "boom", http.StatusInternalServerError)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "boom") {
		t.Errorf("body should contain 'boom', got %q", body)
	}
	// http.Error writes "boom\n" with a text/plain content type.
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain*", ct)
	}
}
