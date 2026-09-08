package database_test

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func setupRolloutWorkerSQLMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	return db, mock, func() {
		database.DB = previousDB
		_ = db.Close()
	}
}

func TestClaimQueuedRolloutReturnsFencedLease(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WithArgs(float64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "site_id", "worker_token", "worker_cursor"}).
			AddRow("rollout-1", "site-1", "worker-token", 2))
	_ = db

	lease, err := database.ClaimQueuedRollout(context.Background(), "tenant_demo", 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimQueuedRollout returned error: %v", err)
	}
	if lease.RolloutID != "rollout-1" || lease.SiteID != "site-1" || lease.Token != "worker-token" || lease.Cursor != 2 {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActiveTenantSchemasRejectsInvalidAliases(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT schema_alias FROM tenants WHERE is_active = true")).
		WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).
			AddRow("tenant_demo").AddRow("tenant-invalid!").AddRow("tenant_two"))

	schemas, err := database.ActiveTenantSchemas(context.Background())
	if err != nil {
		t.Fatalf("ActiveTenantSchemas returned error: %v", err)
	}
	if got, want := strings.Join(schemas, ","), "tenant_tenant_demo,tenant_tenant_two"; got != want {
		t.Fatalf("schemas = %q, want %q", got, want)
	}
	_ = db
}

func TestGetRolloutProgressCountsTerminalResults(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, plan, results FROM tenant_demo.rollout_runs WHERE id = $1 AND site_id = $2")).
		WithArgs("rollout-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "plan", "results"}).AddRow("RUNNING", []byte(`{}`), []byte(`[
			{"status":"QUEUED"},
			{"status":"SUCCESS"},
			{"status":"QUEUED","change_set_state":"COMMITTED"}
		]`)))

	progress, err := database.GetRolloutProgress(context.Background(), "tenant_demo", "rollout-1", "site-1")
	if err != nil {
		t.Fatalf("GetRolloutProgress returned error: %v", err)
	}
	if progress.Status != "RUNNING" || progress.ResultCount != 3 || progress.TerminalCount != 2 || progress.FailureCount != 0 {
		t.Fatalf("progress = %#v, want running/3/2/0", progress)
	}
	_ = db
}

func TestGetRolloutProgressCountsTerminalFailure(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status, plan, results FROM tenant_demo.rollout_runs WHERE id = $1 AND site_id = $2")).
		WithArgs("rollout-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "plan", "results"}).AddRow("QUEUED", []byte(`{}`), []byte(`[{"status":"FAILED","change_set_state":"REJECTED"}]`)))

	progress, err := database.GetRolloutProgress(context.Background(), "tenant_demo", "rollout-1", "site-1")
	if err != nil {
		t.Fatalf("GetRolloutProgress returned error: %v", err)
	}
	if progress.TerminalCount != 1 || progress.FailureCount != 1 {
		t.Fatalf("progress = %#v, want one terminal failure", progress)
	}
	_ = db
}

func TestUpdateRolloutWorkerCursorUsesLeaseFence(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WithArgs(2, "rollout-1", "site-1", "worker-token").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := database.UpdateRolloutWorkerCursor(context.Background(), "tenant_demo", database.RolloutLease{
		RolloutID: "rollout-1", SiteID: "site-1", Token: "worker-token",
	}, 2)
	if err != nil {
		t.Fatalf("UpdateRolloutWorkerCursor returned error: %v", err)
	}
	_ = db
}

func TestClaimQueuedRolloutReportsUnavailableQueue(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WithArgs(float64(30)).WillReturnError(sqlmock.ErrCancelled)

	_, err := database.ClaimQueuedRollout(context.Background(), "tenant_demo", 30*time.Second)
	if !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatalf("error = %v, want database error", err)
	}
	_ = db
}

func TestRenewRolloutLeaseRejectsLostFence(t *testing.T) {
	db, mock, cleanup := setupRolloutWorkerSQLMock(t)
	defer cleanup()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.rollout_runs")).
		WithArgs(float64(30), "rollout-1", "site-1", "worker-token").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := database.RenewRolloutLease(context.Background(), "tenant_demo", database.RolloutLease{
		RolloutID: "rollout-1", SiteID: "site-1", Token: "worker-token",
	}, 30*time.Second)
	if !errors.Is(err, database.ErrRolloutLeaseLost) {
		t.Fatalf("error = %v, want ErrRolloutLeaseLost", err)
	}
	_ = db
}
