package database

import (
	stdsql "database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func mockEnrollmentDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = db
	t.Cleanup(func() {
		DB = previous
		_ = db.Close()
	})
	return mock
}

func TestIssueDeviceEnrollmentTokenRotatesExistingSiteToken(t *testing.T) {
	mock := mockEnrollmentDB(t)
	siteID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM tenant_demo.sites WHERE id = $1)")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.sites SET enrollment_token_hash = $1, enrollment_token_expires_at = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), siteID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := IssueDeviceEnrollmentToken(t.Context(), "tenant_demo", siteID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Token) != 64 || time.Until(result.ExpiresAt) <= 0 {
		t.Fatalf("unexpected enrollment token: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeDeviceEnrollmentTokenClearsExistingToken(t *testing.T) {
	mock := mockEnrollmentDB(t)
	siteID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.sites SET enrollment_token_hash = NULL, enrollment_token_expires_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1")).
		WithArgs(siteID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := RevokeDeviceEnrollmentToken(t.Context(), "tenant_demo", siteID); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeDeviceEnrollmentTokenReportsMissingSite(t *testing.T) {
	mock := mockEnrollmentDB(t)
	siteID := "11111111-1111-1111-1111-111111111111"

	mock.ExpectExec(regexp.QuoteMeta("UPDATE tenant_demo.sites SET enrollment_token_hash = NULL, enrollment_token_expires_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1")).
		WithArgs(siteID).
		WillReturnResult(sqlmock.NewResult(1, 0))

	if err := RevokeDeviceEnrollmentToken(t.Context(), "tenant_demo", siteID); err != ErrEnrollmentSiteNotFound {
		t.Fatalf("error=%v, want %v", err, ErrEnrollmentSiteNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDeviceEnrollmentTokenFindsOnlyActiveSiteToken(t *testing.T) {
	mock := mockEnrollmentDB(t)
	const token = "site-enrollment-token"

	mock.ExpectQuery(regexp.QuoteMeta("SELECT schema_alias FROM tenants WHERE is_active = true")).
		WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).AddRow("demo"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text FROM "tenant_demo"."sites"`)).
		WithArgs(HashDeviceEnrollmentToken(token)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("11111111-1111-1111-1111-111111111111"))

	schema, siteID, err := ResolveDeviceEnrollmentToken(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if schema != "tenant_demo" || siteID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("resolved %q/%q", schema, siteID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDeviceEnrollmentTokenRejectsMissingToken(t *testing.T) {
	mock := mockEnrollmentDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT schema_alias FROM tenants WHERE is_active = true")).
		WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}))

	_, _, err := ResolveDeviceEnrollmentToken(t.Context(), "expired-or-revoked")
	if err != ErrEnrollmentTokenNotFound {
		t.Fatalf("error=%v, want %v", err, ErrEnrollmentTokenNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollDeviceCreatesIdempotentEnrollmentRecord(t *testing.T) {
	mock := mockEnrollmentDB(t)
	schema := "tenant_demo"
	siteID := "11111111-1111-1111-1111-111111111111"
	deviceID := "04:A1:51:96:A6:4D"
	tokenHash := HashDeviceEnrollmentToken("site-enrollment-token")
	nonce := "0123456789abcdef0123456789abcdef"
	nonceHash := hashEnrollmentNonce(nonce)
	capabilities := []byte(`{"architecture":"ath79"}`)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, tokenHash, true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_id FROM tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, nonceHash).
		WillReturnError(stdsql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, device_token FROM tenant_demo.devices")).
		WithArgs(deviceID).
		WillReturnError(stdsql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tenant_demo.devices")).
		WithArgs(deviceID, siteID, sqlmock.AnyArg(), capabilities).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, nonceHash, deviceID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	result, err := EnrollDevice(t.Context(), schema, siteID, tokenHash, deviceID, nonce, capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.DeviceID != deviceID || result.SiteID != siteID || len(result.Token) != 64 {
		t.Fatalf("unexpected enrollment result: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollDeviceRetriesSameNonceWithoutIssuingAnotherToken(t *testing.T) {
	mock := mockEnrollmentDB(t)
	schema := "tenant_demo"
	siteID := "11111111-1111-1111-1111-111111111111"
	deviceID := "04:A1:51:96:A6:4D"
	tokenHash := HashDeviceEnrollmentToken("site-enrollment-token")
	nonceHash := hashEnrollmentNonce("0123456789abcdef0123456789abcdef")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, tokenHash, true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_id FROM tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, nonceHash).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}).AddRow(deviceID))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_token, site_id::text FROM tenant_demo.devices")).
		WithArgs(deviceID).
		WillReturnRows(sqlmock.NewRows([]string{"device_token", "site_id"}).AddRow("device-token", siteID))
	mock.ExpectCommit()

	result, err := EnrollDevice(t.Context(), schema, siteID, tokenHash, deviceID, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created || result.Token != "device-token" {
		t.Fatalf("retry issued a new enrollment: %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollDeviceRejectsNonceForAnotherDevice(t *testing.T) {
	mock := mockEnrollmentDB(t)
	schema := "tenant_demo"
	siteID := "11111111-1111-1111-1111-111111111111"
	tokenHash := HashDeviceEnrollmentToken("site-enrollment-token")
	nonceHash := hashEnrollmentNonce("0123456789abcdef0123456789abcdef")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, tokenHash, true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_id FROM tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, nonceHash).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}).AddRow("OTHER-DEVICE"))
	mock.ExpectRollback()

	_, err := EnrollDevice(t.Context(), schema, siteID, tokenHash, "04:A1:51:96:A6:4D", "0123456789abcdef0123456789abcdef", nil)
	if err != ErrEnrollmentNonceConflict {
		t.Fatalf("error=%v, want %v", err, ErrEnrollmentNonceConflict)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollDeviceRejectsDisabledSite(t *testing.T) {
	mock := mockEnrollmentDB(t)
	siteID := "11111111-1111-1111-1111-111111111111"
	tokenHash := HashDeviceEnrollmentToken("site-enrollment-token")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(false, tokenHash, true))
	mock.ExpectRollback()

	_, err := EnrollDevice(t.Context(), "tenant_demo", siteID, tokenHash, "ROUTER-01", "0123456789abcdef0123456789abcdef", nil)
	if err != ErrEnrollmentDisabled {
		t.Fatalf("error=%v, want %v", err, ErrEnrollmentDisabled)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollDeviceRejectsExpiredToken(t *testing.T) {
	mock := mockEnrollmentDB(t)
	siteID := "11111111-1111-1111-1111-111111111111"
	tokenHash := HashDeviceEnrollmentToken("site-enrollment-token")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, tokenHash, false))
	mock.ExpectRollback()

	_, err := EnrollDevice(t.Context(), "tenant_demo", siteID, tokenHash, "ROUTER-01", "0123456789abcdef0123456789abcdef", nil)
	if err != ErrEnrollmentTokenNotFound {
		t.Fatalf("error=%v, want %v", err, ErrEnrollmentTokenNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollDeviceRejectsTokenForAnotherSite(t *testing.T) {
	mock := mockEnrollmentDB(t)
	siteID := "11111111-1111-1111-1111-111111111111"
	tokenHash := HashDeviceEnrollmentToken("site-enrollment-token")
	otherTokenHash := HashDeviceEnrollmentToken("another-site-token")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, otherTokenHash, true))
	mock.ExpectRollback()

	_, err := EnrollDevice(t.Context(), "tenant_demo", siteID, tokenHash, "ROUTER-01", "0123456789abcdef0123456789abcdef", nil)
	if err != ErrEnrollmentTokenNotFound {
		t.Fatalf("error=%v, want %v", err, ErrEnrollmentTokenNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
