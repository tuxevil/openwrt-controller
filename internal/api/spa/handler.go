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

// looksLikeAssetPath reports whether the final path segment has a
// file extension. We use this as a proxy for "this is a static asset
// request" so the SPA fallback doesn't paper over missing files with
// an HTML body when the browser is expecting a script or stylesheet.
func looksLikeAssetPath(urlPath string) bool {
	// Strip query string if any.
	if i := strings.IndexByte(urlPath, '?'); i >= 0 {
		urlPath = urlPath[:i]
	}
	lastSlash := strings.LastIndexByte(urlPath, '/')
	seg := urlPath
	if lastSlash >= 0 {
		seg = urlPath[lastSlash+1:]
	}
	if seg == "" {
		return false
	}
	return strings.Contains(seg, ".")
}

func serveIndex(w http.ResponseWriter, body []byte) {
	if len(body) == 0 {
		http.Error(w, "frontend not built (index.html missing)", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Strict CSP. Defends against reverse-proxy shims (e.g. Coolify's
	// wrs_env.js) that try to inject inline scripts into the SPA
	// response — the page then throws before Vue can mount and the
	// user sees a blank screen. Inline <script> is disallowed
	// entirely; the production build only loads /assets/*.js from
	// the same origin, which is covered by 'self'. We keep
	// 'unsafe-inline' for style only because Vite's runtime CSS
	// dev-shim uses it, and the harm surface is much smaller.
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: https:; "+
			"font-src 'self' data:; "+
			"connect-src 'self'; "+
			"worker-src 'self' blob:; "+
			"frame-ancestors 'none'; "+
			"base-uri 'self'; "+
			"form-action 'self'")
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
