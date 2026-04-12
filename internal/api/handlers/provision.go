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

	// --- Módulo 3: Hardening - Validar X-Device-Token ---
	token := r.Header.Get("X-Device-Token")
	if token != "" {
		// Solo valida si se envía un token; sin token = acceso sin auth (modo legado)
		var storedToken sql.NullString
		err := database.DB.QueryRow("SELECT device_token FROM devices WHERE id = $1", deviceID).Scan(&storedToken)
		if err == nil && storedToken.Valid && storedToken.String != "" && storedToken.String != token {
			http.Error(w, `{"error": "invalid device token"}`, http.StatusUnauthorized)
			return
		}
	}

	var siteID sql.NullString
	var siteKey *string
	err := database.DB.QueryRow(`
		SELECT d.site_id, s.api_key 
		FROM devices d 
		LEFT JOIN sites s ON d.site_id = s.id 
		WHERE d.id = $1`, deviceID).Scan(&siteID, &siteKey)

	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	providedKey := r.Header.Get("X-Site-Key")
	if siteKey != nil && *siteKey != "" {
		if providedKey != *siteKey {
			http.Error(w, `{"error": "Forbidden: invalid site key"}`, http.StatusForbidden)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if !siteID.Valid {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"action":  "wait",
			"message": "Pending adoption",
		})
		return
	}

	// --- Módulo 2: Actualizar last_config_pulled_at ---
	_, _ = database.DB.Exec(
		"UPDATE devices SET last_config_pulled_at = CURRENT_TIMESTAMP WHERE id = $1",
		deviceID,
	)

	rows, err := database.DB.Query(
		"SELECT ssid, security, password FROM wlans WHERE site_id = $1 AND enabled = true",
		siteID.String,
	)
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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"action": "apply",
		"config": map[string]interface{}{
			"wireless": map[string]interface{}{
				"wlans": wlansList,
			},
		},
	})
}
