package handlers

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func TestGetDeviceIPForSiteScopesDeviceToSite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_ip FROM "tenant_demo".devices WHERE id = $1 AND site_id = $2`)).
		WithArgs("device-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"last_ip"}).AddRow("192.0.2.10"))

	got, err := getDeviceIPForSite(context.Background(), "tenant_demo", "site-1", "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "192.0.2.10" {
		t.Fatalf("device IP = %q, want 192.0.2.10", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetDeviceIPForTenantRejectsMissingIP(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_ip FROM "tenant_demo".devices WHERE id = $1`)).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"last_ip"}).AddRow(nil))

	if _, err := getDeviceIPForTenant(context.Background(), "tenant_demo", "device-1"); err == nil {
		t.Fatal("missing device IP was accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
