package spa

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain builds a small fixture dist tree once and reuses it across
// the suite. We point the handler at a tempdir so the tests don't
// depend on the build-time location of web/dist.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func newTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	// Per-test dist under t.TempDir() so parallel tests don't collide
	// and so the handler reads from a known-good snapshot.
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "index.html"), "<!doctype html><html><body>SPA</body></html>")
	mustMkdir(t, filepath.Join(dir, "assets"))
	mustWrite(t, filepath.Join(dir, "assets", "app-abc.js"), "console.log('hi');")
	mustWrite(t, filepath.Join(dir, "assets", "app-abc.css"), "body { color: red; }")
	mustWrite(t, filepath.Join(dir, "favicon.svg"), "<svg/>")
	return NewHandler(dir), dir
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func get(h http.Handler, p string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, p, nil)
	h.ServeHTTP(rr, req)
	return rr
}

func TestHandler_ServesExistingAsset(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := get(h, "/assets/app-abc.js")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "" && !strings.HasPrefix(got, "application/javascript") && !strings.HasPrefix(got, "text/javascript") {
		// Go's http.FileServer sniffs content-type; .js typically becomes
		// application/javascript or text/javascript. The only thing that
		// matters for the bug under test is that we do NOT return text/html.
		t.Errorf("expected JS content type, got %q", got)
	}
	if rr.Header().Get("Content-Type") == "text/html" {
		t.Errorf("asset served as text/html (the bug we are fixing)")
	}
	if rr.Body.String() != "console.log('hi');" {
		t.Errorf("body mismatch: %q", rr.Body.String())
	}
}

func TestHandler_ServesIndexForRoot(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := get(h, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", ct)
	}
	if !contains(rr.Body.String(), "SPA") {
		t.Errorf("expected index.html body, got %q", rr.Body.String())
	}
}

func TestHandler_SPAFallbackForRouteWithoutExtension(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, p := range []string{
		"/site/abc-123",
		"/site/abc-123/settings",
		"/global/sentinel",
		"/landlord",
	} {
		rr := get(h, p)
		if rr.Code != http.StatusOK {
			t.Errorf("path %q: expected 200 SPA fallback, got %d", p, rr.Code)
			continue
		}
		if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("path %q: expected text/html, got %q", p, ct)
		}
	}
}

func TestHandler_404ForMissingAsset(t *testing.T) {
	h, _ := newTestHandler(t)
	// This is the regression: a stale browser cache asks for an old
	// bundle name like /assets/index-OLD.js. The previous implementation
	// fell back to index.html (text/html), which the browser refuses
	// to execute as a module. We must return 404 instead so the user
	// sees a clear error and a hard-refresh fixes the mismatch.
	for _, p := range []string{
		"/assets/index-OLD.js",
		"/assets/index-MISSING.css",
		"/favicon-missing.svg",
	} {
		rr := get(h, p)
		if rr.Code != http.StatusNotFound {
			t.Errorf("path %q: expected 404, got %d (body=%q)", p, rr.Code, rr.Body.String())
		}
		if ct := rr.Header().Get("Content-Type"); ct == "text/html" {
			t.Errorf("path %q: must not return text/html for missing asset (regression: MIME mismatch blocks module load)", p)
		}
	}
}

func TestHandler_CacheHeaders(t *testing.T) {
	h, _ := newTestHandler(t)
	// Asset: long cache, immutable
	rr := get(h, "/assets/app-abc.js")
	if cc := rr.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("asset cache-control: got %q", cc)
	}
	// Index: no-cache (always revalidate)
	rr = get(h, "/")
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
		t.Errorf("index cache-control: got %q", cc)
	}
}

func TestHandler_PreservesDirectoryAsset(t *testing.T) {
	// Some Vite builds emit e.g. /assets/icons.svg served from a nested
	// path. Verify the handler can serve those when they exist on disk.
	h, dir := newTestHandler(t)
	mustMkdir(t, filepath.Join(dir, "assets", "icons"))
	mustWrite(t, filepath.Join(dir, "assets", "icons", "foo.svg"), "<svg/>")
	rr := get(h, "/assets/icons/foo.svg")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
