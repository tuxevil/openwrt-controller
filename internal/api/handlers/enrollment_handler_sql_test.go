package handlers

import (
	stdsql "database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func mockEnrollmentHandlerDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previous
		_ = db.Close()
	})
	return mock
}

func expectEnrollmentTokenResolution(mock sqlmock.Sqlmock, token string, found bool) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT schema_alias FROM tenants WHERE is_active = true")).
		WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).AddRow("demo"))
	siteQuery := mock.ExpectQuery(regexp.QuoteMeta(`SELECT id::text FROM "tenant_demo"."sites"`)).
		WithArgs(database.HashDeviceEnrollmentToken(token))
	if found {
		siteQuery.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("11111111-1111-1111-1111-111111111111"))
	} else {
		siteQuery.WillReturnError(stdsql.ErrNoRows)
	}
}

func TestDeviceEnrollmentHandlerCreatesAndRetriesEnrollment(t *testing.T) {
	mock := mockEnrollmentHandlerDB(t)
	const (
		enrollmentToken = "site-enrollment-token"
		siteID          = "11111111-1111-1111-1111-111111111111"
		deviceID        = "ROUTER-01"
		nonce           = "0123456789abcdef0123456789abcdef"
	)
	tokenHash := database.HashDeviceEnrollmentToken(enrollmentToken)
	capabilities := []byte(`{"architecture":"ath79"}`)

	expectEnrollmentTokenResolution(mock, enrollmentToken, true)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, tokenHash, true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_id FROM tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, sqlmock.AnyArg()).
		WillReturnError(stdsql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT site_id::text, device_token FROM tenant_demo.devices")).
		WithArgs(deviceID).
		WillReturnError(stdsql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tenant_demo.devices")).
		WithArgs(deviceID, siteID, sqlmock.AnyArg(), capabilities).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, sqlmock.AnyArg(), deviceID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	first := httptest.NewRequest(http.MethodPost, "/api/device-enrollment", strings.NewReader(`{"device_id":"router-01","nonce":"0123456789abcdef0123456789abcdef","capabilities":{"architecture":"ath79"}}`))
	first.Header.Set("X-Site-Enrollment-Token", enrollmentToken)
	firstResponse := httptest.NewRecorder()
	DeviceEnrollmentHandler(firstResponse, first)
	if firstResponse.Code != http.StatusCreated || !strings.Contains(firstResponse.Body.String(), `"device_id":"ROUTER-01"`) {
		t.Fatalf("first enrollment response: status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}

	expectEnrollmentTokenResolution(mock, enrollmentToken, true)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, tokenHash, true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_id FROM tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}).AddRow(deviceID))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_token, site_id::text FROM tenant_demo.devices")).
		WithArgs(deviceID).
		WillReturnRows(sqlmock.NewRows([]string{"device_token", "site_id"}).AddRow("device-token", siteID))
	mock.ExpectCommit()

	retry := httptest.NewRequest(http.MethodPost, "/api/device-enrollment", strings.NewReader(`{"device_id":"router-01","nonce":"0123456789abcdef0123456789abcdef","capabilities":{"architecture":"ath79"}}`))
	retry.Header.Set("X-Site-Enrollment-Token", enrollmentToken)
	retryResponse := httptest.NewRecorder()
	DeviceEnrollmentHandler(retryResponse, retry)
	if retryResponse.Code != http.StatusOK || !strings.Contains(retryResponse.Body.String(), `"device_token":"device-token"`) {
		t.Fatalf("retry response: status=%d body=%s", retryResponse.Code, retryResponse.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceEnrollmentHandlerRejectsExpiredOrWrongSiteToken(t *testing.T) {
	for _, name := range []string{"expired", "wrong site"} {
		t.Run(name, func(t *testing.T) {
			mock := mockEnrollmentHandlerDB(t)
			expectEnrollmentTokenResolution(mock, "invalid-site-token", false)

			req := httptest.NewRequest(http.MethodPost, "/api/device-enrollment", strings.NewReader(`{"device_id":"ROUTER-01","nonce":"0123456789abcdef","capabilities":{}}`))
			req.Header.Set("X-Site-Enrollment-Token", "invalid-site-token")
			res := httptest.NewRecorder()
			DeviceEnrollmentHandler(res, req)

			if res.Code != http.StatusForbidden {
				t.Fatalf("status=%d, want %d", res.Code, http.StatusForbidden)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeviceEnrollmentHandlerRejectsNonceConflict(t *testing.T) {
	mock := mockEnrollmentHandlerDB(t)
	const enrollmentToken = "site-enrollment-token"
	const siteID = "11111111-1111-1111-1111-111111111111"
	expectEnrollmentTokenResolution(mock, enrollmentToken, true)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(auto_adopt, false), enrollment_token_hash, COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM tenant_demo.sites")).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"auto_adopt", "enrollment_token_hash", "token_valid"}).AddRow(true, database.HashDeviceEnrollmentToken(enrollmentToken), true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT device_id FROM tenant_demo.device_enrollment_nonces")).
		WithArgs(siteID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}).AddRow("OTHER-DEVICE"))
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodPost, "/api/device-enrollment", strings.NewReader(`{"device_id":"ROUTER-01","nonce":"0123456789abcdef","capabilities":{}}`))
	req.Header.Set("X-Site-Enrollment-Token", enrollmentToken)
	res := httptest.NewRecorder()
	DeviceEnrollmentHandler(res, req)

	if res.Code != http.StatusConflict {
		t.Fatalf("status=%d, want %d", res.Code, http.StatusConflict)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryRejectsUnknownDeviceTokenWithoutCreatingDevice(t *testing.T) {
	mock := mockEnrollmentHandlerDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT schema_alias FROM tenants WHERE is_active = true")).
		WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).AddRow("demo"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM "tenant_demo"."devices" WHERE device_token = $1)`)).
		WithArgs("unknown-device-token").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	req := httptest.NewRequest(http.MethodPost, "/api/telemetry", strings.NewReader(`{"device_id":"ROUTER-01"}`))
	req.Header.Set("X-Device-Token", "unknown-device-token")
	res := httptest.NewRecorder()
	TelemetryHandler(res, req)

	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", res.Code, http.StatusForbidden)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
