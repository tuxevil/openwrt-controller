package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"
)

// DetectSurveyorHandler identifies the Wi-Fi client opening the survey form.
func DetectSurveyorHandler(w http.ResponseWriter, r *http.Request) {
	_, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") || middleware.GetTenantSchema(r) == "" {
		http.Error(w, "Operator access required", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	// Use the direct peer address. Forwarded headers are insufficient evidence
	// to identify the browser's client in the site's station telemetry.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsGlobalUnicast() {
		writeJSON(w, http.StatusOK, map[string]string{"surveyor_mac": ""})
		return
	}

	rows, err := database.Tx(r.Context()).QueryContext(r.Context(), `
		SELECT state_json FROM devices
		WHERE site_id = $1 AND state_json IS NOT NULL
		  AND updated_at >= CURRENT_TIMESTAMP - INTERVAL '45 seconds'
	`, r.PathValue("site_id"))
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "Unable to detect Wi-Fi client", err)
		return
	}
	defer rows.Close()

	var states [][]byte
	for rows.Next() {
		var state []byte
		if err := rows.Scan(&state); err != nil {
			RespondError(w, http.StatusInternalServerError, "Unable to read Wi-Fi telemetry", err)
			return
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		RespondError(w, http.StatusInternalServerError, "Unable to read Wi-Fi telemetry", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"surveyor_mac": detectSurveyMAC(ip, states)})
}

type surveyNeighbor struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"`
}

func detectSurveyMAC(peer netip.Addr, states [][]byte) string {
	neighbors := make(map[string]bool)
	wireless := make(map[string]bool)
	for _, state := range states {
		var node struct {
			ARP          []surveyNeighbor `json:"arp_table"`
			NeighborData struct {
				ARP []surveyNeighbor `json:"arp_table"`
			} `json:"neighbor_stats"`
			Stations map[string][]struct {
				MAC string `json:"mac"`
			} `json:"wireless_stations"`
		}
		if json.Unmarshal(state, &node) != nil {
			continue
		}
		for _, entry := range append(node.NeighborData.ARP, node.ARP...) {
			ip, err := netip.ParseAddr(entry.IP)
			if err == nil && ip.Unmap() == peer.Unmap() {
				mac, err := net.ParseMAC(entry.MAC)
				if err == nil && len(mac) == 6 && mac[0]&1 == 0 && mac.String() != "00:00:00:00:00:00" {
					neighbors[mac.String()] = true
				}
			}
		}
		for _, stations := range node.Stations {
			for _, station := range stations {
				if mac, err := net.ParseMAC(station.MAC); err == nil {
					wireless[mac.String()] = true
				}
			}
		}
	}
	if len(neighbors) == 1 {
		for mac := range neighbors {
			if wireless[mac] {
				return mac
			}
		}
	}
	return ""
}
