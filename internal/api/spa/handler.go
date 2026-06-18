package spa

import (
	"net/http"
	"path/filepath"
	"strings"
)

// NewHandler returns an http.Handler that serves files from distDir.
//
// Behaviour:
//   - GET "/" or empty path → serve distDir/index.html with
//     Cache-Control: no-cache, no-store, must-revalidate so users
//     always see the latest dashboard after a release.
//   - GET /assets/... or any path whose final segment has a file
//     extension (e.g. ".js", ".css", ".svg", ".woff2", ".map") →
//     serve the file with Cache-Control: public, max-age=31536000,
//     immutable if it exists, or HTTP 404 if it does not. The 404
//     is critical: a stale browser cache will request old bundle
//     names; returning index.html (text/html) in that case confuses
//     the browser's module loader ("Expected a JavaScript-or-Wasm
//     module script but the server responded with a MIME type of
//     text/html"). A real 404 surfaces the problem clearly and lets
//     the user hard-refresh to pick up the new index.html.
//   - GET /some-spa-route (no file extension, file not on disk) →
//     serve index.html so vue-router's history mode can take over.
func NewHandler(distDir string) http.Handler {
	fs := http.FileServer(http.Dir(distDir))
	indexPath := filepath.Join(distDir, "index.html")
	spaIndex, _ := readFile(indexPath)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCacheHeaders(w, r.URL.Path)
		if r.URL.Path == "/" || r.URL.Path == "" {
			serveIndex(w, spaIndex)
			return
		}
		// Real on-disk file? serve as-is (FileServer handles Range, etc.)
		if fileExists(distDir, r.URL.Path) {
			fs.ServeHTTP(w, r)
			return
		}
		// Path looks like a static asset (last segment has a file
		// extension) but the file is missing — return 404 so the
		// browser sees a clear error instead of an HTML body with
		// a JS Content-Type expectation.
		if looksLikeAssetPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		// SPA history-mode fallback.
		serveIndex(w, spaIndex)
	})
}

// setCacheHeaders sets per-asset immutable cache, otherwise no-cache
// so a release is always picked up on the next reload.
func setCacheHeaders(w http.ResponseWriter, urlPath string) {
	if looksLikeAssetPath(urlPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
	}
}

func serveIndex(w http.ResponseWriter, body []byte) {
	if len(body) == 0 {
		http.Error(w, "frontend not built (index.html missing)", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// fileExists reports whether p (interpreted relative to distDir) maps
// to a real file on disk. We stat directly instead of poking the
// FileServer so we can distinguish "file exists" from "directory
// exists" without an extra round trip.
func fileExists(distDir, urlPath string) bool {
	clean := filepath.Clean(urlPath)
	if clean == "." || clean == "/" {
		return false
	}
	// filepath.Join discards leading slash; that's what we want here.
	full := filepath.Join(distDir, clean)
	info, err := stat(full)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// looksLikeAssetPath returns true if the final segment of urlPath
// contains a dot — i.e. it looks like "foo.js", "bar.css",
// "favicon.svg", etc. We use this as a proxy for "this is a static
// asset request, not an SPA route".
func looksLikeAssetPath(urlPath string) bool {
	if urlPath == "" {
		return false
	}
	seg := urlPath
	if i := strings.LastIndex(urlPath, "/"); i >= 0 {
		seg = urlPath[i+1:]
	}
	return strings.Contains(seg, ".")
}
