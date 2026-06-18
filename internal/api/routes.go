package api

import (
	"net/http"

	"openwrt-controller/internal/api/handlers"
	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/api/spa"
	"openwrt-controller/internal/metrics"
)

// Metrics is the global metrics registry, initialised in main and
// injected into the api package via SetupRoutes. It is exposed so
// other packages can record custom metrics.
var Metrics *metrics.Registry

// SetMetrics wires the global metrics registry. Must be called once
// from main before serving traffic. Idempotent.
func SetMetrics(m *metrics.Registry) { Metrics = m }

func SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// ── Public routes (no auth) ──────────────────────────────────────────────
	mux.HandleFunc("POST /api/auth/login", handlers.LoginHandler)
	mux.HandleFunc("POST /api/telemetry", handlers.TelemetryHandler)
	mux.HandleFunc("GET /api/devices/{device_id}/config", handlers.GetDeviceConfigHandler)

	// ── Container probes (no auth) ───────────────────────────────────────────
	// /healthz  — liveness. 200 as long as the process is up.
	// /readyz   — readiness. 200 only when DB is reachable.
	mux.HandleFunc("GET /healthz", handlers.HealthzHandler)
	mux.HandleFunc("GET /readyz", handlers.ReadyzHandler)

	// /metrics  — Prometheus exposition format. Anonymous by design so
	// a sidecar scraper does not need a JWT. Restrict access at the
	// network layer (firewall, mesh policy) if needed.
	if Metrics != nil {
		mux.Handle("GET /metrics", Metrics.Handler())
	}

	// ── WebSocket ticket issuance (auth via JWT in Authorization header) ──
	// The dashboard calls this to mint a short-lived single-use ticket
	// that it then exchanges for a WebSocket upgrade. Avoids putting
	// the JWT in the URL.
	mux.HandleFunc("POST /api/ws-ticket", middleware.WithAuth(handlers.IssueWSTicketHandler))

	// ── Public Guest Portal routes ───────────────────────────────────────────
	mux.HandleFunc("GET /portal/auth/{site_id}", handlers.GetPortalAuthHandler)
	mux.HandleFunc("POST /api/public/portal/{site_id}/validate", handlers.ValidatePortalHandler)

	// ── WIFI_SURVEY / public sample ingest (X-Survey-Token auth, no JWT) ─────
	mux.HandleFunc("POST /api/surveys/{id}/samples", handlers.PostSurveySampleHandler)

	// ── Protected API routes ─────────────────────────────────────────────────
	mux.HandleFunc("GET /api/global/health", middleware.WithAuth(handlers.GetGlobalHealthHandler))
	mux.HandleFunc("GET /api/global/sentinel", middleware.WithAuth(handlers.GetSentinelInsightsHandler))
	mux.HandleFunc("POST /api/global/sentinel/trigger", middleware.WithAuth(handlers.TriggerManualSentinelHandler))
	mux.HandleFunc("GET /api/global/settings", middleware.WithAuth(handlers.GetPlatformSettingsHandler))
	mux.HandleFunc("POST /api/global/settings", middleware.WithAuth(handlers.UpdatePlatformSettingsHandler))
	mux.HandleFunc("POST /api/chatops/query", middleware.WithAuth(handlers.ChatOpsQueryHandler))
	mux.HandleFunc("GET /api/sites", middleware.WithAuth(handlers.GetSitesHandler))
	mux.HandleFunc("PUT /api/sites/{site_id}/location", middleware.WithAuth(handlers.UpdateSiteLocationHandler))
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
	mux.HandleFunc("POST /api/devices/{device_id}/pcap", middleware.WithAuth(handlers.CapturePacketHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/iperf", middleware.WithAuth(handlers.RunIperfHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/devices", middleware.WithAuth(handlers.GetSiteDevicesHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/adopt", middleware.WithAuth(handlers.AdoptDeviceHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/migrate", middleware.WithAuth(handlers.MigrateDeviceHandler))
	mux.HandleFunc("POST /api/devices/{device_id}/import-config", middleware.WithAuth(middleware.RequireAdmin(handlers.ImportDeviceConfigHandler)))

	mux.HandleFunc("POST /api/sites/{site_id}/wlans", middleware.WithAuth(handlers.CreateWLANHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/wlans", middleware.WithAuth(handlers.GetWLANsHandler))
	mux.HandleFunc("PUT /api/sites/{site_id}/wlans/{wlan_id}", middleware.WithAuth(handlers.UpdateWLANHandler))
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
	mux.HandleFunc("POST /api/orchestrator/command", middleware.WithAuth(middleware.RequireAdmin(handlers.MassCommandHandler)))

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

	// ── VPN MESH ORCHESTRATION ─────────────────────────────────────────────

	mux.HandleFunc("GET /api/radius/users", middleware.WithAuth(handlers.GetRadiusUsersHandler))
	mux.HandleFunc("POST /api/radius/users", middleware.WithAuth(handlers.CreateRadiusUserHandler))
	mux.HandleFunc("DELETE /api/radius/users", middleware.WithAuth(handlers.DeleteRadiusUserHandler))

	mux.HandleFunc("GET /api/webhooks", middleware.WithAuth(middleware.RequireAdmin(handlers.GetWebhooksHandler)))
	mux.HandleFunc("POST /api/webhooks", middleware.WithAuth(middleware.RequireAdmin(handlers.CreateWebhookHandler)))
	mux.HandleFunc("DELETE /api/webhooks/{webhook_id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteWebhookHandler)))

	mux.HandleFunc("GET /api/vpn-meshes", middleware.WithAuth(middleware.RequireAdmin(handlers.GetVPNMeshesHandler)))

	mux.HandleFunc("POST /api/vpn-meshes", middleware.WithAuth(middleware.RequireAdmin(handlers.CreateVPNMeshHandler)))

	mux.HandleFunc("DELETE /api/vpn-meshes/{mesh_id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteVPNMeshHandler)))

	mux.HandleFunc("GET /api/vpn-meshes/{mesh_id}/nodes", middleware.WithAuth(middleware.RequireAdmin(handlers.GetVPNMeshNodesHandler)))

	mux.HandleFunc("POST /api/vpn-meshes/{mesh_id}/nodes", middleware.WithAuth(middleware.RequireAdmin(handlers.AddVPNMeshNodeHandler)))

	mux.HandleFunc("DELETE /api/vpn-mesh-nodes/{node_id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteVPNMeshNodeHandler)))
	mux.HandleFunc("POST /api/vpn-meshes/{mesh_id}/sync", middleware.WithAuth(middleware.RequireAdmin(handlers.SyncVPNMeshHandler)))

	// ── LANDLORD / Multi-Tenant Management (SuperAdmin only) ─────────────────
	mux.HandleFunc("GET /api/landlord/tenants", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.GetTenantsHandler)))
	mux.HandleFunc("POST /api/landlord/tenants", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.CreateTenantHandler)))
	mux.HandleFunc("PUT /api/landlord/tenants/{id}/toggle", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.ToggleTenantHandler)))
	mux.HandleFunc("GET /api/billing/usage", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.GetBillingUsageHandler)))
	mux.HandleFunc("GET /api/landlord/tenants/{id}/stats", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.GetTenantStatsHandler)))

	// ── WIFI_SURVEY / site-scoped CRUD (admin) ──────────────────────────────
	mux.HandleFunc("POST /api/sites/{site_id}/surveys", middleware.WithAuth(handlers.CreateSurveyHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/surveys", middleware.WithAuth(handlers.ListSiteSurveysHandler))
	mux.HandleFunc("GET /api/sites/{site_id}/surveys/{survey_id}", middleware.WithAuth(handlers.GetSurveyHandler))
	mux.HandleFunc("DELETE /api/sites/{site_id}/surveys/{survey_id}", middleware.WithAuth(middleware.RequireAdmin(handlers.DeleteSurveyHandler)))
	mux.HandleFunc("POST /api/sites/{site_id}/surveys/{survey_id}/start", middleware.WithAuth(handlers.StartSurveyHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/surveys/{survey_id}/stop", middleware.WithAuth(handlers.StopSurveyHandler))
	mux.HandleFunc("POST /api/sites/{site_id}/surveys/{survey_id}/rotate-token", middleware.WithAuth(middleware.RequireAdmin(handlers.RotateSurveyTokenHandler)))
	mux.HandleFunc("POST /api/sites/{site_id}/surveys/{survey_id}/revoke-token", middleware.WithAuth(middleware.RequireAdmin(handlers.RevokeSurveyTokenHandler)))
	mux.HandleFunc("GET /api/sites/{site_id}/surveys/{survey_id}/samples", middleware.WithAuth(handlers.GetSurveyPointsHandler))
	mux.HandleFunc("PUT /api/global/surveys/lockdown", middleware.WithAuth(middleware.RequireSuperAdmin(handlers.SetGlobalSurveyLockdownHandler)))

	// ── Agent Management ─────────────────────────────────────────────────────
	// Device-facing: authenticated by X-Site-Key header (no JWT)
	mux.HandleFunc("GET /api/agent/latest", handlers.GetLatestAgentHandler)
	mux.HandleFunc("GET /api/agent/latest/raw", handlers.GetLatestAgentRawHandler)
	// Dashboard-facing: JWT required
	mux.HandleFunc("POST /api/agent/deploy", middleware.WithAuth(handlers.DeployAgentHandler))
	mux.HandleFunc("GET /api/agent/status", middleware.WithAuth(handlers.GetAgentVersionsStatusHandler))
	mux.HandleFunc("GET /api/agent/site/raw", middleware.WithAuth(handlers.GetSiteAgentRawHandler))

	// ── SPA Static files ─────────────────────────────────────────────────────
	// Extracted into internal/api/spa so the asset/SPA-fallback rules
	// are unit-tested. In particular: a path that LOOKS like a static
	// asset (any last-segment with a file extension) must return 404
	// when the file is missing, NOT fall through to index.html. A
	// stale browser cache requesting an old bundle name was getting
	// text/html back, which the module loader refuses to execute
	// ("Expected a JavaScript-or-Wasm module script but the server
	// responded with a MIME type of text/html").
	mux.Handle("/", spa.NewHandler("./web/dist"))

	return mux
}

