package api

import (
	"net/http"

	"openwrt-controller/internal/api/handlers"
	"openwrt-controller/internal/api/middleware"
)

func SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// ── Public routes (no auth) ──────────────────────────────────────────────
	mux.HandleFunc("POST /api/auth/login", handlers.LoginHandler)
	// Telemetry is device-authenticated (X-Device-Token), not user-authenticated
	mux.HandleFunc("POST /api/telemetry", handlers.TelemetryHandler)
	mux.HandleFunc("GET /api/devices/{device_id}/config", handlers.GetDeviceConfigHandler)

	// ── Protected API routes ─────────────────────────────────────────────────
	mux.HandleFunc("GET /api/sites", middleware.WithAuth(handlers.GetSitesHandler))
	mux.HandleFunc("POST /api/sites", middleware.WithAuth(handlers.CreateSiteHandler))

	mux.HandleFunc("GET /api/devices", middleware.WithAuth(handlers.GetDevicesHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/devices", middleware.WithAuth(handlers.GetSiteDevicesHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/adopt", middleware.WithAuth(handlers.AdoptDeviceHandler))

	mux.HandleFunc("POST /api/sites/{site_id}/wlans", middleware.WithAuth(handlers.CreateWLANHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/wlans", middleware.WithAuth(handlers.GetWLANsHandler))
	mux.HandleFunc("DELETE /api/wlans/{wlan_id}", middleware.WithAuth(handlers.DeleteWLANHandler))
	mux.HandleFunc("GET /api/devices/{device_id}/metrics", middleware.WithAuth(handlers.GetDeviceMetricsHandler))
	mux.HandleFunc("GET /api/devices/{device_id}/ssh", middleware.WithAuth(handlers.DeviceSSHHandler))

	mux.HandleFunc("GET /api/sites/{site_id}/clients", middleware.WithAuth(handlers.GetClientsHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/settings", middleware.WithAuth(handlers.GetSiteSettingsHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/settings", middleware.WithAuth(handlers.UpdateSiteSettingsHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/rotate-key", middleware.WithAuth(handlers.RotateSiteKeyHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/history", middleware.WithAuth(handlers.GetSiteHistoryHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/logs", middleware.WithAuth(handlers.GetLogsHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/incidents", middleware.WithAuth(handlers.GetIncidentsHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/topology", middleware.WithAuth(handlers.GetSiteTopologyHandler))
	mux.HandleFunc("PUT /api/sites/{site_id}/profile", middleware.WithAuth(handlers.AssignSiteProfileHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/rf-optimization", middleware.WithAuth(handlers.GetRFOptimizationHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/rf-fix", middleware.WithAuth(handlers.RunRFFixHandler))

	// ── Orchestrator ──────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/profiles", middleware.WithAuth(handlers.ListProfilesHandler))
	mux.HandleFunc("POST /api/profiles", middleware.WithAuth(handlers.CreateProfileHandler))
	mux.HandleFunc("DELETE /api/profiles/{profile_id}", middleware.WithAuth(handlers.DeleteProfileHandler))
	mux.HandleFunc("POST /api/orchestrator/command", middleware.WithAuth(handlers.MassCommandHandler))

	// ── SPA Static files ─────────────────────────────────────────────────────
	fs := http.FileServer(http.Dir("./web/dist"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" || !isAsset(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		_, err := http.Dir("./web/dist").Open(r.URL.Path)
		if err != nil {
			r.URL.Path = "/"
		}
		fs.ServeHTTP(w, r)
	})

	return mux
}

func isAsset(path string) bool {
	return len(path) > 8 && path[:8] == "/assets/"
}
