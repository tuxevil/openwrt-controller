package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRecordDeviceOperationStatusRejectsLateNonterminalStatus(t *testing.T) {
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" {
		t.Skip("set OPENWRT_INTEGRATION_DB=1 to run PostgreSQL device operation integration tests")
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

	schema := "tenant_operation_" + strings.ToLower(strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", ""))
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	quotedDevices := pgx.Identifier{schema, "devices"}.Sanitize()
	if _, err := DB.Exec("CREATE SCHEMA " + quotedSchema); err != nil {
		t.Fatalf("create temporary schema: %v", err)
	}
	defer DB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE")

	if _, err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s.devices (
			id VARCHAR(50) PRIMARY KEY,
			site_id UUID,
			desired_generation BIGINT NOT NULL DEFAULT 0,
			observed_generation BIGINT NOT NULL DEFAULT 0,
			last_successful_generation BIGINT NOT NULL DEFAULT 0,
			pending_operation JSONB,
			last_operation JSONB,
			pending_change_set JSONB,
			last_change_set JSONB,
			last_rollout_status VARCHAR(32),
			last_rollout_at TIMESTAMP WITH TIME ZONE,
			last_health_check_at TIMESTAMP WITH TIME ZONE,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, quotedSchema)); err != nil {
		t.Fatalf("create temporary devices table: %v", err)
	}
	if _, err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s.rollout_runs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			site_id UUID,
			status VARCHAR(32) NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, quotedSchema)); err != nil {
		t.Fatalf("create temporary rollout table: %v", err)
	}

	terminalStatus := `{"id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"state":"COMMITTED"}`
	if _, err := DB.Exec(fmt.Sprintf(`
		INSERT INTO %s (id, desired_generation, last_operation, last_rollout_status)
		VALUES ($1, $2, $3::jsonb, $4)`, quotedDevices), "device-1", int64(42), terminalStatus, "RUNNING"); err != nil {
		t.Fatalf("seed terminal operation: %v", err)
	}

	lateStatus := `{"id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","generation":42,"state":"APPLYING"}`
	err = RecordDeviceOperationStatus(context.Background(), schema, "device-1", []byte(lateStatus))
	if !errors.Is(err, ErrDeviceOperationStatusNotApplied) {
		t.Fatalf("late status error = %v, want %v", err, ErrDeviceOperationStatusNotApplied)
	}

	var stored []byte
	var rolloutStatus string
	if err := DB.QueryRow(fmt.Sprintf("SELECT last_operation, last_rollout_status FROM %s", quotedDevices)).Scan(&stored, &rolloutStatus); err != nil {
		t.Fatalf("read terminal operation: %v", err)
	}
	var gotStatus, wantStatus map[string]interface{}
	if err := json.Unmarshal(stored, &gotStatus); err != nil {
		t.Fatalf("decode stored terminal operation: %v", err)
	}
	if err := json.Unmarshal([]byte(terminalStatus), &wantStatus); err != nil {
		t.Fatalf("decode expected terminal operation: %v", err)
	}
	if !reflect.DeepEqual(gotStatus, wantStatus) {
		t.Fatalf("last operation = %s, want terminal status", stored)
	}
	if rolloutStatus != "RUNNING" {
		t.Fatalf("last rollout status = %q, want RUNNING", rolloutStatus)
	}
}

func TestQueueDeviceOperationAcceptsBoundRetryAfterLegacyStatus(t *testing.T) {
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" {
		t.Skip("set OPENWRT_INTEGRATION_DB=1 to run PostgreSQL device operation integration tests")
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

	schema := "tenant_retry_" + strings.ToLower(strings.ReplaceAll(fmt.Sprint(time.Now().UnixNano()), "-", ""))
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	quotedDevices := pgx.Identifier{schema, "devices"}.Sanitize()
	if _, err := DB.Exec("CREATE SCHEMA " + quotedSchema); err != nil {
		t.Fatalf("create temporary schema: %v", err)
	}
	defer DB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE")
	if _, err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s.devices (
			id VARCHAR(50) PRIMARY KEY,
			site_id UUID,
			desired_generation BIGINT NOT NULL DEFAULT 0,
			observed_generation BIGINT NOT NULL DEFAULT 0,
			last_successful_generation BIGINT NOT NULL DEFAULT 0,
			pending_operation JSONB,
			last_operation JSONB,
			pending_change_set JSONB,
			last_change_set JSONB,
			last_rollout_status VARCHAR(32),
			last_rollout_at TIMESTAMP WITH TIME ZONE,
			last_health_check_at TIMESTAMP WITH TIME ZONE,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, quotedSchema)); err != nil {
		t.Fatalf("create temporary devices table: %v", err)
	}
	if _, err := DB.Exec(fmt.Sprintf(`
		CREATE TABLE %s.rollout_runs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			site_id UUID,
			status VARCHAR(32) NOT NULL,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, quotedSchema)); err != nil {
		t.Fatalf("create temporary rollout table: %v", err)
	}

	planHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	legacyStatus := `{"id":"operation-1","plan_hash":"` + planHash + `","state":"COMMITTED"}`
	plan := `{"operation_id":"operation-1","plan_hash":"` + planHash + `","generation":42}`
	if _, err := DB.Exec(fmt.Sprintf(`
		INSERT INTO %s (id, desired_generation, pending_operation)
		VALUES ($1, $2, $3::jsonb)`, quotedDevices), "device-1", int64(42), plan); err != nil {
		t.Fatalf("seed pending operation: %v", err)
	}
	staleStatus := `{"id":"operation-1","plan_hash":"` + planHash + `","generation":41,"state":"COMMITTED"}`
	if err := RecordDeviceOperationStatus(context.Background(), schema, "device-1", []byte(staleStatus)); !errors.Is(err, ErrDeviceOperationStatusNotApplied) {
		t.Fatalf("stale status error = %v, want %v", err, ErrDeviceOperationStatusNotApplied)
	}
	if err := RecordDeviceOperationStatus(context.Background(), schema, "device-1", []byte(legacyStatus)); err != nil {
		t.Fatalf("legacy status error = %v", err)
	}

	generation, err := QueueDeviceOperation(context.Background(), schema, "device-1", []byte(plan))
	if err != nil {
		t.Fatalf("bound retry error = %v", err)
	}
	if generation != 42 {
		t.Fatalf("bound retry generation = %d, want 42", generation)
	}
}
