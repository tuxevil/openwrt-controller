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
				q := string(body)
				if strings.Contains(q, "client_signal") {
					for _, want := range []string{mac, surveyID, ts.Add(-10 * time.Second).Format(time.RFC3339Nano), ts.Add(10 * time.Second).Format(time.RFC3339Nano), "distance_ns"} {
						if !strings.Contains(q, want) {
							t.Errorf("Influx query missing %q", want)
						}
					}
					if matched {
						w.Header().Set("Content-Type", "text/csv")
						fmt.Fprintf(w, "#datatype,string,long,dateTime:RFC3339,string,double,double\n,result,table,_time,device_id,signal_dbm,noise_dbm\n,,0,%s,ap-2,-69,-94\n", ts.Format(time.RFC3339Nano))
					}
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
					"token_revoked_at", "token_rotated_at", "created_by", "survey_token_hash", "ap_positions_snapshot"}).
					AddRow(surveyID, "site", "Walk", "active", "public", nil, nil, nil, nil, mac, nil, "already used", nil, nil, nil, nil, nil, database.HashToken(token), "[]"))
			mock.ExpectQuery("allow_public_surveys").WithArgs("site").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
			mock.ExpectQuery("surveys_public_lockdown").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
			if !matched {
				mock.ExpectQuery("FROM tenant_demo.devices").WithArgs("site").WillReturnRows(
					sqlmock.NewRows([]string{"id", "name", "last_ip"}).
						AddRow("ap-1", "MR8300", "10.128.128.1").
						AddRow("ap-2", "UAP-Mesh", "10.128.128.2"),
				)
			}
			insert := mock.ExpectExec("INSERT INTO tenant_demo.wifi_survey_points")
			if matched {
				insert.WithArgs(
					surveyID, "ap-2", -1.0, -79.0, 5.0, -69.0, -94.0, 25.0, nil, "[]", ts.Format(time.RFC3339Nano),
					nil, "", nil, nil, "[]", nil, nil, nil,
				)
			} else {
				insert.WithArgs(
					surveyID, "", -1.0, -79.0, 5.0, nil, nil, nil, nil, "[]", ts.Format(time.RFC3339Nano),
					nil, "no_ap_telemetry", 0, 2, `["MR8300 (10.128.128.1)","UAP-Mesh (10.128.128.2)"]`, nil, nil, nil,
				)
			}
			insert.WillReturnResult(sqlmock.NewResult(1, 1))
			req := httptest.NewRequest(http.MethodPost, "/api/surveys/"+surveyID+"/samples",
				strings.NewReader(fmt.Sprintf(`{"lat":-1,"lon":-79,"accuracy_m":5,"ts":%d}`, ts.UnixMilli())))
			req.SetPathValue("id", surveyID)
			req.Header.Set("X-Survey-Token", token)
			w := httptest.NewRecorder()
			PostSurveySampleHandler(w, req)
			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != http.StatusCreated || response["matched"] != matched {
				t.Fatalf("unexpected response %d: %s", w.Code, w.Body.String())
			}
			if matched {
				if response["ap_id"] != "ap-2" || response["signal_dbm"] != float64(-69) || response["snr"] != float64(25) {
					t.Fatalf("expected live signal metadata in response, got %v", response)
				}
			}
			if !matched && response["missing_reason"] != "no_ap_telemetry" {
				t.Fatalf("expected missing_reason=no_ap_telemetry, got %v", response["missing_reason"])
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSurveySampleDiagnosesMissingSignalAndDeduplicates(t *testing.T) {
	cases := []struct {
		name           string
		reportingAPs   []string
		wantReason     string
		wantReporting  int
		wantTotal      int
		wantSilentJSON string
		rowsAffected   int64
		wantStatus     int
		wantDuplicate  bool
	}{
		{
			name:           "all APs reporting means client not associated",
			reportingAPs:   []string{"ap-1", "ap-2", "ap-3"},
			wantReason:     "client_not_associated",
			wantReporting:  3,
			wantTotal:      3,
			wantSilentJSON: "[]",
			rowsAffected:   1,
			wantStatus:     http.StatusCreated,
		},
		{
			name:           "one AP silent means partial telemetry",
			reportingAPs:   []string{"ap-1", "ap-2"},
			wantReason:     "partial_ap_telemetry",
			wantReporting:  2,
			wantTotal:      3,
			wantSilentJSON: `["WNDR3800 (10.128.128.3)"]`,
			rowsAffected:   1,
			wantStatus:     http.StatusCreated,
		},
		{
			name:           "duplicate retry returns 200 idempotently",
			reportingAPs:   []string{"ap-1", "ap-2", "ap-3"},
			wantReason:     "client_not_associated",
			wantReporting:  3,
			wantTotal:      3,
			wantSilentJSON: "[]",
			rowsAffected:   0,
			wantStatus:     http.StatusOK,
			wantDuplicate:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const mac = "02:11:22:33:44:55"
			const surveyID = "00000000-0000-4000-8000-000000000010"
			const token = "test-qr-token"
			ts := time.UnixMilli(time.Now().Add(-time.Minute).UnixMilli()).UTC()

			influx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				q := string(body)
				if strings.Contains(q, `device_metrics`) {
					w.Header().Set("Content-Type", "text/csv")
					fmt.Fprint(w, "#datatype,string,long,string\n,result,table,device_id\n")
					for _, ap := range tc.reportingAPs {
						fmt.Fprintf(w, ",,0,%s\n", ap)
					}
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
			cacheMu.Lock()
			delete(surveyLookupCache, surveyID)
			cacheMu.Unlock()

			mock.ExpectQuery("SELECT schema_alias FROM tenants").WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).AddRow("demo"))
			mock.ExpectQuery("survey_token_hash.*FROM tenant_demo.wifi_surveys").WithArgs(surveyID).WillReturnRows(
				sqlmock.NewRows([]string{"id", "site_id", "name", "status", "access_mode", "started_at", "ended_at", "created_at", "updated_at",
					"surveyor_mac", "surveyor_label", "token_first_used_at", "token_first_ip", "token_first_ua",
					"token_revoked_at", "token_rotated_at", "created_by", "survey_token_hash", "ap_positions_snapshot"}).
					AddRow(surveyID, "site", "Walk", "active", "public", nil, nil, nil, nil, mac, nil, "already used", nil, nil, nil, nil, nil, database.HashToken(token), "[]"))
			mock.ExpectQuery("allow_public_surveys").WithArgs("site").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
			mock.ExpectQuery("surveys_public_lockdown").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
			mock.ExpectQuery("FROM tenant_demo.devices").WithArgs("site").WillReturnRows(
				sqlmock.NewRows([]string{"id", "name", "last_ip"}).
					AddRow("ap-1", "MR8300", "10.128.128.1").
					AddRow("ap-2", "UAP-Mesh", "10.128.128.2").
					AddRow("ap-3", "WNDR3800", "10.128.128.3"),
			)

			sampleID := "sample-123"
			rtt := float32(18.5)
			jitter := float32(4.2)
			loss := float32(0)
			mock.ExpectExec("INSERT INTO tenant_demo.wifi_survey_points").WithArgs(
				surveyID, "", -1.0, -79.0, float32(6.0), nil, nil, nil, nil, "[]", ts.Format(time.RFC3339Nano),
				&sampleID, tc.wantReason, &tc.wantReporting, &tc.wantTotal, tc.wantSilentJSON,
				&rtt, &jitter, &loss,
			).WillReturnResult(sqlmock.NewResult(1, tc.rowsAffected))

			payload := fmt.Sprintf(
				`{"lat":-1,"lon":-79,"accuracy_m":6,"ts":%d,"client_sample_id":"%s","probe_rtt_ms":18.5,"probe_jitter_ms":4.2,"probe_loss_pct":0}`,
				ts.UnixMilli(), sampleID,
			)
			req := httptest.NewRequest(http.MethodPost, "/api/surveys/"+surveyID+"/samples", strings.NewReader(payload))
			req.SetPathValue("id", surveyID)
			req.Header.Set("X-Survey-Token", token)
			w := httptest.NewRecorder()
			PostSurveySampleHandler(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("bad json: %v", err)
			}
			if resp["missing_reason"] != tc.wantReason {
				t.Fatalf("missing_reason = %v, want %q", resp["missing_reason"], tc.wantReason)
			}
			if resp["duplicate"] != tc.wantDuplicate {
				t.Fatalf("duplicate = %v, want %t", resp["duplicate"], tc.wantDuplicate)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateSurveyFloorplan(t *testing.T) {
	valid := database.SurveyFloorplan{
		ImageDataURL: "data:image/png;base64,iVBORw0KGgo=",
		Filename:     "rancho.png",
		Bounds:       [2][2]float64{{-2.001, -78.961}, {-1.999, -78.959}},
		Opacity:      0.7,
	}
	if err := validateSurveyFloorplan(valid); err != nil {
		t.Fatalf("expected valid floorplan, got %v", err)
	}

	svg := valid
	svg.ImageDataURL = "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="
	if err := validateSurveyFloorplan(svg); err == nil {
		t.Fatal("expected SVG floorplan to be rejected")
	}

	badBounds := valid
	badBounds.Bounds = [2][2]float64{{-2.0, -78.9}, {-2.0, -78.9}}
	if err := validateSurveyFloorplan(badBounds); err == nil {
		t.Fatal("expected degenerate bounds to be rejected")
	}
}

func TestPublicSurveyStatusAndPing(t *testing.T) {
	const mac = "02:11:22:33:44:55"
	const surveyID = "00000000-0000-4000-8000-000000000020"
	const token = "test-status-token"

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB := database.DB
	database.DB = db
	defer func() { database.DB = oldDB }()
	cacheMu.Lock()
	delete(surveyLookupCache, surveyID)
	cacheMu.Unlock()

	// 1) Status endpoint
	mock.ExpectQuery("SELECT schema_alias FROM tenants").WillReturnRows(sqlmock.NewRows([]string{"schema_alias"}).AddRow("demo"))
	mock.ExpectQuery("survey_token_hash.*FROM tenant_demo.wifi_surveys").WithArgs(surveyID).WillReturnRows(
		sqlmock.NewRows([]string{"id", "site_id", "name", "status", "access_mode", "started_at", "ended_at", "created_at", "updated_at",
			"surveyor_mac", "surveyor_label", "token_first_used_at", "token_first_ip", "token_first_ua",
			"token_revoked_at", "token_rotated_at", "created_by", "survey_token_hash", "ap_positions_snapshot"}).
			AddRow(surveyID, "site", "Walk", "active", "public", nil, nil, nil, nil, mac, nil, "used", nil, nil, nil, nil, nil, database.HashToken(token), `[{"id":"ap-1","label":"MR8300","lat":-2.0237,"lon":-78.9474}]`))
	mock.ExpectQuery("allow_public_surveys").WithArgs("site").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
	mock.ExpectQuery("surveys_public_lockdown").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))
	mock.ExpectQuery("FROM tenant_demo.devices").WithArgs("site").WillReturnRows(
		sqlmock.NewRows([]string{"id", "name", "last_ip", "age", "state_json"}).
			AddRow("ap-1", "MR8300", "10.128.128.1", 5, []byte(`{"wireless_stations":{"phy0":[{"mac":"02:11:22:33:44:55","signal":-61}]}}`)).
			AddRow("ap-3", "WNDR3800", "10.128.128.3", 120, []byte(`{}`)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/surveys/"+surveyID+"/status", nil)
	req.SetPathValue("id", surveyID)
	req.Header.Set("X-Survey-Token", token)
	w := httptest.NewRecorder()
	GetPublicSurveyStatusHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", w.Code, w.Body.String())
	}
	var statusResp struct {
		ReportingCount int      `json:"reporting_count"`
		TotalCount     int      `json:"total_count"`
		SurveyorSeen   bool     `json:"surveyor_seen"`
		SurveyorAP     string   `json:"surveyor_ap"`
		SurveyorDBM    *float64 `json:"surveyor_dbm"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &statusResp); err != nil {
		t.Fatal(err)
	}
	if statusResp.ReportingCount != 1 || statusResp.TotalCount != 2 || !statusResp.SurveyorSeen || statusResp.SurveyorDBM == nil || *statusResp.SurveyorDBM != -61 {
		t.Fatalf("unexpected status response: %+v", statusResp)
	}

	// 2) Ping endpoint (hits lookup cache for schema, re-fetches survey)
	mock.ExpectQuery("survey_token_hash.*FROM tenant_demo.wifi_surveys").WithArgs(surveyID).WillReturnRows(
		sqlmock.NewRows([]string{"id", "site_id", "name", "status", "access_mode", "started_at", "ended_at", "created_at", "updated_at",
			"surveyor_mac", "surveyor_label", "token_first_used_at", "token_first_ip", "token_first_ua",
			"token_revoked_at", "token_rotated_at", "created_by", "survey_token_hash", "ap_positions_snapshot"}).
			AddRow(surveyID, "site", "Walk", "active", "public", nil, nil, nil, nil, mac, nil, "used", nil, nil, nil, nil, nil, database.HashToken(token), `[{"id":"ap-1","label":"MR8300","lat":-2.0237,"lon":-78.9474}]`))
	mock.ExpectQuery("allow_public_surveys").WithArgs("site").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
	mock.ExpectQuery("surveys_public_lockdown").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(false))

	pingReq := httptest.NewRequest(http.MethodGet, "/api/surveys/"+surveyID+"/ping", nil)
	pingReq.SetPathValue("id", surveyID)
	pingReq.Header.Set("X-Survey-Token", token)
	pingW := httptest.NewRecorder()
	PublicSurveyPingHandler(pingW, pingReq)

	if pingW.Code != http.StatusOK || pingW.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected ping response: %d cc=%q body=%s", pingW.Code, pingW.Header().Get("Cache-Control"), pingW.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
