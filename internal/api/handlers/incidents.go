package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func GetIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	rows, err := database.Tx(r.Context()).Query(`
		SELECT i.id, i.site_id, i.device_id,
		       COALESCE(NULLIF(d.state_json->'board'->>'hostname', ''), i.device_id),
		       i.incident_type, i.severity, i.status, i.created_at, i.resolved_at 
		FROM incidents i
		LEFT JOIN devices d ON d.id = i.device_id
		WHERE i.site_id = $1
		ORDER BY i.created_at DESC 
		LIMIT 100
	`, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		var inc models.Incident
		if err := rows.Scan(&inc.ID, &inc.SiteID, &inc.DeviceID, &inc.DeviceName, &inc.IncidentType, &inc.Severity, &inc.Status, &inc.CreatedAt, &inc.ResolvedAt); err == nil {
			incidents = append(incidents, inc)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": incidents})
}
