package handlers

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func buildTestKMZ(t *testing.T, files map[string][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "test.kmz")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseKMZSuperOverlayAndServeTiles(t *testing.T) {
	pngBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	jpgBytes := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0}
	kmzPath := buildTestKMZ(t, map[string][]byte{
		"doc.kml": []byte(`<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2"><Document><name>Alta Vista</name>
<NetworkLink><Region><LatLonAltBox><north>-2.0199</north><south>-2.0254</south><east>-78.9455</east><west>-78.9512</west></LatLonAltBox></Region></NetworkLink>
</Document></kml>`),
		"0/0/0.kml": []byte(`<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2"><Document>
<Region><LatLonAltBox><north>-2.019919</north><south>-2.025369</south><east>-78.945526</east><west>-78.951177</west></LatLonAltBox></Region>
</Document></kml>`),
		"0/0/0.png": pngBytes,
		"1/0/0.jpg": jpgBytes,
		"1/1/1.png": pngBytes,
	})

	parsed, err := parseKMZArchive(kmzPath, "altavista.kmz")
	if err != nil {
		t.Fatalf("parseKMZArchive failed: %v", err)
	}
	if parsed.Meta.Mode != "superoverlay" {
		t.Fatalf("mode = %q, want superoverlay", parsed.Meta.Mode)
	}
	if parsed.Meta.Name != "Alta Vista" {
		t.Fatalf("name = %q, want Alta Vista", parsed.Meta.Name)
	}
	if parsed.Meta.MinLevel != 0 || parsed.Meta.MaxLevel != 1 || parsed.Meta.TileCount != 3 {
		t.Fatalf("unexpected levels/count: %+v", parsed.Meta)
	}
	if parsed.Meta.Bounds[0][0] != -2.025369 || parsed.Meta.Bounds[1][1] != -78.945526 {
		t.Fatalf("expected 0/0/0.kml bounds, got %+v", parsed.Meta.Bounds)
	}

	// Install into temp maps dir and serve via tile handler helper
	mapsDir := t.TempDir()
	t.Setenv("SURVEY_MAPS_DIR", mapsDir)
	siteID := "d982665c-8fd8-4c32-815f-257cf5bb450a"
	if _, err := installSiteKMZ(siteID, kmzPath, "altavista.kmz"); err != nil {
		t.Fatalf("installSiteKMZ failed: %v", err)
	}

	// Tile 1/0/0 -> 200 image/jpeg
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID+"/maps/kmz/tile/1/0/0", nil)
	req.SetPathValue("site_id", siteID)
	req.SetPathValue("z", "1")
	req.SetPathValue("x", "0")
	req.SetPathValue("y", "0")
	w := httptest.NewRecorder()
	GetSiteKMZTileHandler(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("tile 1/0/0 got %d %q", w.Code, w.Header().Get("Content-Type"))
	}

	// Missing tile 1/1/0 -> 204 No Content
	reqMiss := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID+"/maps/kmz/tile/1/1/0", nil)
	reqMiss.SetPathValue("site_id", siteID)
	reqMiss.SetPathValue("z", "1")
	reqMiss.SetPathValue("x", "1")
	reqMiss.SetPathValue("y", "0")
	wMiss := httptest.NewRecorder()
	GetSiteKMZTileHandler(wMiss, reqMiss)
	if wMiss.Code != http.StatusNoContent {
		t.Fatalf("missing tile got %d, want 204", wMiss.Code)
	}

	// Path traversal site_id rejected
	reqBad := httptest.NewRequest(http.MethodGet, "/api/sites/../etc/maps/kmz/tile/0/0/0", nil)
	reqBad.SetPathValue("site_id", "../etc")
	reqBad.SetPathValue("z", "0")
	reqBad.SetPathValue("x", "0")
	reqBad.SetPathValue("y", "0")
	wBad := httptest.NewRecorder()
	GetSiteKMZTileHandler(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("bad site_id got %d, want 400", wBad.Code)
	}
}

func TestParseKMZSingleOverlay(t *testing.T) {
	pngBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	kmzPath := buildTestKMZ(t, map[string][]byte{
		"doc.kml": []byte(`<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2"><Document><name>Finca</name>
<GroundOverlay><Icon><href>files/ortho.png</href></Icon>
<LatLonBox><north>-2.01</north><south>-2.03</south><east>-78.94</east><west>-78.96</west></LatLonBox>
</GroundOverlay></Document></kml>`),
		"files/ortho.png": pngBytes,
	})

	parsed, err := parseKMZArchive(kmzPath, "finca.kmz")
	if err != nil {
		t.Fatalf("parseKMZArchive single failed: %v", err)
	}
	if parsed.Meta.Mode != "single" || parsed.SingleEntry != "files/ortho.png" {
		t.Fatalf("unexpected single overlay meta: %+v entry=%q", parsed.Meta, parsed.SingleEntry)
	}
}
