package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSiteConfigMigrationContract(t *testing.T) {
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" {
		t.Skip("set OPENWRT_INTEGRATION_DB=1 to run PostgreSQL migration integration tests")
	}
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Fatal("DATABASE_URL_TEST is required when OPENWRT_INTEGRATION_DB=1")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	previousDB := DB
	DB = db
	defer func() { DB = previousDB }()

	schema := "tenant_migration_" + strings.ToLower(strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", ""))
	if _, err := DB.Exec(fmt.Sprintf("CREATE SCHEMA %s", pgx.Identifier{schema}.Sanitize())); err != nil {
		t.Fatalf("create temporary schema: %v", err)
	}
	defer DB.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", pgx.Identifier{schema}.Sanitize()))

	if err := createLegacyTenantTables(schema); err != nil {
		t.Fatalf("create legacy tenant schema: %v", err)
	}
	if err := createTenantTables(schema); err != nil {
		t.Fatalf("legacy tenant migration: %v", err)
	}
	if err := createTenantTables(schema); err != nil {
		t.Fatalf("legacy tenant migration was not idempotent: %v", err)
	}

	rows, err := DB.Query(`
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'site_configs'
	`, schema)
	if err != nil {
		t.Fatalf("query site_configs columns: %v", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan site_configs column: %v", err)
		}
		columns[column] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate site_configs columns: %v", err)
	}

	for _, column := range []string{
		"sqm_cake_enabled", "sqm_download", "sqm_upload", "dpi_enabled",
		"secure_tunnel_enabled", "tailscale_enabled", "tailscale_auth_key",
		"topology_metadata", "benchmark_baseline", "health_checks",
		"wan_interfaces", "allow_public_surveys",
	} {
		if !columns[column] {
			t.Errorf("site_configs is missing contract column %q", column)
		}
	}
}

func createLegacyTenantTables(schema string) error {
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s.site_configs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			site_id UUID UNIQUE,
			enable_global_ssid BOOLEAN DEFAULT true,
			global_ssid VARCHAR(255) DEFAULT '',
			global_wpa_key VARCHAR(255) DEFAULT '',
			global_encryption VARCHAR(50) DEFAULT 'psk2',
			lan_ipaddr VARCHAR(50) DEFAULT '192.168.1.1'
		)`, quoted))
	return err
}
