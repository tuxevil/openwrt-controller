package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func TestSentinelMCPRevocationAndCaseScope(t *testing.T) {
	const token = "generated-test-value-not-for-production"
	const site = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	const caseID = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"
	path := filepath.Join(t.TempDir(), "identities.json")
	data, err := json.Marshal([]sentinelMCPIdentity{{
		Name: "hermes", TokenSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(token))),
		TenantSchema: "tenant_lab", SiteID: site,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	handler := NewSentinelMCPHandler(path)
	call := func(method, params, credential string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":%s}`, method, params)
		req := httptest.NewRequestWithContext(context.Background(), "POST", "https://localhost/api/sentinel/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-11-25")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if r := call("tools/list", "{}", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous response: %d %s", r.Code, r.Body)
	}
	if r := call("tools/list", "{}", token); r.Code != 200 || !strings.Contains(r.Body.String(), "get_site_health") || !strings.Contains(r.Body.String(), "get_case") {
		t.Fatalf("list response: %d %s", r.Code, r.Body)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; _ = db.Close() })
	mock.ExpectQuery(`WHERE site_id = \$1 AND id = \$2`).WithArgs(site, caseID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "severity", "status", "title", "summary", "updated_at"}))
	params := fmt.Sprintf(`{"name":"get_case","arguments":{"case_id":%q}}`, caseID)
	if r := call("tools/call", params, token); r.Code != 200 || !strings.Contains(r.Body.String(), "case unavailable") {
		t.Fatalf("case response: %d %s", r.Code, r.Body)
	}
	mock.ExpectQuery(`SELECT count\(d.id\).*FROM "tenant_lab".incidents WHERE site_id = \$1.*s.id = \$1`).WithArgs(site).
		WillReturnRows(sqlmock.NewRows([]string{"devices", "fresh", "now", "open", "opened", "resolved"}).
			AddRow(3, 2, time.Now(), 1, 2, 1))
	if r := call("tools/call", `{"name":"get_site_health","arguments":{}}`, token); r.Code != 200 ||
		!strings.Contains(r.Body.String(), `"open_incidents":1`) {
		t.Fatalf("health response: %d %s", r.Code, r.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := call("tools/list", "{}", token); r.Code != http.StatusUnauthorized {
		t.Fatalf("revoked response: %d %s", r.Code, r.Body)
	}
}
