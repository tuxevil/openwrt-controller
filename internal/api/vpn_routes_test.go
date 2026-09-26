package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	jwt "github.com/golang-jwt/jwt/v5"
	"openwrt-controller/internal/database"
)

func TestVPNEndpointRouteChecksCommitBeforeSuccess(t *testing.T) {
	secret := "test-only-vpn-route-signing-key-at-least-32-bytes"
	t.Setenv("JWT_SECRET", secret)
	for _, failCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "commit failure"}[failCommit], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			previous := database.DB
			database.DB = db
			defer func() { database.DB = previous }()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT COUNT").WithArgs("demo").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery("SELECT set_config").WithArgs("tenant_demo, public").WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("tenant_demo, public"))
			mock.ExpectQuery("WITH changed AS .*UPDATE sites.*INSERT INTO audit_logs.*SELECT count").
				WithArgs("vpn.example.test:51820", "site-id", "tester", `{"endpoint":"vpn.example.test:51820"}`, "192.0.2.1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			commit := mock.ExpectCommit()
			if failCommit {
				commit.WillReturnError(errors.New("commit rejected"))
			}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": "tester", "role": "ADMIN", "schema_alias": "demo", "exp": time.Now().Add(time.Minute).Unix(),
			}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/api/sites/site-id/vpn/endpoint", strings.NewReader(`{"endpoint":"vpn.example.test:51820"}`))
			r.RemoteAddr = "192.0.2.1:12345"
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			SetupRoutes().ServeHTTP(w, r)
			want := http.StatusOK
			if failCommit {
				want = http.StatusInternalServerError
			}
			if w.Code != want {
				t.Fatalf("status=%d, want=%d, body=%s", w.Code, want, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVPNEndpointRouteRequiresAdmin(t *testing.T) {
	secret := "test-only-vpn-route-signing-key-at-least-32-bytes"
	t.Setenv("JWT_SECRET", secret)
	for _, role := range []string{"VIEWER", "ADMIN", "SUPERADMIN"} {
		t.Run(role, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			previous := database.DB
			database.DB = db
			defer func() { database.DB = previous }()
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT COUNT").WithArgs("demo").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery("SELECT set_config").WithArgs("tenant_demo, public").WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("tenant_demo, public"))
			mock.ExpectCommit()
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": "tester", "role": role, "schema_alias": "demo", "exp": time.Now().Add(time.Minute).Unix(),
			}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/api/sites/site-id/vpn/endpoint", strings.NewReader(`{"endpoint":"invalid"}`))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			SetupRoutes().ServeHTTP(w, r)
			want := http.StatusBadRequest // Authorized requests reach validation.
			if role == "VIEWER" {
				want = http.StatusForbidden
			}
			if w.Code != want {
				t.Fatalf("status=%d, want=%d, body=%s", w.Code, want, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
