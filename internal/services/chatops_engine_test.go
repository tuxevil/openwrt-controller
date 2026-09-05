package services

import (
	"strings"
	"testing"
)

func TestQualifyChatOpsQueryUsesTenantSchema(t *testing.T) {
	query, err := qualifyChatOpsQuery("tenant_dragontec", "SELECT id FROM devices ORDER BY last_seen_at DESC")
	if err != nil {
		t.Fatalf("qualifyChatOpsQuery returned error: %v", err)
	}
	if !strings.Contains(query, "FROM tenant_dragontec.devices") {
		t.Fatalf("query is not tenant-qualified: %s", query)
	}
}

func TestQualifyChatOpsQueryRejectsUntrustedSchema(t *testing.T) {
	if _, err := qualifyChatOpsQuery(`tenant_ok; DROP SCHEMA public;--`, "SELECT id FROM devices"); err == nil {
		t.Fatal("expected invalid tenant schema to be rejected")
	}
}
