package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// GetThreatShieldStatusHandler returns intel metadata (admin dashboard).
// GET /api/threat-shield/status
func GetThreatShieldStatusHandler(w http.ResponseWriter, r *http.Request) {
	status := services.GetThreatIntelStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetThreatShieldListHandler serves the raw blocklist to agents via X-Site-Key.
// GET /api/threat-shield/list
func GetThreatShieldListHandler(w http.ResponseWriter, r *http.Request) {
	siteKey := r.Header.Get("X-Site-Key")
	if siteKey == "" {
		http.Error(w, "Forbidden: missing X-Site-Key", http.StatusForbidden)
		return
	}

	var siteID string
	err := database.Tx(r.Context()).QueryRow(
		"SELECT id FROM sites WHERE api_key = $1", siteKey,
	).Scan(&siteID)
	if err != nil || siteID == "" {
		http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
		return
	}

	// Only serve if the site has threat_shield enabled
	var enabled bool
	_ = database.Tx(r.Context()).QueryRow(
		"SELECT COALESCE(threat_shield_enabled, false) FROM sites WHERE id = $1", siteID,
	).Scan(&enabled)
	if !enabled {
		http.Error(w, "Threat Shield not enabled for this site", http.StatusForbidden)
		return
	}

	content := services.GetThreatListContent()
	if content == "" {
		http.Error(w, "Threat intel not yet available. Try again in a minute.", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-IP-Count", "see Content-Length")
	w.Write([]byte(content))
}

// ToggleThreatShieldHandler enables or disables threat shield for a site.
// POST /api/sites/{site_id}/threat-shield
func ToggleThreatShieldHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	_, err := database.Tx(r.Context()).Exec(
		"UPDATE sites SET threat_shield_enabled = $1 WHERE id = $2",
		req.Enabled, siteID,
	)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site_id": siteID,
		"enabled": req.Enabled,
	})
}

// GetSiteThreatShieldHandler returns per-site threat shield status + global intel metadata.
// GET /api/sites/{site_id}/threat-shield
func GetSiteThreatShieldHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	var enabled bool
	err := database.Tx(r.Context()).QueryRow(
		"SELECT COALESCE(threat_shield_enabled, false) FROM sites WHERE id = $1", siteID,
	).Scan(&enabled)
	if err != nil {
		http.Error(w, `{"error":"site not found"}`, http.StatusNotFound)
		return
	}

	// Collect per-device drop stats
	rows, _ := database.Tx(r.Context()).Query(`
		SELECT id, COALESCE(name, id), COALESCE(threat_shield_drops, 0)
		FROM devices
		WHERE site_id = $1
	`, siteID)
	defer rows.Close()

	type DeviceStat struct {
		DeviceID string `json:"device_id"`
		Name     string `json:"name"`
		Drops    int64  `json:"drops"`
	}
	var deviceStats []DeviceStat
	if rows != nil {
		for rows.Next() {
			var d DeviceStat
			if err := rows.Scan(&d.DeviceID, &d.Name, &d.Drops); err == nil {
				deviceStats = append(deviceStats, d)
			}
		}
	}
	if deviceStats == nil {
		deviceStats = []DeviceStat{}
	}

	intel := services.GetThreatIntelStatus()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site_id":      siteID,
		"enabled":      enabled,
		"intel":        intel,
		"device_stats": deviceStats,
	})
}
