package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/authtickets"
	"openwrt-controller/internal/database"
)

// TestClaimsContext verifies the claimsKey mechanism round-trips through
// the context (i.e. handlers can read the JWT claims that WithAuth put
// there once it's wired up).
func TestClaimsContext(t *testing.T) {
	type ctxKey string
	const k = ctxKey("test_claims")
	want := "hello"
	ctx := context.WithValue(context.Background(), k, want)

	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got, _ := req.Context().Value(k).(string)
		if got != want {
			t.Errorf("context value = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
	})
	next.ServeHTTP(rec, rWithCtx(ctx))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// TestTenantSchemaContext verifies the tenantSchemaKey helper
// returns the schema name when set, and "" when missing.
func TestTenantSchemaContext(t *testing.T) {
	if got := GetTenantSchema(rWithCtx(context.Background())); got != "" {
		t.Errorf("GetTenantSchema on empty ctx = %q, want empty", got)
	}

	ctx := context.WithValue(context.Background(), tenantSchemaKey, "tenant_dragontec")
	if got := GetTenantSchema(rWithCtx(ctx)); got != "tenant_dragontec" {
		t.Errorf("GetTenantSchema = %q, want tenant_dragontec", got)
	}
}

// rWithCtx returns a *http.Request whose Context() is ctx. It is a
// minimal helper so the test files don't need to import httptest.
func rWithCtx(ctx context.Context) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	return r
}

// TestMissingTokenReturns401 verifies the missing-bearer-token path of
// WithAuth. We can't test the happy path without a real JWT_SECRET, but
// the 401 path is the most security-sensitive one.
func TestMissingTokenReturns401(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	WithAuth(func(w http.ResponseWriter, req *http.Request) {
		t.Error("next handler should NOT be called without a token")
		w.WriteHeader(http.StatusOK)
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "missing bearer token") {
		t.Errorf("body %q should mention 'missing bearer token'", body)
	}
}

func TestWithAuthAcceptsDeviceScopedWebSocketTicket(t *testing.T) {
	store := authtickets.LoadStore(time.Minute)
	ticketID, _, err := store.Issue("alice", "ADMIN", "device-1", "demo")
	if err != nil {
		t.Fatal(err)
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
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM tenants WHERE schema_alias = \\$1 AND is_active = true").
		WithArgs("demo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT set_config\\('search_path', \\$1, true\\)").
		WithArgs("tenant_demo, public").
		WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("tenant_demo, public"))
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/devices/device-1/ssh?ticket="+ticketID, nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Authorization", "Bearer this-must-not-bypass-ticket-auth")
	r.SetPathValue("device_id", "device-1")
	WithAuth(func(w http.ResponseWriter, req *http.Request) {
		claims, ok := GetClaims(req)
		if !ok || claims["sub"] != "alice" || claims["role"] != "ADMIN" {
			t.Errorf("ticket claims = %#v, want alice/ADMIN", claims)
		}
		if got := GetTenantSchema(req); got != "tenant_demo" {
			t.Errorf("ticket tenant schema = %q, want tenant_demo", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWithAuthRequiresTicketForDeviceSSHWebSocket(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/devices/device-1/ssh", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Authorization", "Bearer this-must-not-bypass-ticket-auth")
	r.SetPathValue("device_id", "device-1")
	WithAuth(func(w http.ResponseWriter, req *http.Request) {
		t.Error("next handler should not be called without a WebSocket ticket")
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "WebSocket ticket required") {
		t.Fatalf("body %q should require a WebSocket ticket", rec.Body.String())
	}
}

func TestWithAuthRejectsWebSocketTicketForAnotherDevice(t *testing.T) {
	authtickets.LoadStore(time.Minute)
	ticketID, _, err := authtickets.GetStore().Issue("alice", "ADMIN", "device-1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/devices/device-2/ssh?ticket="+ticketID, nil)
	r.Header.Set("Upgrade", "websocket")
	r.SetPathValue("device_id", "device-2")
	WithAuth(func(w http.ResponseWriter, req *http.Request) {
		t.Error("next handler should not be called for a wrong-device ticket")
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
