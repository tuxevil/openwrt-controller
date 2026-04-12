package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

func GetDeviceConfigHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	var siteID sql.NullString
	// 1. Busca el device_id, verifica su estado
	err := database.DB.QueryRow("SELECT site_id FROM devices WHERE id = $1", deviceID).Scan(&siteID)
	
	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// 2. Si no está adoptado (site_id es null)
	if !siteID.Valid {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"action":  "wait",
			"message": "Pending adoption",
		})
		return
	}

	// 3. Busca en la tabla wlans TODAS las redes asociadas a ese site_id
	rows, err := database.DB.Query("SELECT ssid, security, password FROM wlans WHERE site_id = $1 AND enabled = true", siteID.String)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var wlansList []map[string]string
	for rows.Next() {
		var ssid, security, password string
		if err := rows.Scan(&ssid, &security, &password); err == nil {
			wlan := map[string]string{
				"ssid":     ssid,
				"security": security,
			}
			if password != "" {
				wlan["key"] = password
			}
			wlansList = append(wlansList, wlan)
		}
	}

	if wlansList == nil {
		wlansList = make([]map[string]string, 0)
	}

	// 4. Devuelve payload JSON estructurado con WLANs
	json.NewEncoder(w).Encode(map[string]interface{}{
		"action": "apply",
		"config": map[string]interface{}{
			"wireless": map[string]interface{}{
				"wlans": wlansList,
			},
		},
	})
}
