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
	mux.HandleFunc("GET /api/global/health", middleware.WithAuth(handlers.GetGlobalHealthHandler))
	mux.HandleFunc("GET /api/global/sentinel", middleware.WithAuth(handlers.GetSentinelInsightsHandler))
	mux.HandleFunc("POST /api/global/sentinel/trigger", middleware.WithAuth(handlers.TriggerManualSentinelHandler))
	mux.HandleFunc("GET /api/global/settings", middleware.WithAuth(handlers.GetPlatformSettingsHandler))
	mux.HandleFunc("POST /api/global/settings", middleware.WithAuth(handlers.UpdatePlatformSettingsHandler))
	mux.HandleFunc("POST /api/chatops/query", middleware.WithAuth(handlers.ChatOpsQueryHandler))
	mux.HandleFunc("GET /api/sites", middleware.WithAuth(handlers.GetSitesHandler))
	mux.HandleFunc("POST /api/sites", middleware.WithAuth(handlers.CreateSiteHandler))

	// ── Users / RBAC ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/users", middleware.WithAuth(middleware.RequireAdmin(handlers.GetUsersHandler)))
	mux.HandleFunc("POST /api/users", middleware.WithAuth(middleware.RequireAdmin(handlers.CreateUserHandler)))
	mux.HandleFunc("PUT /api/users/{id}/role", middleware.WithAuth(middleware.RequireAdmin(handlers.UpdateUserRoleHandler)))
	mux.HandleFunc("PUT /api/users/{id}/password", middleware.WithAuth(middleware.RequireAdmin(handlers.UpdateUserPasswordHandler)))
	mux.HandleFunc("DELETE /api/users/{id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteUserHandler)))
	mux.HandleFunc("GET /api/audit-logs", middleware.WithAuth(middleware.RequireAdmin(handlers.GetAuditLogsHandler)))

	mux.HandleFunc("GET /api/devices", middleware.WithAuth(handlers.GetDevicesHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/devices", middleware.WithAuth(handlers.GetSiteDevicesHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/adopt", middleware.WithAuth(handlers.AdoptDeviceHandler))

	mux.HandleFunc("POST /api/sites/{site_id}/wlans", middleware.WithAuth(handlers.CreateWLANHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/wlans", middleware.WithAuth(handlers.GetWLANsHandler))
	mux.HandleFunc("DELETE /api/wlans/{wlan_id}", middleware.WithAuth(handlers.DeleteWLANHandler))
	mux.HandleFunc("GET /api/devices/{device_id}/metrics", middleware.WithAuth(handlers.GetDeviceMetricsHandler))
	mux.HandleFunc("GET /api/devices/{device_id}/ssh", middleware.WithAuth(handlers.DeviceSSHHandler))

	mux.HandleFunc("GET /api/sites/{site_id}/clients", middleware.WithAuth(handlers.GetClientsHandler))
	mux.HandleFunc("PATCH /api/sites/{site_id}/clients/{mac}/hostname", middleware.WithAuth(handlers.UpdateClientHostnameHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/settings", middleware.WithAuth(handlers.GetSiteSettingsHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/settings", middleware.WithAuth(handlers.UpdateSiteSettingsHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/rotate-key", middleware.WithAuth(handlers.RotateSiteKeyHandler))
	mux.HandleFunc("PATCH /api/sites/{site_id}/auto-adopt", middleware.WithAuth(middleware.RequireAdmin(handlers.ToggleAutoAdoptHandler)))
	mux.HandleFunc("GET /api/sites/{site_id}/flow-sense", middleware.WithAuth(handlers.GetSiteFlowSenseHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/history", middleware.WithAuth(handlers.GetSiteHistoryHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/logs", middleware.WithAuth(handlers.GetLogsHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/incidents", middleware.WithAuth(handlers.GetIncidentsHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/topology", middleware.WithAuth(handlers.GetSiteTopologyHandler))
	mux.HandleFunc("PUT /api/sites/{site_id}/profile", middleware.WithAuth(handlers.AssignSiteProfileHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/rf-optimization", middleware.WithAuth(handlers.GetRFOptimizationHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/rf-fix", middleware.WithAuth(handlers.RunRFFixHandler))

	// ── VPN Matrix / SECURE_TUNNEL ───────────────────────────────────────────
	mux.HandleFunc("GET /api/sites/{site_id}/vpn", middleware.WithAuth(handlers.GetVPNConfigHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/vpn/endpoint", middleware.WithAuth(handlers.UpdateVPNEndpointHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/vpn/peers", middleware.WithAuth(handlers.GetVPNPeersHandler))

	// ── Vault / Firmware ──────────────────────────────────────────────────────
	mux.HandleFunc("POST /api/devices/{device_id}/backup", middleware.WithAuth(handlers.CreateBackupTrigger))
	mux.HandleFunc("GET /api/devices/{device_id}/backups", middleware.WithAuth(handlers.GetDeviceBackupsHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/audit", middleware.WithAuth(handlers.TriggerVaultAuditHandler))
	mux.HandleFunc("GET /api/devices/{device_id}/audit", middleware.WithAuth(handlers.GetDeviceAuditResultsHandler))
	mux.HandleFunc("GET /api/backups/{backup_id}/diff", middleware.WithAuth(handlers.DiffBackupHandler))
	mux.HandleFunc("POST /api/firmwares", middleware.WithAuth(handlers.UploadFirmwareHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/sysupgrade", middleware.WithAuth(handlers.TriggerSysupgradeHandler))
	mux.HandleFunc("GET /api/profiles", middleware.WithAuth(handlers.ListProfilesHandler))
	mux.HandleFunc("POST /api/profiles", middleware.WithAuth(handlers.CreateProfileHandler))
	mux.HandleFunc("DELETE /api/profiles/{profile_id}", middleware.WithAuth(handlers.DeleteProfileHandler))
	mux.HandleFunc("POST /api/orchestrator/command", middleware.WithAuth(handlers.MassCommandHandler))

	// ── Traffic Management ───────────────────────────────────────────────────
	mux.HandleFunc("POST /api/bandwidth/limit", middleware.WithAuth(handlers.LimitBandwidthHandler))
	mux.HandleFunc("GET /api/bandwidth/stats", middleware.WithAuth(handlers.BandwidthStatsHandler))
	mux.HandleFunc("POST /api/bandwidth/sniper", middleware.WithAuth(handlers.SniperBandwidthHandler))

	// ── Threat Shield / IPS ──────────────────────────────────────────────────
	mux.HandleFunc("GET /api/threat-shield/status", middleware.WithAuth(handlers.GetThreatShieldStatusHandler))
	mux.HandleFunc("GET /api/threat-shield/list", handlers.GetThreatShieldListHandler) // X-Site-Key auth
	mux.HandleFunc("GET /api/sites/{site_id}/threat-shield", middleware.WithAuth(handlers.GetSiteThreatShieldHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/threat-shield", middleware.WithAuth(handlers.ToggleThreatShieldHandler))

	// ── EDGE_NEXUS / L3 Edge Management ──────────────────────────────────────
	mux.HandleFunc("GET /api/devices/{id}/edge-network", middleware.WithAuth(handlers.GetEdgeNetworkHandler))
	mux.HandleFunc("PUT /api/devices/{id}/edge-network", middleware.WithAuth(handlers.PutEdgeNetworkHandler))
	mux.HandleFunc("GET /api/devices/{id}/edge-dhcp", middleware.WithAuth(handlers.GetEdgeDHCPHandler))
	mux.HandleFunc("PUT /api/devices/{id}/edge-dhcp", middleware.WithAuth(handlers.PutEdgeDHCPHandler))
	mux.HandleFunc("GET /api/devices/{id}/edge-firewall", middleware.WithAuth(handlers.GetEdgeFirewallHandler))
	mux.HandleFunc("PUT /api/devices/{id}/edge-firewall", middleware.WithAuth(handlers.PutEdgeFirewallHandler))

	// ── Agent Management ─────────────────────────────────────────────────────
	// Device-facing: authenticated by X-Site-Key header (no JWT)
	mux.HandleFunc("GET /api/agent/latest", handlers.GetLatestAgentHandler)
	mux.HandleFunc("GET /api/agent/latest/raw", handlers.GetLatestAgentRawHandler)
	// Dashboard-facing: JWT required
	mux.HandleFunc("POST /api/agent/deploy", middleware.WithAuth(handlers.DeployAgentHandler))
	mux.HandleFunc("GET /api/agent/status", middleware.WithAuth(handlers.GetAgentVersionsStatusHandler))
	mux.HandleFunc("GET /api/agent/site/raw", middleware.WithAuth(handlers.GetSiteAgentRawHandler))

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
