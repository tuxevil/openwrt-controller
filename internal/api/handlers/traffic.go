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
		var id, name string
		var stateJSON []byte
		if err := rows.Scan(&id, &name, &stateJSON); err == nil {
			var state map[string]interface{}
			if len(stateJSON) > 0 {
				if err := json.Unmarshal(stateJSON, &state); err == nil {
					devData := map[string]interface{}{
						"device_id":   id,
						"name":        name,
						"top_talkers": state["top_talkers"],
						"iface_stats": state["iface_stats"],
					}
					result = append(result, devData)
				}
			}
		}
	}

	if result == nil {
		result = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": result,
	})
}
