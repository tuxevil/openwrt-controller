package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func TestVPNEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"192.0.2.1:51820", "vpn.example.test:31337", "vpn.example.test.:65535", "gateway:1"} {
		if !validVPNEndpoint(endpoint) {
			t.Errorf("rejected %q", endpoint)
		}
	}
	for _, endpoint := range []string{"", "192.0.2.1", "999.1.1.1:51820", "0.0.0.0:51820", "224.0.0.1:1", "vpn:0", "vpn:65536", "vpn:+80", "vpn: 80", " vpn:80", "vpn:80\n", "a..b:80", "-vpn:80", "vpn-:80", "vpn;reboot:80", "$(reboot):80", "a'b:80", "[2001:db8::1]:51820", "[192.0.2.1]:51820", "[vpn.example.test]:51820"} {
		if validVPNEndpoint(endpoint) {
			t.Errorf("accepted %q", endpoint)
		}
	}
}

func TestVPNEndpointRejectsMalformedPayloadBeforeDatabase(t *testing.T) {
	for _, body := range []string{`{"endpoint":"vpn:80"} {}`, `{"endpoint":"vpn:80","extra":1}`, `{"endpoint":"$(reboot):80"}`, strings.Repeat("x", 4097)} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		UpdateVPNEndpointHandler(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", w.Code)
		}
	}
}

func TestVPNEndpointUpdateAuditsAndReportsMissingSite(t *testing.T) {
	for _, count := range []int{0, 1} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		previous := database.DB
		database.DB = db
		mock.ExpectQuery("WITH changed AS .*UPDATE sites.*INSERT INTO audit_logs.*SELECT count").
			WithArgs("vpn.example.test:51820", "site-id", sqlmock.AnyArg(), `{"endpoint":"vpn.example.test:51820"}`, "2001:db8::1").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"endpoint":"vpn.example.test:51820"}`))
		r.SetPathValue("site_id", "site-id")
		r.RemoteAddr = "[2001:db8::1]:12345"
		w := httptest.NewRecorder()
		UpdateVPNEndpointHandler(w, r)
		database.DB = previous
		db.Close()
		want := http.StatusOK
		if count == 0 {
			want = http.StatusNotFound
		}
		if w.Code != want {
			t.Fatalf("status=%d, want=%d, body=%s", w.Code, want, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVPNMeshResponsesExcludePrivateKey(t *testing.T) {
	node := models.VPNMeshNode{ID: "node", PublicKey: "public-value", PrivateKey: "secret-value"}
	for _, value := range []any{node, publicVPNMeshNode(node), []vpnMeshNodeResponse{publicVPNMeshNode(node)}} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private_key") || strings.Contains(string(encoded), "secret-value") {
			t.Fatal("response serialized a private key")
		}
		if !strings.Contains(string(encoded), "public-value") {
			t.Fatal("public key missing")
		}
	}
}
