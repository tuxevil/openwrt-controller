package handlers

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"openwrt-controller/internal/database"

	"github.com/DATA-DOG/go-sqlmock"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

// TestSurveyBaseURLOverride ensures the SURVEY_PUBLIC_URL env var
// forces the QR base URL to a known value, ignoring whatever the
// inbound Host header says. This is the escape hatch used when the
// controller is reached through a reverse proxy (e.g. Coolify) that
// injects scripts or serves a hostname the phone can't reach.
func TestSurveyBaseURLOverride(t *testing.T) {
	const override = "https://10.128.128.6:3000"
	t.Setenv("SURVEY_PUBLIC_URL", override)
	defer os.Unsetenv("SURVEY_PUBLIC_URL")

	tests := []struct {
		name   string
		host   string
		tls    bool
		xfp    string
		expect string
	}{
		{"override beats plain host", "controller.local:3000", false, "", override},
		{"override beats xfp", "controller.local", true, "https", override},
		{"override beats loopback host", "localhost:3000", false, "", override},
		{"override beats another port", "1.2.3.4:8443", true, "https", override},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://"+c.host+"/x", nil)
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if c.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", c.xfp)
			}
			got := surveyBaseURL(r)
			if got != c.expect {
				t.Errorf("surveyBaseURL() = %q, want %q", got, c.expect)
			}
		})
	}
}

// TestSurveyBaseURLFallback verifies the default path: when the env
// var is unset, the function falls back to r.Host + scheme, honouring
// X-Forwarded-Proto for TLS-terminating proxies.
func TestSurveyBaseURLFallback(t *testing.T) {
	os.Unsetenv("SURVEY_PUBLIC_URL")
	tests := []struct {
		name   string
		host   string
		tls    bool
		xfp    string
		expect string
	}{
		{"plain http", "10.0.0.1:3000", false, "", "http://10.0.0.1:3000"},
		{"https direct", "10.0.0.1:3000", true, "", "https://10.0.0.1:3000"},
		{"http + xfp https", "controller.example", false, "https", "https://controller.example"},
		{"https + xfp http (xfp wins)", "controller.example", true, "http", "http://controller.example"},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			scheme := "https"
			if !c.tls && c.xfp == "" {
				scheme = "http"
			}
			r := httptest.NewRequest("GET", scheme+"://"+c.host+"/x", nil)
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if c.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", c.xfp)
			}
			got := surveyBaseURL(r)
			if !strings.HasPrefix(got, c.expect) {
				t.Errorf("surveyBaseURL() = %q, want prefix %q", got, c.expect)
			}
		})
	}
}

func TestSurveySamplePersistsGPSWithAndWithoutSignal(t *testing.T) {
	for _, matched := range []bool{true, false} {
		t.Run(fmt.Sprintf("matched=%t", matched), func(t *testing.T) {
			const mac = "02:11:22:33:44:55"
			surveyID := "00000000-0000-4000-8000-000000000001"
			if !matched {
				surveyID = "00000000-0000-4000-8000-000000000002"
			}
			const token = "test-qr-token"
			ts := time.UnixMilli(time.Now().Add(-time.Minute).UnixMilli()).UTC()
			influx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				for _, want := range []string{mac, surveyID, ts.Add(-10 * time.Second).Format(time.RFC3339Nano), ts.Add(10 * time.Second).Format(time.RFC3339Nano), "distance_ns"} {
					if !strings.Contains(string(body), want) {
						t.Errorf("Influx query missing %q", want)
					}
				}
				if matched {
					w.Header().Set("Content-Type", "text/csv")
					fmt.Fprintf(w, "#datatype,string,long,dateTime:RFC3339,string,double,double\n,result,table,_time,device_id,signal_dbm,noise_dbm\n,,0,%s,ap-2,-69,-94\n", ts.Format(time.RFC3339Nano))
				}
			}))
			defer influx.Close()
			client := influxdb2.NewClient(influx.URL, "test")
			defer client.Close()
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			oldDB, oldInflux := database.DB, database.InfluxClient
			database.DB, database.InfluxClient = db, client
			defer func() { database.DB, database.InfluxClient = oldDB, oldInflux }()
			mock.ExpectQuery("SELECT schema_alias FROM tenants").WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).AddRow("demo"))
			mock.ExpectQuery("survey_token_hash.*FROM tenant_demo.wifi_surveys").WithArgs(surveyID).WillReturnRows(
				sqlmock.NewRows([]string{"id", "site_id", "name", "status", "access_mode", "started_at", "ended_at", "created_at", "updated_at",
					"surveyor_mac", "surveyor_label", "token_first_used_at", "token_first_ip", "token_first_ua",
					"token_revoked_at", "token_rotated_at", "created_by", "survey_token_hash"}).
					AddRow(surveyID, "site", "Walk", "active", "public", nil, nil, nil, nil, mac, nil, "already used", nil, nil, nil, nil, nil, database.HashToken(token)))
			mock.ExpectQuery("allow_public_surveys").WithArgs("site").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
			mock.ExpectQuery("surveys_public_lockdown").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
			insert := mock.ExpectExec("INSERT INTO tenant_demo.wifi_survey_points")
			if matched {
				insert.WithArgs(surveyID, "ap-2", -1.0, -79.0, 5.0, -69.0, -94.0, 25.0, nil, "[]", ts.Format(time.RFC3339Nano))
			} else {
				insert.WithArgs(surveyID, "", -1.0, -79.0, 5.0, nil, nil, nil, nil, "[]", ts.Format(time.RFC3339Nano))
			}
			insert.WillReturnResult(sqlmock.NewResult(1, 1))
			req := httptest.NewRequest(http.MethodPost, "/api/surveys/"+surveyID+"/samples",
				strings.NewReader(fmt.Sprintf(`{"lat":-1,"lon":-79,"accuracy_m":5,"ts":%d}`, ts.UnixMilli())))
			req.SetPathValue("id", surveyID)
			req.Header.Set("X-Survey-Token", token)
			w := httptest.NewRecorder()
			PostSurveySampleHandler(w, req)
			var response map[string]bool
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != http.StatusCreated || response["matched"] != matched {
				t.Fatalf("unexpected response %d: %s", w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
