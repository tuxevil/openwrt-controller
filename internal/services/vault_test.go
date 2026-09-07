package services

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func TestCreateBackupForSiteScopesDeviceLookup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(last_ip, '') FROM "tenant_demo".devices WHERE id = $1 AND site_id = $2`)).
		WithArgs("device-1", "site-1").
		WillReturnError(sql.ErrNoRows)

	if err := CreateBackupForSite(context.Background(), "tenant_demo", "site-1", "device-1"); err == nil {
		t.Fatal("backup lookup unexpectedly succeeded for a missing site-scoped device")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackupSSHAddressSupportsIPv6(t *testing.T) {
	if got := backupSSHAddress("2001:db8::10"); got != "[2001:db8::10]:22" {
		t.Fatalf("backup SSH address = %q, want [2001:db8::10]:22", got)
	}
}
