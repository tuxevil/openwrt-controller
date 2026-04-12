package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func GetIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	rows, err := database.DB.Query(`
		SELECT id, site_id, device_id, incident_type, severity, status, created_at, resolved_at 
		FROM incidents 
		WHERE site_id = $1
		ORDER BY created_at DESC 
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
		if err := rows.Scan(&inc.ID, &inc.SiteID, &inc.DeviceID, &inc.IncidentType, &inc.Severity, &inc.Status, &inc.CreatedAt, &inc.ResolvedAt); err == nil {
			incidents = append(incidents, inc)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": incidents})
}
