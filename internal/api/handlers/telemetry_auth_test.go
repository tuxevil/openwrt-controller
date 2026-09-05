package handlers

import "testing"

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
