package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// ─── SITE_ORCHESTRATOR Handlers ──────────────────────────────────────────────

// GetSiteConfigHandler returns the desired-state template for a site.
func GetSiteConfigHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		// No config yet — return defaults
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services.SiteConfig{
			SiteID:              siteID,
			GlobalEncryption:    "psk2",
			LanIPAddr:           "192.168.1.1",
			LanNetmask:          "255.255.255.0",
			DHCPStart:           100,
			DHCPLimit:           150,
			DHCPLeasetime:       "12h",
			DNSPrimary:          "9.9.9.9",
			DNSSecondary:        "1.1.1.1",
			Timezone:            "UTC",
			HostnamePrefix:      "nerve",
			FirewallSynFlood:    true,
			FirewallDropInvalid: true,
			DropbearPort:        22,
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
		SiteID:               siteID,
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

	devs, err := services.GetSiteDevicesWithRoles(siteID)
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

	devs, err := services.GetSiteDevicesWithRoles(siteID)
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
		"site_id":  siteID,
		"devices":  previews,
		"total":    len(previews),
	})
}

// SyncFleetHandler executes the full fleet synchronization.
// It renders UCI commands per role, then pushes batch scripts to each device in parallel.
func SyncFleetHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	username := GetUsernameFromReq(r)

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		http.Error(w, `{"error":"no site config found — save a template first"}`, http.StatusBadRequest)
		return
	}

	devs, err := services.GetSiteDevicesWithRoles(siteID)
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

	results := services.RenderSiteConfig(*sc, devs)

	// Execute in parallel, collect results
	type SyncResult struct {
		DeviceID string `json:"device_id"`
		Hostname string `json:"hostname"`
		Role     string `json:"role"`
		Status   string `json:"status"`
		Output   string `json:"output"`
		Error    string `json:"error,omitempty"`
		CmdCount int    `json:"cmd_count"`
	}

	var wg sync.WaitGroup
	syncResults := make([]SyncResult, len(results))

	for i, res := range results {
		wg.Add(1)
		go func(idx int, rr services.RenderResult) {
			defer wg.Done()

			sr := SyncResult{
				DeviceID: rr.DeviceID,
				Hostname: rr.Hostname,
				Role:     rr.Role,
				CmdCount: len(rr.Commands),
			}

			if len(rr.Commands) == 0 {
				sr.Status = "SKIPPED"
				sr.Output = "No commands to execute"
				syncResults[idx] = sr
				return
			}

			// Group commands by config namespace to build per-namespace batch scripts
			configGroups := groupCommandsByConfig(rr.Commands)

			var allOutput string
			for cfg, cmds := range configGroups {
				script := services.BuildBatchScript(cfg, cmds)
				out, err := runSSHScript(rr.DeviceID, script)
				allOutput += out + "\n"
				if err != nil {
					sr.Status = "FAILED"
					sr.Error = err.Error()
					sr.Output = allOutput
					syncResults[idx] = sr
					return
				}
			}

			sr.Status = "SUCCESS"
			sr.Output = allOutput
			syncResults[idx] = sr

			log.Printf("[SITE_ORCHESTRATOR] ✓ Synced %s (%s) — %d commands", rr.Hostname, rr.Role, len(rr.Commands))
		}(i, res)
	}

	wg.Wait()

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
		"status":    "completed",
		"successes": successes,
		"failures":  failures,
		"results":   syncResults,
	})
}

// groupCommandsByConfig splits commands into per-namespace buckets for batch execution.
func groupCommandsByConfig(cmds []services.UciCommand) map[string][]services.UciCommand {
	groups := make(map[string][]services.UciCommand)
	for _, cmd := range cmds {
		groups[cmd.Config] = append(groups[cmd.Config], cmd)
	}
	return groups
}
