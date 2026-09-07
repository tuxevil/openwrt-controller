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
		t.Skip("set DATABASE_URL_TEST when OPENWRT_INTEGRATION_DB=1")
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

	// The migration system is additive and has no destructive down migration.
	// Starting from this legacy shape verifies backward compatibility, then the
	// second run verifies that the forward migration remains idempotent.
	if err := createLegacyTenantTables(schema); err != nil {
		t.Fatalf("create legacy tenant schema: %v", err)
	}
	legacySiteID := "11111111-1111-1111-1111-111111111111"
	legacyRolloutID := "22222222-2222-2222-2222-222222222222"
	if _, err := DB.Exec(fmt.Sprintf("INSERT INTO %s.sites (id) VALUES ($1)", pgx.Identifier{schema, "sites"}.Sanitize()), legacySiteID); err != nil {
		t.Fatalf("seed legacy site: %v", err)
	}
	if _, err := DB.Exec(fmt.Sprintf(`
		INSERT INTO %s.rollout_runs (id, site_id, generation, status, plan_hash, target_device_ids, results)
		VALUES ($1, $2, 4, 'completed', $3, '[]', '[]')
	`, pgx.Identifier{schema, "rollout_runs"}.Sanitize()), legacyRolloutID, legacySiteID, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("seed legacy rollout: %v", err)
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

	rolloutRows, err := DB.Query(`
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'rollout_runs'
	`, schema)
	if err != nil {
		t.Fatalf("query rollout_runs columns: %v", err)
	}
	defer rolloutRows.Close()
	rolloutColumns := map[string]bool{}
	for rolloutRows.Next() {
		var column string
		if err := rolloutRows.Scan(&column); err != nil {
			t.Fatalf("scan rollout_runs column: %v", err)
		}
		rolloutColumns[column] = true
	}
	if err := rolloutRows.Err(); err != nil {
		t.Fatalf("iterate rollout_runs columns: %v", err)
	}
	for _, column := range []string{"plan", "claim_token"} {
		if !rolloutColumns[column] {
			t.Errorf("rollout_runs is missing immutable rollout column %q", column)
		}
	}
	var migratedPlan []byte
	var migratedGeneration int64
	if err := DB.QueryRow(fmt.Sprintf("SELECT generation, plan FROM %s.rollout_runs WHERE id = $1", pgx.Identifier{schema, "rollout_runs"}.Sanitize()), legacyRolloutID).Scan(&migratedGeneration, &migratedPlan); err != nil {
		t.Fatalf("read migrated rollout: %v", err)
	}
	if migratedGeneration != 4 || string(migratedPlan) != "{}" {
		t.Fatalf("legacy rollout changed during migration: generation=%d plan=%s", migratedGeneration, migratedPlan)
	}

	siteRows, err := DB.Query(`
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'sites'
	`, schema)
	if err != nil {
		t.Fatalf("query sites columns: %v", err)
	}
	defer siteRows.Close()
	siteColumns := map[string]bool{}
	for siteRows.Next() {
		var column string
		if err := siteRows.Scan(&column); err != nil {
			t.Fatalf("scan sites column: %v", err)
		}
		siteColumns[column] = true
	}
	if err := siteRows.Err(); err != nil {
		t.Fatalf("iterate sites columns: %v", err)
	}
	for _, column := range []string{"enrollment_token_hash", "enrollment_token_expires_at", "auto_adopt"} {
		if !siteColumns[column] {
			t.Errorf("sites is missing enrollment contract column %q", column)
		}
	}

	var nonceTableExists bool
	if err := DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = 'device_enrollment_nonces'
		)
	`, schema).Scan(&nonceTableExists); err != nil {
		t.Fatalf("query enrollment nonce table: %v", err)
	}
	if !nonceTableExists {
		t.Fatal("device_enrollment_nonces table was not created")
	}
}

func createLegacyTenantTables(schema string) error {
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s.sites (
			id UUID PRIMARY KEY
		);
		CREATE TABLE %s.rollout_runs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			site_id UUID NOT NULL REFERENCES %s.sites(id),
			generation BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL,
			plan_hash CHAR(64) NOT NULL,
			requested_by VARCHAR(100) NOT NULL DEFAULT '',
			target_device_ids JSONB NOT NULL DEFAULT '[]',
			results JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE %s.site_configs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			site_id UUID UNIQUE,
			enable_global_ssid BOOLEAN DEFAULT true,
			global_ssid VARCHAR(255) DEFAULT '',
			global_wpa_key VARCHAR(255) DEFAULT '',
			global_encryption VARCHAR(50) DEFAULT 'psk2',
			lan_ipaddr VARCHAR(50) DEFAULT '192.168.1.1'
		)`, quoted, quoted, quoted, quoted))
	return err
}
