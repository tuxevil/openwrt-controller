package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
)

func signOperatorJWT(t *testing.T) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username":     "admin",
		"role":         "OPERATOR",
		"tenant_id":    "demo",
		"schema_alias": "demo",
		"exp":          time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString(getJWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestValidateDeviceLocation(t *testing.T) {
	lat := -2.023712
	lon := -78.947345
	if err := validateDeviceLocation(&lat, &lon, "Techo oeste"); err != nil {
		t.Fatalf("expected valid location, got %v", err)
	}
	if err := validateDeviceLocation(nil, nil, ""); err != nil {
		t.Fatalf("expected clear location to be valid, got %v", err)
	}
	if err := validateDeviceLocation(&lat, nil, ""); err == nil {
		t.Fatal("expected partial coords to fail")
	}
	badLat := 95.0
	if err := validateDeviceLocation(&badLat, &lon, ""); err == nil {
		t.Fatal("expected out-of-range lat to fail")
	}
}

func TestUpdateDeviceLocationHandlerEndToEnd(t *testing.T) {
	token := signOperatorJWT(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB := database.DB
	database.DB = db
	defer func() { database.DB = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM tenants WHERE schema_alias = $1 AND is_active = true")).
		WithArgs("demo").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT set_config('search_path', $1, true)")).
		WithArgs("tenant_demo, public").
		WillReturnRows(sqlmock.NewRows([]string{"set_config"}).AddRow("tenant_demo, public"))

	lat := -2.023712
	lon := -78.947345
	mock.ExpectExec("UPDATE tenant_demo.devices").
		WithArgs(&lat, &lon, "Poste sur 3m", "78:8a:20:2c:c1:12", "site-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPut, "/api/sites/site-1/devices/78:8a:20:2c:c1:12/location",
		strings.NewReader(`{"lat":-2.023712,"lon":-78.947345,"note":"Poste sur 3m"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.SetPathValue("site_id", "site-1")
	req.SetPathValue("device_id", "78:8a:20:2c:c1:12")
	w := httptest.NewRecorder()
	middleware.WithAuth(UpdateDeviceLocationHandler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			ID           string   `json:"id"`
			LocationLat  *float64 `json:"location_lat"`
			LocationLon  *float64 `json:"location_lon"`
			LocationNote string   `json:"location_note"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.LocationLat == nil || *resp.Data.LocationLat != lat || resp.Data.LocationNote != "Poste sur 3m" {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
