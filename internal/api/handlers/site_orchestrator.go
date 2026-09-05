package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

const fleetSyncMaxConcurrency = 4

type fleetSyncResult struct {
	DeviceID string `json:"device_id"`
	Hostname string `json:"hostname"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	CmdCount int    `json:"cmd_count"`
}

// ─── SITE_ORCHESTRATOR Handlers ──────────────────────────────────────────────

// GetSiteConfigHandler returns the desired-state template for a site.
func GetSiteConfigHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		// No config yet — return defaults
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services.SiteConfig{
			SiteID:               siteID,
			GlobalEncryption:     "psk2",
			LanIPAddr:            "192.168.1.1",
			LanNetmask:           "255.255.255.0",
			DHCPStart:            100,
			DHCPLimit:            150,
			DHCPLeasetime:        "12h",
			DNSPrimary:           "9.9.9.9",
			DNSSecondary:         "1.1.1.1",
			Timezone:             "UTC",
			HostnamePrefix:       "nerve",
			SecureTunnelEnabled:  true,
			FirewallSynFlood:     true,
			FirewallDropInvalid:  true,
			DropbearPort:         22,
			DropbearPasswordAuth: true,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sc)
}

// PutSiteConfigHandler saves the desired-state template for a site.
func PutSiteConfigHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	username := GetUsernameFromReq(r)

	// DTO: uses json.RawMessage for JSONB blobs so Go does not attempt
	// base64-decoding (which happens with []byte fields).
	var dto struct {
		EnableGlobalSSID     bool            `json:"enable_global_ssid"`
		GlobalSSID           string          `json:"global_ssid"`
		GlobalWPAKey         string          `json:"global_wpa_key"`
		GlobalEncryption     string          `json:"global_encryption"`
		LanIPAddr            string          `json:"lan_ipaddr"`
		LanNetmask           string          `json:"lan_netmask"`
		DHCPStart            int             `json:"dhcp_start"`
		DHCPLimit            int             `json:"dhcp_limit"`
		DHCPLeasetime        string          `json:"dhcp_leasetime"`
		DNSPrimary           string          `json:"dns_primary"`
		DNSSecondary         string          `json:"dns_secondary"`
		Timezone             string          `json:"timezone"`
		HostnamePrefix       string          `json:"hostname_prefix"`
		FirewallSynFlood     bool            `json:"firewall_syn_flood"`
		FirewallDropInvalid  bool            `json:"firewall_drop_invalid"`
		DropbearPort         int             `json:"dropbear_port"`
		DropbearPasswordAuth bool            `json:"dropbear_password_auth"`
		DHCPReservations     json.RawMessage `json:"dhcp_reservations"`
		PortForwardingRules  json.RawMessage `json:"port_forwarding_rules"`
		ThreatShieldEnabled  bool            `json:"threat_shield_enabled"`
		SQMCakeEnabled       bool            `json:"sqm_cake_enabled"`
		SqmDownload          int             `json:"sqm_download"`
		SqmUpload            int             `json:"sqm_upload"`
		DPIEnabled           bool            `json:"dpi_enabled"`
		SecureTunnelEnabled  bool            `json:"secure_tunnel_enabled"`
		TailscaleEnabled     bool            `json:"tailscale_enabled"`
		TailscaleAuthKey     string          `json:"tailscale_auth_key"`
		AllowPublicSurveys   bool            `json:"allow_public_surveys"`
		BenchmarkBaseline    json.RawMessage `json:"benchmark_baseline"`
		TopologyMetadata     json.RawMessage `json:"topology_metadata"`
		HealthChecks         json.RawMessage `json:"health_checks"`
	}
	if !readBody(w, r, &dto) {
		return
	}

	// Normalize JSONB blobs — default to empty array if null/missing
	dhcpRes := []byte(dto.DHCPReservations)
	if len(dhcpRes) == 0 || string(dhcpRes) == "null" {
		dhcpRes = []byte("[]")
	}
	pfRules := []byte(dto.PortForwardingRules)
	if len(pfRules) == 0 || string(pfRules) == "null" {
		pfRules = []byte("[]")
	}

	sc := services.SiteConfig{
		SQMCakeEnabled:       dto.SQMCakeEnabled,
		SqmDownload:          dto.SqmDownload,
		SqmUpload:            dto.SqmUpload,
		DPIEnabled:           dto.DPIEnabled,
		SecureTunnelEnabled:  dto.SecureTunnelEnabled,
		TailscaleEnabled:     dto.TailscaleEnabled,
		TailscaleAuthKey:     dto.TailscaleAuthKey,
		SiteID:               siteID,
		EnableGlobalSSID:     dto.EnableGlobalSSID,
		GlobalSSID:           dto.GlobalSSID,
		GlobalWPAKey:         dto.GlobalWPAKey,
		GlobalEncryption:     dto.GlobalEncryption,
		LanIPAddr:            dto.LanIPAddr,
		LanNetmask:           dto.LanNetmask,
		DHCPStart:            dto.DHCPStart,
		DHCPLimit:            dto.DHCPLimit,
		DHCPLeasetime:        dto.DHCPLeasetime,
		DNSPrimary:           dto.DNSPrimary,
		DNSSecondary:         dto.DNSSecondary,
		Timezone:             dto.Timezone,
		HostnamePrefix:       dto.HostnamePrefix,
		FirewallSynFlood:     dto.FirewallSynFlood,
		FirewallDropInvalid:  dto.FirewallDropInvalid,
		DropbearPort:         dto.DropbearPort,
		DropbearPasswordAuth: dto.DropbearPasswordAuth,
		DHCPReservations:     dhcpRes,
		PortForwardingRules:  pfRules,
		ThreatShieldEnabled:  dto.ThreatShieldEnabled,
		AllowPublicSurveys:   dto.AllowPublicSurveys,
		BenchmarkBaseline:    dto.BenchmarkBaseline,
		TopologyMetadata:     dto.TopologyMetadata,
		HealthChecks:         dto.HealthChecks,
	}

	if err := services.UpsertSiteConfig(r.Context(), sc); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	database.InsertAuditLog(username, "SITE_ORCHESTRATOR_CONFIG_SAVE", "SITE", siteID,
		fmt.Sprintf("Updated site desired-state template (SSID: %s)", sc.GlobalSSID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}

// GetSiteDeviceRolesHandler returns devices with their assigned roles.
func GetSiteDeviceRolesHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	devs, err := services.GetSiteDevicesWithRoles(r.Context(), siteID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": devs,
	})
}

// PutDeviceRoleHandler updates a device's role (Gateway, AP, IoT_Node).
func PutDeviceRoleHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	username := GetUsernameFromReq(r)

	var body struct {
		Role string `json:"role"`
	}
	if !readBody(w, r, &body) {
		return
	}

	valid := map[string]bool{"Gateway": true, "AP": true, "IoT_Node": true}
	if !valid[body.Role] {
		http.Error(w, `{"error":"invalid role, must be Gateway, AP, or IoT_Node"}`, http.StatusBadRequest)
		return
	}

	if err := services.UpdateDeviceRole(deviceID, body.Role); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	database.InsertAuditLog(username, "SITE_ORCHESTRATOR_ROLE_CHANGE", "DEVICE", deviceID,
		fmt.Sprintf("Role changed to: %s", body.Role), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "role": body.Role})
}

// PreviewSyncHandler renders what WOULD be deployed without executing.
func PreviewSyncHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		http.Error(w, `{"error":"no site config found — save a template first"}`, http.StatusBadRequest)
		return
	}

	devs, err := services.GetSiteDevicesWithRoles(r.Context(), siteID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	results := services.RenderSiteConfig(*sc, devs)

	// Convert to preview strings per device
	type DevicePreview struct {
		DeviceID string   `json:"device_id"`
		Hostname string   `json:"hostname"`
		Role     string   `json:"role"`
		Commands []string `json:"commands"`
		Count    int      `json:"count"`
	}

	var previews []DevicePreview
	for _, r := range results {
		lines := services.PreviewCommands(r.Commands)
		previews = append(previews, DevicePreview{
			DeviceID: r.DeviceID,
			Hostname: r.Hostname,
			Role:     r.Role,
			Commands: lines,
			Count:    len(lines),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site_id": siteID,
		"devices": previews,
		"total":   len(previews),
	})
}

// SyncFleetHandler executes a staged fleet synchronization.
// It validates one canary device before pushing bounded batches to the rest.
func SyncFleetHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	username := GetUsernameFromReq(r)

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		http.Error(w, `{"error":"no site config found — save a template first"}`, http.StatusBadRequest)
		return
	}

	devs, err := services.GetSiteDevicesWithRoles(r.Context(), siteID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if len(devs) == 0 {
		http.Error(w, `{"error":"no devices found for this site"}`, http.StatusBadRequest)
		return
	}
	var healthTargets []string
	if len(sc.HealthChecks) > 0 {
		if err := json.Unmarshal(sc.HealthChecks, &healthTargets); err != nil {
			http.Error(w, `{"error":"invalid health_checks configuration"}`, http.StatusBadRequest)
			return
		}
	}
	healthTargets, err = services.ValidateHealthTargets(healthTargets)
	if err != nil {
		http.Error(w, `{"error":"invalid health check target"}`, http.StatusBadRequest)
		return
	}

	results := services.RenderSiteConfig(*sc, devs)
	rolloutID, generation, err := createRolloutRun(r, siteID, username, results)
	if err != nil {
		http.Error(w, `{"error":"could not create rollout run"}`, http.StatusInternalServerError)
		return
	}

	// Execute the first device as a canary, then process the remaining devices
	// in bounded batches only after the canary succeeds.
	syncResults := make([]fleetSyncResult, len(results))
	executeBatch := func(indices []int) {
		jobs := make(chan int)
		workerCount := fleetSyncMaxConcurrency
		if len(indices) < workerCount {
			workerCount = len(indices)
		}
		var wg sync.WaitGroup
		for worker := 0; worker < workerCount; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range jobs {
					rr := results[idx]

					sr := fleetSyncResult{
						DeviceID: rr.DeviceID,
						Hostname: rr.Hostname,
						Role:     rr.Role,
						CmdCount: len(rr.Commands),
					}

					if len(rr.Commands) == 0 {
						sr.Status = "SKIPPED"
						sr.Output = "No commands to execute"
						syncResults[idx] = sr
						continue
					}

					// Group commands by config namespace to build per-namespace batch scripts
					configGroups := groupCommandsByConfig(rr.Commands)
					var allOutput string
					for _, cfg := range sortedConfigNames(configGroups) {
						cmds := configGroups[cfg]
						script := services.BuildSafeBatchScript(cfg, cmds, healthTargets)
						out, err := runSSHScript(rr.DeviceID, script)
						allOutput += out + "\n"
						if err != nil {
							sr.Status = "FAILED"
							sr.Error = err.Error()
							sr.Output = allOutput
							syncResults[idx] = sr
							break
						}
					}
					if sr.Status == "FAILED" {
						continue
					}
					sr.Status = "SUCCESS"
					sr.Output = allOutput
					syncResults[idx] = sr
					log.Printf("[SITE_ORCHESTRATOR] Synced %s (%s) - %d commands", rr.Hostname, rr.Role, len(rr.Commands))
				}
			}()
		}
		for _, idx := range indices {
			jobs <- idx
		}
		close(jobs)
		wg.Wait()
	}

	phases := fleetRolloutPhases(len(results))
	executeBatch(phases[0])
	canaryOK := syncResults[phases[0][0]].Status == "SUCCESS"
	rolloutStatus := "completed"
	if !canaryOK {
		rolloutStatus = "canary_failed"
		for _, phase := range phases[1:] {
			for _, idx := range phase {
				syncResults[idx] = fleetSyncResult{DeviceID: results[idx].DeviceID, Hostname: results[idx].Hostname, Role: results[idx].Role, Status: "ABORTED", CmdCount: len(results[idx].Commands), Error: "canary failed; rollout not started"}
			}
		}
	} else {
		for _, phase := range phases[1:] {
			executeBatch(phase)
		}
	}
	if err := updateRolloutRun(r, rolloutID, rolloutStatus, syncResults); err != nil {
		log.Printf("[SITE_ORCHESTRATOR][WARN] failed to persist rollout %s: %v", rolloutID, err)
	}

	// Count successes/failures
	successes, failures := 0, 0
	for _, sr := range syncResults {
		if sr.Status == "SUCCESS" || sr.Status == "SKIPPED" {
			successes++
		} else {
			failures++
		}
	}

	database.InsertAuditLog(username, "SITE_ORCHESTRATOR_FLEET_SYNC", "SITE", siteID,
		fmt.Sprintf("Fleet sync: %d success, %d failed, SSID: %s", successes, failures, sc.GlobalSSID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     rolloutStatus,
		"rollout_id": rolloutID,
		"generation": generation,
		"successes":  successes,
		"failures":   failures,
		"results":    syncResults,
	})
}

func createRolloutRun(r *http.Request, siteID, username string, results []services.RenderResult) (string, int64, error) {
	targets := make([]string, len(results))
	for i, result := range results {
		targets[i] = result.DeviceID
	}
	planHash := fleetPlanHash(results)
	targetDeviceIDs, err := json.Marshal(targets)
	if err != nil {
		return "", 0, err
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		return "", 0, err
	}
	tx, err := database.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", siteID); err != nil {
		return "", 0, err
	}
	var generation int64
	err = tx.QueryRowContext(r.Context(), "SELECT COALESCE(MAX(generation), 0) + 1 FROM "+schema+".rollout_runs WHERE site_id = $1", siteID).Scan(&generation)
	if err != nil {
		return "", 0, err
	}
	var id string
	err = tx.QueryRowContext(r.Context(), "INSERT INTO "+schema+".rollout_runs (site_id, generation, status, plan_hash, requested_by, target_device_ids) VALUES ($1, $2, 'RUNNING', $3, $4, $5) RETURNING id", siteID, generation, planHash, username, targetDeviceIDs).Scan(&id)
	if err != nil {
		return "", 0, err
	}
	return id, generation, tx.Commit()
}

func fleetPlanHash(results []services.RenderResult) string {
	encoded, _ := json.Marshal(results)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func updateRolloutRun(r *http.Request, rolloutID, status string, results []fleetSyncResult) error {
	schema, err := getTenantSchema(r)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(stripFleetSyncOutput(results))
	if err != nil {
		return err
	}
	tx, err := database.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "UPDATE "+schema+".rollout_runs SET status = $1, results = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3", status, encoded, rolloutID); err != nil {
		return err
	}
	return tx.Commit()
}

func stripFleetSyncOutput(results []fleetSyncResult) []fleetSyncResult {
	clean := make([]fleetSyncResult, len(results))
	copy(clean, results)
	for i := range clean {
		clean[i].Output = ""
	}
	return clean
}

// groupCommandsByConfig splits commands into per-namespace buckets for batch execution.
func groupCommandsByConfig(cmds []services.UciCommand) map[string][]services.UciCommand {
	groups := make(map[string][]services.UciCommand)
	for _, cmd := range cmds {
		groups[cmd.Config] = append(groups[cmd.Config], cmd)
	}
	return groups
}

func sortedConfigNames(groups map[string][]services.UciCommand) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fleetRolloutPhases(deviceCount int) [][]int {
	if deviceCount == 0 {
		return nil
	}
	phases := [][]int{{0}}
	for start := 1; start < deviceCount; start += fleetSyncMaxConcurrency {
		end := start + fleetSyncMaxConcurrency
		if end > deviceCount {
			end = deviceCount
		}
		phase := make([]int, end-start)
		for offset := range phase {
			phase[offset] = start + offset
		}
		phases = append(phases, phase)
	}
	return phases
}
