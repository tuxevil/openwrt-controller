package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"openwrt-controller/internal/database"
)

func TestGetSiteTailscaleHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	prevDB := database.DB
	database.DB = db
	defer func() { database.DB = prevDB }()

	stateJSON := `{
		"capabilities": {"packages": ["wireguard", "tailscale"]},
		"tailscale": {
			"backend_state": "Running",
			"dns_name": "mr8300.ts.net",
			"online": true,
			"exit_node": false,
			"tailscale_ips": ["100.64.0.1", "fd7a:115c:a1e0::1"],
			"rx_bytes": 1024,
			"tx_bytes": 2048
		}
	}`

	mock.ExpectQuery(`SELECT id, COALESCE\(name, id\), COALESCE\(model, 'Unknown'\), state_json, COALESCE\(last_seen_at::text, ''\) FROM devices WHERE site_id = \$1`).
		WithArgs("site-123").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "model", "state_json", "last_seen_at"}).
			AddRow("dev-1", "Gateway", "Linksys MR8300", stateJSON, "2026-11-26 12:00:00"))

	r := httptest.NewRequest(http.MethodGet, "/api/sites/site-123/tailscale", nil)
	r.SetPathValue("site_id", "site-123")
	w := httptest.NewRecorder()

	GetSiteTailscaleHandler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var results []TailscaleDeviceStatus
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	dev := results[0]
	if !dev.Installed || dev.BackendState != "Running" || dev.DNSName != "mr8300.ts.net" {
		t.Errorf("unexpected status parsed: %+v", dev)
	}
	if len(dev.TailscaleIPs) != 2 || dev.TailscaleIPs[0] != "100.64.0.1" {
		t.Errorf("unexpected tailscale IPs: %+v", dev.TailscaleIPs)
	}
}

func TestGenerateTailscalePlanHandler_Validation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	prevDB := database.DB
	database.DB = db
	defer func() { database.DB = prevDB }()

	// Test 1: Invalid route CIDR
	{
		r := httptest.NewRequest(http.MethodPost, "/api/devices/dev-1/tailscale/plan", strings.NewReader(`{
			"auth_key": "tskey-auth-valid",
			"advertise_routes": ["invalid-cidr"]
		}`))
		r.SetPathValue("device_id", "dev-1")
		w := httptest.NewRecorder()

		GenerateTailscalePlanHandler(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid CIDR, got %d", w.Code)
		}
	}

	// Test 2: Shell injection in auth_key
	{
		r := httptest.NewRequest(http.MethodPost, "/api/devices/dev-1/tailscale/plan", strings.NewReader(`{
			"auth_key": "tskey; rm -rf /",
			"advertise_routes": ["10.128.128.0/24"]
		}`))
		r.SetPathValue("device_id", "dev-1")
		w := httptest.NewRecorder()

		GenerateTailscalePlanHandler(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for malicious auth_key, got %d", w.Code)
		}
	}

	// Test 3: Successful generation with watchdog TTL and audit log
	{
		mock.ExpectExec(`INSERT INTO audit_logs`).
			WithArgs(sqlmock.AnyArg(), "TAILSCALE_PLAN_GENERATED", "DEVICE", "dev-1", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		r := httptest.NewRequest(http.MethodPost, "/api/devices/dev-1/tailscale/plan", strings.NewReader(`{
			"auth_key": "tskey-auth-k1234567890",
			"advertise_routes": ["10.128.128.0/24"],
			"is_exit_node": false,
			"accept_dns": false,
			"watchdog_ttl": 90
		}`))
		r.SetPathValue("device_id", "dev-1")
		r.RemoteAddr = "10.128.128.6:12345"
		w := httptest.NewRecorder()

		GenerateTailscalePlanHandler(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}

		var resp TailscalePlanResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.WatchdogTTL != 90 {
			t.Errorf("expected watchdog TTL 90, got %d", resp.WatchdogTTL)
		}
		if len(resp.UCICommands) == 0 {
			t.Errorf("expected non-empty UCI commands")
		}
		if !strings.Contains(resp.TailscaleCmd, "--advertise-routes=10.128.128.0/24") {
			t.Errorf("expected advertised route in tailscale command, got %s", resp.TailscaleCmd)
		}
		if !strings.Contains(resp.TailscaleCmd, "--accept-dns=false") {
			t.Errorf("expected --accept-dns=false in tailscale command, got %s", resp.TailscaleCmd)
		}
	}
}
