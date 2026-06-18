// Nerve Center — script firewall service worker.
// 
// Installed on first visit. Intercepts every request to the
// controller origin and only allows Vite-built bundles. Any
// <script src> added to the page by a third party (Chrome
// extension, captive portal, MDM) that points outside the
// allowlist gets a 204 No Content — the browser drops it before
// the script ever executes.
//
// This is the second layer of defence behind the server-side
// strip in internal/api/spa/handler.go. That one removes script
// tags from the served HTML; this one rejects the actual fetch
// if something on the device re-adds them client-side.

const ALLOWED_SCRIPT_HOSTS = ['/assets/']
const ALLOWED_SCRIPT_EXT = ['.js']

self.addEventListener('install', () => {
  // Take over immediately on first install — the user is
  // already on the page, they shouldn't have to refresh.
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  // Claim any uncontrolled clients (e.g. if the SW was updated
  // mid-session by a new build, the previous version's clients
  // would otherwise be left outside the firewall).
  event.waitUntil(self.clients.claim())
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)

  // Only filter requests to our own origin. Third-party hosts
  // are left untouched (so the page can still load Leaflet
  // tiles, etc., if we ever switch away from Carto).
  if (url.origin !== self.location.origin) return

  // Only filter script-style requests. CSS, images, fonts,
  // XHR, and the SPA HTML itself all pass through unchanged.
  if (req.destination !== 'script' && req.destination !== 'worker') {
    return
  }

  // Allow Vite's hashed bundles under /assets/*.js. Anything
  // else that resolves to a .js URL — wrs_env.js, web-client-
  // content-script.js, /payload.js — gets nuked.
  const path = url.pathname
  if (!ALLOWED_SCRIPT_HOSTS.some(p => path.startsWith(p))) return
  if (!ALLOWED_SCRIPT_EXT.some(ext => path.endsWith(ext))) return

  // Allow through.
  return
})
