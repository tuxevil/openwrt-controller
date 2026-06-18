// Package spa provides the static-asset + history-mode-fallback handler
// for the Vue dashboard. It serves files from a dist directory and
// transparently serves index.html for client-side routes that don't
// map to a real file, while returning 404 for missing assets so a
// stale browser cache request for an old bundle name surfaces as a
// real error instead of a confusing "text/html in module" failure.
package spa
