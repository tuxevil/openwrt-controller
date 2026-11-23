package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"openwrt-controller/internal/api/middleware"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

type LimitBandwidthRequest struct {
	DeviceID string `json:"device_id"`
	MAC      string `json:"mac"`
	Download int    `json:"download"`
	Upload   int    `json:"upload"`
}

func LimitBandwidthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req LimitBandwidthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "bad request"}`, http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, `{"error": "invalid parameters"}`, http.StatusBadRequest)
		return
	}
	if err := services.ValidateBandwidth(req.Download, req.Upload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := services.LimitBandwidth(req.DeviceID, req.MAC, req.Download, req.Upload)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "success"})
}

type SniperRequest struct {
	DeviceID string `json:"device_id"`
	MAC      string `json:"mac"`
	Rate     int    `json:"rate_mbytes"`
	Duration int    `json:"duration_minutes"`
	Clear    bool   `json:"clear,omitempty"`
}

func SniperBandwidthHandler(w http.ResponseWriter, r *http.Request) {
	var req SniperRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if req.DeviceID == "" || req.MAC == "" {
		http.Error(w, "device_id and mac are required", http.StatusBadRequest)
		return
	}
	if !services.ValidMACAddress(req.MAC) {
		http.Error(w, "mac must be a valid six-octet address", http.StatusBadRequest)
		return
	}

	if req.Clear {
		if err := services.ClearShaping(middleware.GetTenantSchema(r), req.DeviceID, req.MAC); err != nil {
			http.Error(w, fmt.Sprintf("Clear shaping failed: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
		return
	}

	if err := services.ApplySniperShaping(middleware.GetTenantSchema(r), req.DeviceID, req.MAC, req.Rate, req.Duration); err != nil {
		if err.Error() == "Incompatible Engine: nftables not supported on this device" {
			http.Error(w, err.Error(), http.StatusNotImplemented)
		} else {
			http.Error(w, fmt.Sprintf("Sniper shaping failed: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "applied"})
}

func BandwidthStatsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.URL.Query().Get("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id required"}`, http.StatusBadRequest)
		return
	}

	lookup := database.LoadSiteClientHostnameLookup(r.Context(), siteID)

	query := `SELECT id, name, COALESCE(last_ip, ''), state_json FROM devices WHERE site_id = $1`
	rows, err := database.Tx(r.Context()).Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
		return
	}

	type rawDeviceRow struct {
		id     string
		name   *string
		lastIP string
		state  map[string]interface{}
	}
	var devRows []rawDeviceRow
	for rows.Next() {
		var id string
		var name *string // use pointer to handle NULL
		var lastIP string
		var stateJSON []byte
		if err := rows.Scan(&id, &name, &lastIP, &stateJSON); err != nil {
			continue
		}
		var state map[string]interface{}
		if len(stateJSON) > 0 {
			if err := json.Unmarshal(stateJSON, &state); err != nil {
				state = map[string]interface{}{}
			}
		} else {
			state = map[string]interface{}{}
		}
		devName := ""
		if name != nil {
			devName = *name
		}
		lookup.MergeDeviceState(id, devName, lastIP, state)
		devRows = append(devRows, rawDeviceRow{id: id, name: name, lastIP: lastIP, state: state})
	}
	rows.Close()

	// Build MAC -> IP lookup from merged device states so clients carry ip_address when known
	macToIP := make(map[string]string)
	for ip, host := range lookup.ByIP {
		for mac, mHost := range lookup.ByMAC {
			if mHost == host && macToIP[mac] == "" {
				macToIP[mac] = ip
			}
		}
	}

	var result []map[string]interface{}
	for _, dev := range devRows {
		devName := database.DeviceDisplayName(dev.id, dev.name, dev.state, lookup)

		// Flatten wireless_stations into a client list
		var clients []map[string]interface{}
		if ws, ok := dev.state["wireless_stations"].(map[string]interface{}); ok {
			for iface, stList := range ws {
				if stations, ok := stList.([]interface{}); ok {
					for _, s := range stations {
						if sm, ok := s.(map[string]interface{}); ok {
							entry := map[string]interface{}{
								"iface": iface,
							}
							for k, v := range sm {
								entry[k] = v
							}
							if macStr, ok := entry["mac"].(string); ok && macStr != "" {
								if h := lookup.ResolveMAC(macStr); h != "" {
									entry["hostname"] = h
								}
								if ip := macToIP[database.NormalizeMAC(macStr)]; ip != "" {
									entry["ip_address"] = ip
								}
							}
							clients = append(clients, entry)
						}
					}
				}
			}
		}
		if clients == nil {
			clients = make([]map[string]interface{}, 0)
		}

		// Enrich top_talkers with hostnames when present
		topTalkers := dev.state["top_talkers"]
		if talkerList, ok := topTalkers.([]interface{}); ok {
			enrichedTalkers := make([]interface{}, 0, len(talkerList))
			for _, rawT := range talkerList {
				if tm, ok := rawT.(map[string]interface{}); ok {
					cp := make(map[string]interface{}, len(tm)+1)
					for k, v := range tm {
						cp[k] = v
					}
					if macStr, ok := cp["mac"].(string); ok && macStr != "" {
						if h := lookup.Resolve(macStr); h != "" {
							cp["hostname"] = h
						}
					}
					enrichedTalkers = append(enrichedTalkers, cp)
				} else {
					enrichedTalkers = append(enrichedTalkers, rawT)
				}
			}
			topTalkers = enrichedTalkers
		}

		devData := map[string]interface{}{
			"device_id":        dev.id,
			"name":             devName,
			"top_talkers":      topTalkers,
			"iface_stats":      dev.state["iface_stats"],
			"wireless_clients": clients,
		}
		result = append(result, devData)
	}

	if result == nil {
		result = make([]map[string]interface{}, 0)
	}

	var shapedMacs []string
	rowsShapes, err := database.Tx(r.Context()).Query(`
		SELECT sr.mac 
		FROM shaping_rules sr
		JOIN devices d ON sr.device_id = d.id
		WHERE d.site_id = $1
	`, siteID)
	if err == nil {
		defer rowsShapes.Close()
		for rowsShapes.Next() {
			var m string
			if err := rowsShapes.Scan(&m); err == nil {
				shapedMacs = append(shapedMacs, m)
			}
		}
	}
	if shapedMacs == nil {
		shapedMacs = make([]string, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":        result,
		"shaped_macs": shapedMacs,
	})
}
