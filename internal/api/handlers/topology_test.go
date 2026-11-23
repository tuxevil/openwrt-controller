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

func TestGetSiteTopologyHandler_DeduplicatesWiredAndOmitsTransitiveSiblingLinks_WithMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	prevDB := database.DB
	database.DB = db
	defer func() { database.DB = prevDB }()

	siteID := "site-wired-meta"
	metaJSON := `{
		"wan": {"id": "starlink", "name": "Starlink Mini", "address": "192.168.100.1", "link_label": "WAN", "speed": "120M Sat"},
		"links": [
			{"source": "aa:bb:cc:dd:ee:01", "target": "aa:bb:cc:dd:ee:02", "label": "PoE uplink", "speed": "100 Mbps"},
			{"source": "aa:bb:cc:dd:ee:01", "target": "aa:bb:cc:dd:ee:03", "label": "Powerline uplink", "speed": "38 Mbps"}
		],
		"roles": {
			"aa:bb:cc:dd:ee:01": "Gateway",
			"aa:bb:cc:dd:ee:02": "Distribution AP",
			"aa:bb:cc:dd:ee:03": "Distribution AP"
		}
	}`

	mock.ExpectQuery(`SELECT COALESCE\(topology_metadata`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"topology_metadata"}).AddRow([]byte(metaJSON)))

	// Simulate flat L2 bridge_table where every node sees the other two routers' MACs.
	gwState := `{"board":{"hostname":"mr8300"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:02","port":"4","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:03","port":"3","is_local":"no"}]}}`
	ap2State := `{"board":{"hostname":"uapmesh"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:01","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:03","port":"1","is_local":"no"}]}}`
	ap3State := `{"board":{"hostname":"wndr3800ch"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:01","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:02","port":"1","is_local":"no"}]}}`

	mock.ExpectQuery(`SELECT id, state_json`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_json", "device_role"}).
			AddRow("aa:bb:cc:dd:ee:01", []byte(gwState), "Gateway").
			AddRow("aa:bb:cc:dd:ee:02", []byte(ap2State), "AP").
			AddRow("aa:bb:cc:dd:ee:03", []byte(ap3State), "AP"))

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

	var wiredEdges []GraphEdge
	for _, e := range resp.Data.Edges {
		if e.Type == "wired" {
			wiredEdges = append(wiredEdges, e)
		}
	}

	if len(wiredEdges) != 2 {
		t.Fatalf("got %d wired edges %+v, want exactly 2 (gw->ap2 and gw->ap3)", len(wiredEdges), wiredEdges)
	}

	seen := make(map[string]GraphEdge)
	for _, e := range wiredEdges {
		seen[e.Source+"->"+e.Target] = e
	}
	if e, ok := seen["aa:bb:cc:dd:ee:01->aa:bb:cc:dd:ee:02"]; !ok || e.Label != "PoE uplink" || e.Speed != "100 Mbps" {
		t.Fatalf("missing or wrong gw->ap2 edge: %+v", wiredEdges)
	}
	if e, ok := seen["aa:bb:cc:dd:ee:01->aa:bb:cc:dd:ee:03"]; !ok || e.Label != "Powerline uplink" || e.Speed != "38 Mbps" {
		t.Fatalf("missing or wrong gw->ap3 edge: %+v", wiredEdges)
	}
}

