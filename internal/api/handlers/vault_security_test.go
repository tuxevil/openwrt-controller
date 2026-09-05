package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTriggerSysupgradeHandlerDoesNotExecutePlaceholder(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/devices/device/sysupgrade", nil)
	req.SetPathValue("device_id", "device")
	rec := httptest.NewRecorder()

	TriggerSysupgradeHandler(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}
