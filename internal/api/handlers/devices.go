package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

func GetDevicesHandler(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	
	query := `SELECT id, site_id, name, model, status, last_seen_at FROM devices LIMIT 1000`
	args := []interface{}{}
	
	if statusFilter == "pending" {
		query = `SELECT id, site_id, name, model, status, last_seen_at FROM devices WHERE site_id IS NULL LIMIT 1000`
	}

	rows, err := database.Tx(r.Context()).Query(query, args...)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id string
		var siteID, name, model, status, lastSeen sql.NullString
		if err := rows.Scan(&id, &siteID, &name, &model, &status, &lastSeen); err == nil {
			dev := map[string]interface{}{
				"id":           id,
				"name":         name.String,
				"model":        model.String,
				"status":       status.String,
				"last_seen_at": lastSeen.String,
			}
			if siteID.Valid {
				dev["site_id"] = siteID.String
			}
			devices = append(devices, dev)
		}
	}

	if devices == nil {
		devices = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  devices,
		"error": nil,
	})
}

func GetSiteDevicesHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	query := `SELECT id, site_id, name, model, status, last_seen_at, last_config_pulled_at, last_ip, agent_version, state_json FROM devices WHERE site_id = $1`
	rows, err := database.Tx(r.Context()).Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id string
		var sID, name, model, status, lastSeen, lastPulled, lastIP, agentVersion sql.NullString
		var stateJSON []byte
		if err := rows.Scan(&id, &sID, &name, &model, &status, &lastSeen, &lastPulled, &lastIP, &agentVersion, &stateJSON); err == nil {
			dev := map[string]interface{}{
				"id":                    id,
				"site_id":               siteID,
				"name":                  name.String,
				"model":                 model.String,
				"status":                status.String,
				"last_seen_at":          lastSeen.String,
				"last_config_pulled_at": lastPulled.String,
				"last_ip":               lastIP.String,
				"agent_version":         agentVersion.String,
			}
			if len(stateJSON) > 0 {
				var parsedState map[string]interface{}
				if json.Unmarshal(stateJSON, &parsedState) == nil {
					dev["state_json"] = parsedState
				}
			}
			devices = append(devices, dev)
		}
	}

	if devices == nil {
		devices = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  devices,
		"error": nil,
	})
}

func ForgetDeviceHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	// Clean up child tables to prevent foreign key constraint violations
	database.Tx(r.Context()).Exec("DELETE FROM backups WHERE device_id = $1", deviceID)
	database.Tx(r.Context()).Exec("DELETE FROM incidents WHERE device_id = $1", deviceID)

	res, err := database.Tx(r.Context()).Exec("DELETE FROM devices WHERE id = $1", deviceID)
	if err != nil {
		http.Error(w, `{"error": "database error: " + err.Error()}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
}
