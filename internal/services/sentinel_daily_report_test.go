package services

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"openwrt-controller/internal/database"
)

func TestSentinelDailyReportPostgres(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if os.Getenv("OPENWRT_INTEGRATION_DB") != "1" || dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB := database.DB
	database.DB = db
	defer func() { database.DB = oldDB }()
	schema := "tenant_daily_" + time.Now().UTC().Format("20060102150405000000000")
	if err := database.RunTenantMigrations(schema); err != nil {
		t.Fatal(err)
	}
	quoted, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + quoted + " CASCADE")
	if _, err := db.Exec("INSERT INTO " + quoted + ".incidents (incident_type, severity, created_at) VALUES ('NODE_DOWN', 'HIGH', (date_trunc('day', CURRENT_TIMESTAMP AT TIME ZONE 'UTC') - INTERVAL '1 hour') AT TIME ZONE 'UTC')"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := generateSentinelDailyReport(schema); err != nil {
			t.Fatal(err)
		}
	}
	var rows, opened int
	if err := db.QueryRow("SELECT count(*), COALESCE(sum(opened),0) FROM "+quoted+".sentinel_daily_reports").Scan(&rows, &opened); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || opened != 1 {
		t.Fatalf("expected one report with one incident, got reports=%d opened=%d", rows, opened)
	}
}
