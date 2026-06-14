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
	mux.HandleFunc("POST /api/telemetry", handlers.TelemetryHandler)
	mux.HandleFunc("GET /api/devices/{device_id}/config", handlers.GetDeviceConfigHandler)

	// ── Public Guest Portal routes ───────────────────────────────────────────
	mux.HandleFunc("GET /portal/auth/{site_id}", handlers.GetPortalAuthHandler)
	mux.HandleFunc("POST /api/public/portal/{site_id}/validate", handlers.ValidatePortalHandler)

	// ── Protected API routes ─────────────────────────────────────────────────
	mux.HandleFunc("GET /api/global/health", middleware.WithAuth(handlers.GetGlobalHealthHandler))
	mux.HandleFunc("GET /api/global/sentinel", middleware.WithAuth(handlers.GetSentinelInsightsHandler))
	mux.HandleFunc("POST /api/global/sentinel/trigger", middleware.WithAuth(handlers.TriggerManualSentinelHandler))
	mux.HandleFunc("GET /api/global/settings", middleware.WithAuth(handlers.GetPlatformSettingsHandler))
	mux.HandleFunc("POST /api/global/settings", middleware.WithAuth(handlers.UpdatePlatformSettingsHandler))
	mux.HandleFunc("POST /api/chatops/query", middleware.WithAuth(handlers.ChatOpsQueryHandler))
	mux.HandleFunc("GET /api/sites", middleware.WithAuth(handlers.GetSitesHandler))
	mux.HandleFunc("POST /api/sites", middleware.WithAuth(handlers.CreateSiteHandler))
	mux.HandleFunc("DELETE /api/sites/{site_id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteSiteHandler)))

	// ── Users / RBAC ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /api/users", middleware.WithAuth(middleware.RequireAdmin(handlers.GetUsersHandler)))
	mux.HandleFunc("POST /api/users", middleware.WithAuth(middleware.RequireAdmin(handlers.CreateUserHandler)))
	mux.HandleFunc("PUT /api/users/{id}/role", middleware.WithAuth(middleware.RequireAdmin(handlers.UpdateUserRoleHandler)))
	mux.HandleFunc("PUT /api/users/{id}/password", middleware.WithAuth(middleware.RequireAdmin(handlers.UpdateUserPasswordHandler)))
	mux.HandleFunc("DELETE /api/users/{id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteUserHandler)))
	mux.HandleFunc("GET /api/audit-logs", middleware.WithAuth(middleware.RequireAdmin(handlers.GetAuditLogsHandler)))

	mux.HandleFunc("GET /api/devices", middleware.WithAuth(handlers.GetDevicesHandler))
	mux.HandleFunc("DELETE /api/devices/{device_id}", middleware.WithAuth(middleware.RequireAdmin(handlers.ForgetDeviceHandler)))
	mux.HandleFunc("GET /api/sites/{site_id}/devices", middleware.WithAuth(handlers.GetSiteDevicesHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/adopt", middleware.WithAuth(handlers.AdoptDeviceHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/migrate", middleware.WithAuth(handlers.MigrateDeviceHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/import-config", middleware.WithAuth(middleware.RequireAdmin(handlers.ImportDeviceConfigHandler)))

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
	mux.HandleFunc("GET /api/sites/{site_id}/echolocation", middleware.WithAuth(handlers.GetSiteEchoLocationHandler))
	mux.HandleFunc("PUT /api/sites/{site_id}/profile", middleware.WithAuth(handlers.AssignSiteProfileHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/rf-optimization", middleware.WithAuth(handlers.GetRFOptimizationHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/rf-fix", middleware.WithAuth(handlers.RunRFFixHandler))

	// ── GUEST PORTAL / Captive Portal ────────────────────────────────────────
	mux.HandleFunc("GET /api/sites/{site_id}/portal/settings", middleware.WithAuth(handlers.GetPortalSettingsHandler))
	mux.HandleFunc("PUT /api/sites/{site_id}/portal/settings", middleware.WithAuth(handlers.UpdatePortalSettingsHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/portal/vouchers", middleware.WithAuth(handlers.GetPortalVouchersHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/portal/vouchers/generate", middleware.WithAuth(handlers.GeneratePortalVouchersHandler))

	// ── MATRIX_ANALYTICS / Deep Telemetry Insights ───────────────────────────
	mux.HandleFunc("GET /api/sites/{site_id}/analytics/throughput", middleware.WithAuth(handlers.GetAnalyticsThroughputHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/analytics/top-talkers", middleware.WithAuth(handlers.GetAnalyticsTopTalkersHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/analytics/protocols", middleware.WithAuth(handlers.GetAnalyticsProtocolsHandler))

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

	// ── OMADA_MIGRATOR / State Migration Bridge ──────────────────────────────
	mux.HandleFunc("POST /api/migration/omada/analyze", middleware.WithAuth(handlers.AnalyzeOmadaBackup))
	mux.HandleFunc("POST /api/migration/omada/commit", middleware.WithAuth(handlers.CommitOmadaMigration))

	// ── UCI_OPS / Universal Configuration Manager ────────────────────────────
	mux.HandleFunc("GET /api/devices/{device_id}/uci", middleware.WithAuth(middleware.RequireAdmin(handlers.GetUciHandler)))
	mux.HandleFunc("PUT /api/devices/{device_id}/uci", middleware.WithAuth(middleware.RequireAdmin(handlers.PutUciHandler)))

	// ── CENTRAL_LUCI / Low-Level Configuration Interface ─────────────────
	mux.HandleFunc("GET /api/devices/{device_id}/central-config", middleware.WithAuth(middleware.RequireAdmin(handlers.GetCentralConfigHandler)))
	mux.HandleFunc("GET /api/devices/{device_id}/central-configs", middleware.WithAuth(middleware.RequireAdmin(handlers.ListCentralConfigsHandler)))
	mux.HandleFunc("PUT /api/devices/{device_id}/central-config", middleware.WithAuth(middleware.RequireAdmin(handlers.PutCentralConfigHandler)))
	mux.HandleFunc("POST /api/central-config/preview", middleware.WithAuth(middleware.RequireAdmin(handlers.PreviewCentralConfigHandler)))

	// ── SITE_ORCHESTRATOR / Global Fleet Templates ───────────────────────
	mux.HandleFunc("GET /api/sites/{site_id}/site-config", middleware.WithAuth(middleware.RequireAdmin(handlers.GetSiteConfigHandler)))
	mux.HandleFunc("PUT /api/sites/{site_id}/site-config", middleware.WithAuth(middleware.RequireAdmin(handlers.PutSiteConfigHandler)))
	mux.HandleFunc("GET /api/sites/{site_id}/device-roles", middleware.WithAuth(middleware.RequireAdmin(handlers.GetSiteDeviceRolesHandler)))
	mux.HandleFunc("PUT /api/devices/{device_id}/role", middleware.WithAuth(middleware.RequireAdmin(handlers.PutDeviceRoleHandler)))
	mux.HandleFunc("POST /api/sites/{site_id}/orchestrator/preview", middleware.WithAuth(middleware.RequireAdmin(handlers.PreviewSyncHandler)))
	mux.HandleFunc("POST /api/sites/{site_id}/orchestrator/sync", middleware.WithAuth(middleware.RequireAdmin(handlers.SyncFleetHandler)))

	// ── LANDLORD / Multi-Tenant Management (SuperAdmin only) ─────────────────
	mux.HandleFunc("GET /api/landlord/tenants", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.GetTenantsHandler)))
	mux.HandleFunc("POST /api/landlord/tenants", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.CreateTenantHandler)))
	mux.HandleFunc("PUT /api/landlord/tenants/{id}/toggle", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.ToggleTenantHandler)))
	mux.HandleFunc("GET /api/landlord/tenants/{id}/stats", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.GetTenantStatsHandler)))

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
