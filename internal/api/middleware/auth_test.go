package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