func TestGetSiteTopologyHandler_InfersStarAndDaisyChainWithoutMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	prevDB := database.DB
	database.DB = db
	defer func() { database.DB = prevDB }()

	siteID := "site-wired-inferred"
	mock.ExpectQuery(`SELECT COALESCE\(topology_metadata`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"topology_metadata"}).AddRow([]byte(`{}`)))

	// Star: gw -> ap2 (port 4), gw -> ap3 (port 3).
	// Daisy chain: ap3 -> ap4 (port 2 on ap3; ap4 is also seen on gw port 3, ap2 port 1).
	gwState := `{"board":{"hostname":"gw"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:02","port":"4","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:03","port":"3","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:04","port":"3","is_local":"no"}]}}`
	ap2State := `{"board":{"hostname":"ap2"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:01","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:03","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:04","port":"1","is_local":"no"}]}}`
	ap3State := `{"board":{"hostname":"ap3"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:01","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:02","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:04","port":"2","is_local":"no"}]}}`
	ap4State := `{"board":{"hostname":"ap4"},"neighbor_stats":{"bridge_table":[{"mac":"aa:bb:cc:dd:ee:01","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:02","port":"1","is_local":"no"},{"mac":"aa:bb:cc:dd:ee:03","port":"1","is_local":"no"}]}}`

	mock.ExpectQuery(`SELECT id, state_json`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_json", "device_role"}).
			AddRow("aa:bb:cc:dd:ee:01", []byte(gwState), "Gateway").
			AddRow("aa:bb:cc:dd:ee:02", []byte(ap2State), "AP").
			AddRow("aa:bb:cc:dd:ee:03", []byte(ap3State), "AP").
			AddRow("aa:bb:cc:dd:ee:04", []byte(ap4State), "AP"))

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

	seen := make(map[string]int)
	for _, e := range resp.Data.Edges {
		if e.Type == "wired" {
			seen[e.Source+"->"+e.Target]++
		}
	}

	// Must have NO ap2<->ap3 or ap2<->ap4 sibling links and NO duplicate reverse links.
	for pair := range seen {
		if pair == "aa:bb:cc:dd:ee:02->aa:bb:cc:dd:ee:03" || pair == "aa:bb:cc:dd:ee:03->aa:bb:cc:dd:ee:02" ||
			pair == "aa:bb:cc:dd:ee:02->aa:bb:cc:dd:ee:04" || pair == "aa:bb:cc:dd:ee:04->aa:bb:cc:dd:ee:02" {
			t.Fatalf("unexpected transitive sibling link %q in %+v", pair, seen)
		}
	}
	if seen["aa:bb:cc:dd:ee:01->aa:bb:cc:dd:ee:02"] != 1 {
		t.Fatalf("expected single gw->ap2 edge, got %+v", seen)
	}
	if seen["aa:bb:cc:dd:ee:01->aa:bb:cc:dd:ee:03"] != 1 {
		t.Fatalf("expected single gw->ap3 edge, got %+v", seen)
	}
	if seen["aa:bb:cc:dd:ee:03->aa:bb:cc:dd:ee:04"] != 1 {
		t.Fatalf("expected single daisy-chain ap3->ap4 edge, got %+v", seen)
	}
	if len(seen) != 3 {
		t.Fatalf("expected exactly 3 wired edges (gw->ap2, gw->ap3, ap3->ap4), got %+v", seen)
	}
}

func TestGetSiteTopologyResolvesCustomHostnameCaseInsensitive(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	origDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = origDB
		_ = db.Close()
	})

	const siteID = "site-host-1"
	mock.ExpectQuery(`SELECT COALESCE\(topology_metadata`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"topology_metadata"}).AddRow([]byte(`{}`)))

	gwState := `{"board":{"hostname":"mr8300"},"wireless_stations":{"phy0-ap0":[{"mac":"3c:28:6d:0f:85:db","signal":-50}]}}`
	mock.ExpectQuery(`SELECT id, state_json`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "state_json", "device_role"}).
			AddRow("e8:9f:80:14:69:c5", []byte(gwState), "Gateway"))

	mock.ExpectQuery(`SELECT device_id FROM incidents`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}))

	// Stored uppercase in client_hostnames, reported lowercase in wireless_stations.
	mock.ExpectQuery(`SELECT mac, hostname FROM client_hostnames`).
		WithArgs(siteID).
		WillReturnRows(sqlmock.NewRows([]string{"mac", "hostname"}).
			AddRow("3C:28:6D:0F:85:DB", "pixel3"))

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

	node, ok := resp.Data.Nodes["3c:28:6d:0f:85:db"]
	if !ok {
		t.Fatalf("expected client node 3c:28:6d:0f:85:db in topology, got %+v", resp.Data.Nodes)
	}
	if node.Name != "pixel3" || node.Hostname != "pixel3" {
		t.Fatalf("client node = %+v, want Name=pixel3 Hostname=pixel3", node)
	}
}
