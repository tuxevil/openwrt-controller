package database

import (
	"strings"
	"testing"
)

func TestSafeTenantSchema(t *testing.T) {
	cases := []struct {
		alias   string
		want    string
		wantErr bool
	}{
		{"dragontec", "tenant_dragontec", false},
		{"a_b_c_1", "tenant_a_b_c_1", false},
		// Max alias length is 55 so that "tenant_" (7) + alias fits the
		// 63-byte PostgreSQL identifier limit.
		{strings.Repeat("a", 55), "tenant_" + strings.Repeat("a", 55), false},
		{strings.Repeat("a", 56), "", true},
		{"", "", true},
		{"DROP TABLE x;--", "", true},
		{"with space", "", true},
		{"with-dash", "", true},
		{"1starts_with_digit", "", true}, // valid in PG but we require [a-zA-Z_] start for safety
		{"ok_123", "tenant_ok_123", false},
	}
	for _, c := range cases {
		got, err := SafeTenantSchema(c.alias)
		if (err != nil) != c.wantErr {
			t.Errorf("SafeTenantSchema(%q) err=%v wantErr=%v", c.alias, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("SafeTenantSchema(%q) = %q, want %q", c.alias, got, c.want)
		}
	}
}

func TestSafeSchemaIdent(t *testing.T) {
	cases := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{"public", "public", false},
		{"tenant_dragontec", "tenant_dragontec", false},
		{"tenant_evil; DROP TABLE x;--", "", true},
		{"", "", true},
		{strings.Repeat("a", 64), "", true}, // over PG identifier limit
	}
	for _, c := range cases {
		got, err := SafeSchemaIdent(c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("SafeSchemaIdent(%q) err=%v wantErr=%v", c.name, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("SafeSchemaIdent(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSafeSQLSchemaIdent(t *testing.T) {
	got, err := SafeSQLSchemaIdent("tenant_dragontec")
	if err != nil {
		t.Fatalf("SafeSQLSchemaIdent returned error: %v", err)
	}
	if got != `"tenant_dragontec"` {
		t.Fatalf("SafeSQLSchemaIdent = %q, want quoted identifier", got)
	}
	if _, err := SafeSQLSchemaIdent(`tenant_evil"; DROP TABLE users;--`); err == nil {
		t.Fatal("SafeSQLSchemaIdent accepted SQL syntax")
	}
}

func TestIsValidSchemaName(t *testing.T) {
	if !isValidSchemaName("tenant_dragontec") {
		t.Error("expected tenant_dragontec to be valid")
	}
	if isValidSchemaName("tenant_; DROP") {
		t.Error("expected semicolon-containing name to be invalid")
	}
}
