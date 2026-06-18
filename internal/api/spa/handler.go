package spa

import (
	"net/http"
	"path/filepath"
	"strings"
)

// _ = strings.Contains // ensure import used (legacy, can remove if unused)

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
	// Belt-and-suspenders: also write the same CSP as a <meta> tag
	// inside the served HTML so the policy survives any reverse
	// proxy that strips response headers but not the body. The
	// meta tag is injected right after the <head> opening tag so
	// the browser picks it up before evaluating any further
	// markup.
	_, _ = w.Write(injectCSPAndFallback(body))
}

// injectCSPAndFallback inserts (a) a <meta> CSP tag as the first
// child of <head> and (b) a static <div id="boot-fallback"> that
// shows "JavaScript is blocked or failed to load" if Vue never
// mounts. The fallback uses a CSS animation with a 4-second delay
// — no JavaScript needed. The Vue app's first action should remove
// the element so it never shows during normal operation.
//
// We deliberately avoid an inline <script> for the reveal: the strict
// CSP (script-src 'self') would block it anyway. The CSS animation
// is the lowest-tech path that works even when all scripts are
// blocked, which is exactly the situation we want to detect.
func injectCSPAndFallback(body []byte) []byte {
	csp := `<meta http-equiv="Content-Security-Policy" content="default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'">`
	fallback := `<style>@keyframes nc-boot-fallback-reveal{from{opacity:0}to{opacity:1}}#boot-fallback{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;flex-direction:column;background:#000;color:#39FF14;font-family:monospace;padding:24px;text-align:center;z-index:9999;opacity:0;animation:nc-boot-fallback-reveal .3s 4s forwards}#boot-fallback .icon{font-size:48px;margin-bottom:16px}#boot-fallback .title{font-size:18px;margin-bottom:12px;color:#39FF14}#boot-fallback .hint{font-size:12px;color:#888;max-width:340px;line-height:1.6}#boot-fallback code{color:#fff;background:#111;padding:2px 6px;border-radius:3px}</style><div id="boot-fallback"><div class="icon">⚠</div><div class="title">JavaScript failed to start</div><div class="hint">The dashboard did not load. This usually means a browser extension, MDM policy, captive portal, or reverse proxy is blocking scripts. Try <code>incognito mode</code> (Ctrl+Shift+N on desktop, ⋮ → New incognito on mobile) or open <code>/survey/&lt;id&gt;</code> in a different network.</div></div><noscript><div style="position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:#000;color:#39FF14;font-family:monospace;padding:24px;text-align:center"><div><div style="font-size:18px;margin-bottom:12px">JavaScript is required</div><div style="font-size:12px;color:#888">Please enable JavaScript in your browser.</div></div></div></noscript>`

	// Insert the CSP meta as the first child of <head>.
	s := string(body)
	if i := strings.Index(s, "<head>"); i >= 0 {
		ins := i + len("<head>")
		s = s[:ins] + csp + s[ins:]
	}
	// Insert the fallback block right after <body>.
	if i := strings.Index(s, "<body>"); i >= 0 {
		ins := i + len("<body>")
		s = s[:ins] + fallback + s[ins:]
	}
	return []byte(s)
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
