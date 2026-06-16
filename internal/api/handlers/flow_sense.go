package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"openwrt-controller/internal/database"
)

// knownBadPorts are ports we flag as suspicious in the UI
var knownBadPorts = map[int]bool{
	6881: true, 6889: true, // BitTorrent
	4662: true, 4672: true, // eMule
	51413: true,             // Transmission
	1194:  true,             // Generic VPN tunnel (unlicensed)
	9001:  true, 9030: true, // Tor relay/dir
	4444: true, 31337: true, 8888: true, // C2 / metasploit
}
var standardPorts = map[int]bool{
	80: true, 443: true, 53: true, 123: true, 22: true,
	25: true, 465: true, 587: true, 993: true, 995: true,
}

func isPrivateIPStr(ip string) bool {
	return strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "172.") || strings.HasPrefix(ip, "127.") ||
		strings.HasPrefix(ip, "169.254.") || strings.HasPrefix(ip, "10.8.")
}

// GetSiteFlowSenseHandler reads flow_sense from each device's state_json and
// returns enriched (flagged) flow data for the FlowRadar UI.
func GetSiteFlowSenseHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"site_id required"}`, http.StatusBadRequest)
		return
	}

	// COALESCE(name, id) avoids NULL scan failures for unnamed devices
	rows, err := database.Tx(r.Context()).Query(`
		SELECT id, COALESCE(name, id) AS device_name, state_json
		FROM devices
		WHERE site_id = $1 AND state_json IS NOT NULL
	`, siteID)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type FlowEntry map[string]interface{}
	type DeviceFlows struct {
		DeviceID   string      `json:"device_id"`
		DeviceName string      `json:"device_name"`
		Flows      []FlowEntry `json:"flows"`
	}

	var result []DeviceFlows

	for rows.Next() {
		var id, name string
		var stateJSON []byte
		if err := rows.Scan(&id, &name, &stateJSON); err != nil {
			continue
		}

		var state map[string]interface{}
		if err := json.Unmarshal(stateJSON, &state); err != nil {
			result = append(result, DeviceFlows{DeviceID: id, DeviceName: name, Flows: []FlowEntry{}})
			continue
		}

		rawFlows, ok := state["flow_sense"].([]interface{})
		if !ok || len(rawFlows) == 0 {
			result = append(result, DeviceFlows{DeviceID: id, DeviceName: name, Flows: []FlowEntry{}})
			continue
		}

		var flows []FlowEntry
		for _, rawF := range rawFlows {
			m, ok := rawF.(map[string]interface{})
			if !ok {
				continue
			}

			// Normalise types from JSON
			dst := ""
			if v, ok := m["dst"].(string); ok {
				dst = v
			}
			dport := 0
			if v, ok := m["dport"].(float64); ok {
				dport = int(v)
			}
			conns := 0
			if v, ok := m["conns"].(float64); ok {
				conns = int(v)
			}

			// Enrichment logic (server-side, no race condition)
			flagged := false
			reason := ""

			if !isPrivateIPStr(dst) {
				if knownBadPorts[dport] {
					flagged = true
					reason = "SUSPECTED_P2P_OR_TUNNEL port " + http.StatusText(dport)
					if dport == 9001 || dport == 9030 {
						reason = "SUSPECTED_TOR"
					} else if dport == 4444 || dport == 31337 {
						reason = "SUSPECTED_C2"
					} else if dport == 6881 || dport == 4662 {
						reason = "SUSPECTED_P2P"
					}
				} else if conns > 50 && !standardPorts[dport] {
					flagged = true
					reason = "HIGH_CONN_COUNT_NON_STD_PORT"
				}
			}

			entry := FlowEntry{
				"proto":      m["proto"],
				"dst":        dst,
				"dport":      dport,
				"conns":      conns,
				"sample_src": m["sample_src"],
				"flagged":    flagged,
				"reason":     reason,
			}
			flows = append(flows, entry)
		}

		if flows == nil {
			flows = []FlowEntry{}
		}

		result = append(result, DeviceFlows{DeviceID: id, DeviceName: name, Flows: flows})
	}

	if result == nil {
		result = []DeviceFlows{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
