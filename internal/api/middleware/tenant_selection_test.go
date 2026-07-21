package middleware

import (
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
)

func TestTenantHeaderRequiresSuperAdmin(t *testing.T) {
	if tenantHeaderAllowed(jwt.MapClaims{"role": "ADMIN"}) {
		t.Fatal("ADMIN must not select an arbitrary tenant with X-Tenant-Schema")
	}
	if tenantHeaderAllowed(jwt.MapClaims{"role": "USER"}) {
		t.Fatal("USER must not select an arbitrary tenant with X-Tenant-Schema")
	}
	if !tenantHeaderAllowed(jwt.MapClaims{"role": "SUPERADMIN"}) {
		t.Fatal("SUPERADMIN should be allowed to select a tenant")
	}
}
