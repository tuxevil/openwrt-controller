package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"openwrt-controller/internal/database"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetSiteTopologyHandler_ConnectsWANOnlyToGateway(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	prevDB := database.DB
	database.DB = db
	defer func() { database.DB = prevDB }()

	siteID := "site-1"
	metaJSON := `{
		"wan": {"id": "starlink", "name": "Starlink Mini", "address": "192.168.100.1", "link_label": "WAN", "speed": "120M Sat"},
		"roles": {
			"aa:bb:cc:dd:ee:01": "Gateway",
			"aa:bb:cc:dd:ee:02": "Distribution AP",
			"aa:bb:cc:dd:ee:03": "Distribution AP"
		}
	}`

	mock.ExpectQuery(`SELECT COALESCE\(topology_metadata`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"topology_metadata"}).AddRow([]byte(metaJSON)))

	mock.ExpectQuery(`SELECT id, state_json`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_json", "device_role"}).
			AddRow("aa:bb:cc:dd:ee:01", []byte(`{"board":{"hostname":"gw-1"}}`), "Gateway").
			AddRow("aa:bb:cc:dd:ee:02", []byte(`{"board":{"hostname":"ap-2"}}`), "AP").
			AddRow("aa:bb:cc:dd:ee:03", []byte(`{"board":{"hostname":"ap-3"}}`), "AP"))

	mock.ExpectQuery(`SELECT device_id FROM incidents`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}))

	mock.ExpectQuery(`SELECT mac, hostname FROM client_hostnames`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"mac", "hostname"}))

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID+"/topology", nil)
	req.SetPathValue("site_id", siteID)
	rec := httptest.NewRecorder()

	GetSiteTopologyHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data TopologyGraph `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var wanEdges []GraphEdge
	for _, e := range resp.Data.Edges {
		if e.Type == "wan" {
			wanEdges = append(wanEdges, e)
		}
	}

	if len(wanEdges) != 1 {
		t.Fatalf("got %d WAN edges %+v, want exactly 1 edge to gateway only", len(wanEdges), wanEdges)
	}
	if wanEdges[0].Source != "starlink" || wanEdges[0].Target != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("WAN edge = %+v, want starlink -> aa:bb:cc:dd:ee:01", wanEdges[0])
	}
	if resp.Data.Nodes["aa:bb:cc:dd:ee:01"].Type != "gateway" {
		t.Fatalf("gateway node type = %q, want gateway", resp.Data.Nodes["aa:bb:cc:dd:ee:01"].Type)
	}
	if resp.Data.Nodes["aa:bb:cc:dd:ee:02"].Type != "ap" || resp.Data.Nodes["aa:bb:cc:dd:ee:03"].Type != "ap" {
		t.Fatalf("AP node types = %q, %q; want ap, ap", resp.Data.Nodes["aa:bb:cc:dd:ee:02"].Type, resp.Data.Nodes["aa:bb:cc:dd:ee:03"].Type)
	}
}

func TestGetSiteTopologyHandler_ExplicitWANTargetAndEmptyTelemetryWAN(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	prevDB := database.DB
	database.DB = db
	defer func() { database.DB = prevDB }()

	siteID := "site-2"
	metaJSON := `{
		"wan": {"id": "starlink", "name": "Starlink Mini", "address": "192.168.100.1", "link_label": "WAN", "speed": "120M Sat", "target": "aa:bb:cc:dd:ee:01"}
	}`

	mock.ExpectQuery(`SELECT COALESCE\(topology_metadata`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"topology_metadata"}).AddRow([]byte(metaJSON)))

	mock.ExpectQuery(`SELECT id, state_json`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_json", "device_role"}).
			AddRow("aa:bb:cc:dd:ee:01", []byte(`{"board":{"hostname":"gw-1"}}`), "Gateway").
			AddRow("aa:bb:cc:dd:ee:02", []byte(`{"board":{"hostname":"ap-2"},"wan":{}}`), "AP"))

	mock.ExpectQuery(`SELECT device_id FROM incidents`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}))

	mock.ExpectQuery(`SELECT mac, hostname FROM client_hostnames`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"mac", "hostname"}))

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID+"/topology", nil)
	req.SetPathValue("site_id", siteID)
	rec := httptest.NewRecorder()

	GetSiteTopologyHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data TopologyGraph `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var wanEdges []GraphEdge
	for _, e := range resp.Data.Edges {
		if e.Type == "wan" {
			wanEdges = append(wanEdges, e)
		}
	}
	if len(wanEdges) != 1 || wanEdges[0].Target != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("WAN edges = %+v, want only starlink -> aa:bb:cc:dd:ee:01", wanEdges)
	}
	if resp.Data.Nodes["aa:bb:cc:dd:ee:02"].Type != "ap" {
		t.Fatalf("AP with empty wan map got type %q, want ap", resp.Data.Nodes["aa:bb:cc:dd:ee:02"].Type)
	}
}
