package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	jwt "github.com/golang-jwt/jwt/v5"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/authtickets"
	"openwrt-controller/internal/database"
)

// TestIssueWSTicketHandler_ValidToken verifies the endpoint returns a ticket
// when called with a valid JWT in the Authorization header. The ticket
// store is loaded by `go test` via TestMain's JWT_SECRET; the handler
// calls getJWTSecret() which reads the env.
func TestIssueWSTicketHandler_ValidToken(t *testing.T) {
	// Ensure the ticket store is initialised (the handler calls
	// authtickets.GetStore(), which is nil unless LoadStore has been
	// called). We load it here because no test binary calls main().
	authtickets.LoadStore(30 * time.Second)

	// Mint a valid JWT.
	claims := jwt.MapClaims{
		"sub":          "testuser",
		"role":         "ADMIN",
		"schema_alias": "demo",
		"exp":          time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(getJWTSecret())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tenants WHERE schema_alias = $1 AND is_active = true")).
		WithArgs("demo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT set_config('search_path', $1, true)")).
		WithArgs("tenant_demo, public").
		WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("tenant_demo, public"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS(SELECT 1 FROM tenant_demo.devices WHERE id = $1)")).
		WithArgs("test-device").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ws-ticket", strings.NewReader(`{"device_id":"test-device"}`))
	r.Header.Set("Authorization", "Bearer "+signed)
	middleware.WithAuth(IssueWSTicketHandler).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	var resp WsTicketResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TokenType != "Ticket" {
		t.Errorf("TokenType = %q, want 'Ticket'", resp.TokenType)
	}
	if len(resp.Ticket) != 32 {
		t.Errorf("ticket length = %d, want 32", len(resp.Ticket))
	}
	if resp.ExpiresIn <= 0 || resp.ExpiresIn > 35 {
		t.Errorf("ExpiresIn = %d, want 1-30", resp.ExpiresIn)
	}
	ticket, err := authtickets.GetStore().Validate(resp.Ticket)
	if err != nil {
		t.Fatalf("validate issued ticket: %v", err)
	}
	if ticket.DeviceID != "test-device" || ticket.TenantSchema != "demo" {
		t.Fatalf("ticket scope = %+v, want device=test-device schema=demo", ticket)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestIssueWSTicketHandler_MissingToken verifies 401 when no Auth header is
// present.
func TestIssueWSTicketHandler_MissingToken(t *testing.T) {
	authtickets.LoadStore(30 * time.Second)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/ws-ticket", strings.NewReader(`{}`))
	IssueWSTicketHandler(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "missing bearer token") {
		t.Errorf("body should mention 'missing bearer token', got %q", body)
	}
}
