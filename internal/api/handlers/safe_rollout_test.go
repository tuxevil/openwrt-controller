package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeRolloutHandlerDefaultsToPreview(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/devices/test/safe-rollout?config=system", strings.NewReader(`{
		"commands": [{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"test-router"}]
	}`))
	req.SetPathValue("device_id", "test")
	req.SetPathValue("config", "system")
	rec := httptest.NewRecorder()

	SafeRolloutHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"preview"`) {
		t.Fatalf("response did not remain in preview mode: %s", body)
	}
	if !strings.Contains(body, "confirm=true") {
		t.Fatalf("response did not describe explicit confirmation: %s", body)
	}
}

func TestSafeRolloutHandlerRejectsMixedNamespacesBeforeExecution(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/devices/test/safe-rollout?config=system", strings.NewReader(`{
		"confirm": true,
		"commands": [{"action":"set","config":"network","section":"lan","option":"ipaddr","value":"10.0.0.1"}]
	}`))
	req.SetPathValue("device_id", "test")
	rec := httptest.NewRecorder()

	SafeRolloutHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "backup") {
		t.Fatalf("validation should reject before backup or execution: %s", rec.Body.String())
	}
}
