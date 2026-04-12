package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func GetClientsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	// Pull state_json from all devices in the site
	rows, err := database.DB.Query("SELECT id, state_json FROM devices WHERE site_id = $1 AND state_json IS NOT NULL", siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var clients []models.Client

	for rows.Next() {
		var devID string
		var state []byte
		if err := rows.Scan(&devID, &state); err == nil {
			var parsed map[string]interface{}
			if json.Unmarshal(state, &parsed) == nil {
				// Search for DHCP leases to determine clients
				if dhcpObj, ok := parsed["dhcp"].(map[string]interface{}); ok {
					if leases, ok := dhcpObj["leases"].([]interface{}); ok {
						for _, leaseRaw := range leases {
							lease, _ := leaseRaw.(map[string]interface{})
							mac, _ := lease["mac"].(string)
							ip, _ := lease["ip"].(string)
							hostname, _ := lease["hostname"].(string)

							clients = append(clients, models.Client{
								MAC:       mac,
								Hostname:  hostname,
								IPAddress: ip,
								DeviceID:  devID,
								Signal:    -65, // Mock signal since pure ubus dhcp block lacks wireless RSI
							})
						}
					}
				}
			}
		}
	}

	if clients == nil {
		clients = make([]models.Client, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": clients,
	})
}
