package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestGetTenantSchemaIgnoresUnresolvedHeader(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/devices", nil)
	r.Header.Set("X-Tenant-Schema", `victim"; DROP SCHEMA public; --`)

	schema, err := getTenantSchema(r)
	if err != nil {
		t.Fatalf("getTenantSchema returned an error for an unresolved request: %v", err)
	}
	if schema != "public" {
		t.Fatalf("getTenantSchema trusted request header: got %q", schema)
	}
}
