package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

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

	if req.DeviceID == "" || req.Download <= 0 || req.Upload <= 0 {
		http.Error(w, `{"error": "invalid parameters"}`, http.StatusBadRequest)
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

	if req.Clear {
		if err := services.ClearShaping(req.DeviceID, req.MAC); err != nil {
			http.Error(w, fmt.Sprintf("Clear shaping failed: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
		return
	}

	if err := services.ApplySniperShaping(req.DeviceID, req.MAC, req.Rate, req.Duration); err != nil {
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

	query := `SELECT id, name, state_json FROM devices WHERE site_id = $1`
	rows, err := database.DB.Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id string
		var name *string  // use pointer to handle NULL
		var stateJSON []byte
		if err := rows.Scan(&id, &name, &stateJSON); err != nil {
			continue
		}
		devName := id // fallback to ID if no name
		if name != nil && *name != "" {
			devName = *name
		}

		var state map[string]interface{}
		if len(stateJSON) > 0 {
			if err := json.Unmarshal(stateJSON, &state); err != nil {
				state = map[string]interface{}{}
			}
		} else {
			state = map[string]interface{}{}
		}

		// Flatten wireless_stations into a client list
		var clients []map[string]interface{}
		if ws, ok := state["wireless_stations"].(map[string]interface{}); ok {
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
							clients = append(clients, entry)
						}
					}
				}
			}
		}
		if clients == nil {
			clients = make([]map[string]interface{}, 0)
		}

		devData := map[string]interface{}{
			"device_id":        id,
			"name":             devName,
			"top_talkers":      state["top_talkers"],
			"iface_stats":      state["iface_stats"],
			"wireless_clients": clients,
		}
		result = append(result, devData)
	}

	if result == nil {
		result = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": result,
	})
}
