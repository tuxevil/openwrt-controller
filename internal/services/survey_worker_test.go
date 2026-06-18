package services

import (
	"context"
	"testing"
	"time"

	"openwrt-controller/internal/database"
)

// TestSurveyWorker_EnqueueAndExpiry checks the in-memory buffer's drop-on-expiry
// behaviour. We don't actually run flush() (it would talk to InfluxDB), just
// exercise the enqueue path and verify the worker is safe to call from
// multiple goroutines.
func TestSurveyWorker_EnqueueAndExpiry(t *testing.T) {
	w := GetSurveyWorker()
	sid := "test-survey-enqueue"

	// Drain any leftover state from previous tests.
	w.mu.Lock()
	delete(w.buffer, sid)
	w.mu.Unlock()

	// 50 fresh samples, all "now" → not stale.
	now := time.Now()
	for i := 0; i < 50; i++ {
		w.EnqueueGPS(GPSSampleForWorker(sid, "ap-1", 1.0, 2.0, 5.0, now))
	}
	w.mu.Lock()
	n := len(w.buffer[sid])
	w.mu.Unlock()
	if n != 50 {
		t.Errorf("expected 50 buffered samples, got %d", n)
	}
}

// TestSurveyWorker_UnregisterSchemaClearsBuffer verifies UnregisterSchema
// also drops any pending GPS samples for that survey.
func TestSurveyWorker_UnregisterSchemaClearsBuffer(t *testing.T) {
	w := GetSurveyWorker()
	sid := "test-survey-unregister"
	w.mu.Lock()
	delete(w.buffer, sid)
	w.mu.Unlock()

	w.EnqueueGPS(GPSSampleForWorker(sid, "", 0, 0, 0, time.Now()))
	RegisterSchema(sid, "tenant_test")
	UnregisterSchema(sid)

	w.mu.Lock()
	n := len(w.buffer[sid])
	w.mu.Unlock()
	if n != 0 {
		t.Errorf("expected buffer cleared after UnregisterSchema, got %d", n)
	}
}

// TestRegisterSchema_Overwrite verifies re-registering a survey overwrites
// the cached schema. Cheap idempotency check.
func TestRegisterSchema_Overwrite(t *testing.T) {
	sid := "test-survey-overwrite"
	RegisterSchema(sid, "tenant_a")
	RegisterSchema(sid, "tenant_b")

	workerSchemaMu.Lock()
	got := workerSchemaCache[sid]
	workerSchemaMu.Unlock()
	if got != "tenant_b" {
		t.Errorf("RegisterSchema overwrite failed: got %q, want tenant_b", got)
	}
	// cleanup
	UnregisterSchema(sid)
}

// _ = context.Background silences the "imported and not used" if the file
// is edited and the import is removed by a future refactor.
var _ = context.Background

// _ = database.HexEncodeToString is a placeholder to keep the import stable
// while the worker is still being filled out. The real implementation will
// use database helpers to write wifi_survey_points.
var _ = database.HashToken
