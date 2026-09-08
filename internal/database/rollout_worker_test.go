package database_test

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
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
