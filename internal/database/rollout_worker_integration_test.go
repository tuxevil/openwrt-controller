package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"openwrt-controller/internal/database"
)

func TestRolloutWorkerReclaimsExpiredLeaseAfterRestart(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" || dsn == "" {
		t.Skip("set OPENWRT_INTEGRATION_DB=1 and DATABASE_URL_TEST to run rollout worker integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	schema := "tenant_worker_" + strings.TrimPrefix(fmt.Sprint(time.Now().UnixNano()), "-")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	quotedRollouts := pgx.Identifier{schema, "rollout_runs"}.Sanitize()
	if _, err := db.Exec("CREATE SCHEMA " + quotedSchema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + quotedSchema + " CASCADE")
	if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE %s (
		id UUID PRIMARY KEY, site_id UUID NOT NULL, generation BIGINT NOT NULL,
		status VARCHAR(32) NOT NULL, worker_token UUID,
		worker_lease_until TIMESTAMP WITH TIME ZONE, worker_cursor INT NOT NULL DEFAULT 0,
		plan JSONB NOT NULL DEFAULT '{}', results JSONB NOT NULL DEFAULT '[]',
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	)`, quotedRollouts)); err != nil {
		t.Fatal(err)
	}
	rolloutID := "00000000-0000-0000-0000-000000000001"
	siteID := "00000000-0000-0000-0000-000000000002"
	if _, err := db.Exec(fmt.Sprintf("INSERT INTO %s (id, site_id, generation, status, worker_lease_until, worker_cursor) VALUES ($1, $2, 1, 'RUNNING', CURRENT_TIMESTAMP - INTERVAL '1 minute', 1)", quotedRollouts), rolloutID, siteID); err != nil {
		t.Fatal(err)
	}

	lease, err := database.ClaimQueuedRollout(context.Background(), schema, 30*time.Second)
	if err != nil {
		t.Fatalf("reclaim expired rollout: %v", err)
	}
	if lease.RolloutID != rolloutID || lease.SiteID != siteID || lease.Cursor != 1 || lease.Token == "" {
		t.Fatalf("unexpected reclaimed lease: %#v", lease)
	}
	if err := database.RenewRolloutLease(context.Background(), schema, lease, 30*time.Second); err != nil {
		t.Fatalf("renew reclaimed lease: %v", err)
	}
}
