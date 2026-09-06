package handlers

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestValidateDeviceTelemetryToken(t *testing.T) {
	tests := []struct {
		name     string
		stored   string
		provided string
		valid    bool
	}{
		{name: "first enrollment with no stored token", stored: "", provided: "", valid: true},
		{name: "valid token", stored: "device-token-a", provided: "device-token-a", valid: true},
		{name: "missing token", stored: "device-token-a", provided: "", valid: false},
		{name: "invalid token", stored: "device-token-a", provided: "wrong-token", valid: false},
		{name: "cross device token", stored: "device-token-a", provided: "device-token-b", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeviceTelemetryToken(tt.stored, tt.provided)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%t, error=%v", tt.valid, err)
			}
		})
	}
}

func TestValidateDeviceTelemetryTokenForMode(t *testing.T) {
	tests := []struct {
		name     string
		stored   string
		provided string
		legacy   bool
		valid    bool
	}{
		{name: "strict valid token", stored: "device-token-a", provided: "device-token-a", valid: true},
		{name: "strict missing token", stored: "device-token-a", valid: false},
		{name: "legacy site key fallback", stored: "device-token-a", legacy: true, valid: true},
		{name: "legacy still rejects wrong token", stored: "device-token-a", provided: "wrong-token", legacy: true, valid: false},
		{name: "uninitialized device", legacy: true, valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeviceTelemetryTokenForMode(tt.stored, tt.provided, tt.legacy)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%t, error=%v", tt.valid, err)
			}
		})
	}
}

func TestTelemetryPersistenceContextSurvivesRequestCancellation(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/telemetry", nil)
	requestContext, cancelRequest := context.WithCancel(r.Context())
	r = r.WithContext(requestContext)
	cancelRequest()

	persistenceContext, cancelPersistence := telemetryPersistenceContext(r)
	defer cancelPersistence()
	if err := persistenceContext.Err(); err != nil {
		t.Fatalf("persistence context inherited request cancellation: %v", err)
	}
	if _, ok := persistenceContext.Deadline(); !ok {
		t.Fatal("persistence context must have a bounded deadline")
	}
}
