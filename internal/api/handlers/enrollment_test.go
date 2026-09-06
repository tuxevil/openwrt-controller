package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeEnrollmentDeviceID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "canonicalizes agent MAC", input: " 04:a1:51:96:a6:4d ", want: "04:A1:51:96:A6:4D"},
		{name: "accepts controller identity", input: "router-01", want: "ROUTER-01"},
		{name: "rejects empty", input: "", wantErr: true},
		{name: "rejects shell punctuation", input: "router;rm -rf /", wantErr: true},
		{name: "rejects oversized identity", input: strings.Repeat("a", 51), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeEnrollmentDeviceID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v, wantErr=%t", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDeviceEnrollmentRequest(t *testing.T) {
	req, err := parseDeviceEnrollmentRequest(strings.NewReader(`{
		"device_id":"04:a1:51:96:a6:4d",
		"nonce":"0123456789abcdef0123456789abcdef",
		"capabilities":{"architecture":"ath79","sqm":false}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "04:A1:51:96:A6:4D" {
		t.Fatalf("device id was not canonicalized: %q", req.DeviceID)
	}
	if string(req.Capabilities) != `{"architecture":"ath79","sqm":false}` {
		t.Fatalf("unexpected capabilities: %s", req.Capabilities)
	}
}

func TestParseDeviceEnrollmentRequestRejectsReplayAmbiguity(t *testing.T) {
	for _, payload := range []string{
		`{"device_id":"router-01","nonce":"short","capabilities":{}}`,
		`{"device_id":"router-01","nonce":"0123456789abcdef0123456789abcdef","capabilities":[]}`,
		`{"device_id":"router-01","nonce":"0123456789abcdef0123456789abcdef","capabilities":{}} trailing`,
	} {
		if _, err := parseDeviceEnrollmentRequest(strings.NewReader(payload)); err == nil {
			t.Fatalf("payload was accepted: %s", payload)
		}
	}
}

func TestDeviceEnrollmentHandlerRequiresEnrollmentToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/device-enrollment", strings.NewReader(`{"device_id":"router-01","nonce":"0123456789abcdef","capabilities":{}}`))
	res := httptest.NewRecorder()

	DeviceEnrollmentHandler(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", res.Code, http.StatusUnauthorized)
	}
}

func TestTelemetryAndConfigRejectSiteKeyWithoutDeviceToken(t *testing.T) {
	t.Setenv("ALLOW_LEGACY_PROVISION", "false")

	telemetryReq := httptest.NewRequest(http.MethodPost, "/api/telemetry", strings.NewReader(`{"device_id":"ROUTER-01"}`))
	telemetryReq.Header.Set("X-Site-Key", "site-key")
	telemetryRes := httptest.NewRecorder()
	TelemetryHandler(telemetryRes, telemetryReq)
	if telemetryRes.Code != http.StatusForbidden {
		t.Fatalf("telemetry status=%d, want %d", telemetryRes.Code, http.StatusForbidden)
	}

	configReq := httptest.NewRequest(http.MethodGet, "/api/devices/ROUTER-01/config", nil)
	configReq.SetPathValue("device_id", "ROUTER-01")
	configReq.Header.Set("X-Site-Key", "site-key")
	configRes := httptest.NewRecorder()
	GetDeviceConfigHandler(configRes, configReq)
	if configRes.Code != http.StatusUnauthorized {
		t.Fatalf("config status=%d, want %d", configRes.Code, http.StatusUnauthorized)
	}
}

func TestConfigRejectsSiteEnrollmentTokenAfterEnrollment(t *testing.T) {
	t.Setenv("ALLOW_LEGACY_PROVISION", "false")
	req := httptest.NewRequest(http.MethodGet, "/api/devices/ROUTER-01/config", nil)
	req.SetPathValue("device_id", "ROUTER-01")
	req.Header.Set("X-Site-Enrollment-Token", "site-enrollment-token")
	res := httptest.NewRecorder()

	GetDeviceConfigHandler(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("config status=%d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestThreatListRejectsSiteEnrollmentTokenAfterEnrollment(t *testing.T) {
	t.Setenv("ALLOW_LEGACY_PROVISION", "false")
	req := httptest.NewRequest(http.MethodGet, "/api/threat-shield/list", nil)
	req.Header.Set("X-Site-Enrollment-Token", "site-enrollment-token")
	res := httptest.NewRecorder()

	GetThreatShieldListHandler(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("threat list status=%d, want %d", res.Code, http.StatusForbidden)
	}
}
