package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

func GetDevicesHandler(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	
	query := `SELECT id, site_id, name, model, status, last_seen_at FROM devices`
	args := []interface{}{}
	
	if statusFilter == "pending" {
		query += ` WHERE site_id IS NULL`
	}

	rows, err := database.DB.Query(query, args...)
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

	query := `SELECT id, site_id, name, model, status, last_seen_at, last_config_pulled_at FROM devices WHERE site_id = $1`
	rows, err := database.DB.Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id string
		var sID, name, model, status, lastSeen, lastPulled sql.NullString
		if err := rows.Scan(&id, &sID, &name, &model, &status, &lastSeen, &lastPulled); err == nil {
			dev := map[string]interface{}{
				"id":                    id,
				"site_id":               siteID,
				"name":                  name.String,
				"model":                 model.String,
				"status":                status.String,
				"last_seen_at":          lastSeen.String,
				"last_config_pulled_at": lastPulled.String,
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
