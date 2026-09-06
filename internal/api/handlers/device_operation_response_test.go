package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"openwrt-controller/internal/services"
)

func TestWriteQueuedDeviceOperationResponseIncludesGeneration(t *testing.T) {
	rec := httptest.NewRecorder()
	plan := services.DeviceOperationPlan{
		OperationID: "operation-1",
		PlanHash:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Generation:  42,
	}

	writeQueuedDeviceOperationResponse(rec, plan)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	var response struct {
		Status      string `json:"status"`
		OperationID string `json:"operation_id"`
		PlanHash    string `json:"plan_hash"`
		Generation  int64  `json:"generation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "queued" || response.OperationID != plan.OperationID || response.PlanHash != plan.PlanHash || response.Generation != plan.Generation {
		t.Fatalf("response = %#v, want queued operation %#v", response, plan)
	}
}
