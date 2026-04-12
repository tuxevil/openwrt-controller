package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"openwrt-controller/internal/database"
)

func GetSiteHistoryHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id required"}`, http.StatusBadRequest)
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric != "signal" && metric != "traffic" && metric != "cpu" {
		http.Error(w, `{"error": "invalid metric. must be signal, traffic, or cpu"}`, http.StatusBadRequest)
		return
	}

	// Fetch all devices for the site
	rows, err := database.DB.Query("SELECT id FROM devices WHERE site_id = $1", siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var deviceIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			deviceIDs = append(deviceIDs, id)
		}
	}

	if len(deviceIDs) == 0 {
		log.Printf("GetSiteHistoryHandler: no devices found for site %s", siteID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []database.TimeValuePair{}})
		return
	}

	data, err := database.GetSiteHistory(deviceIDs, metric)
	if err != nil {
		log.Printf("GetSiteHistoryHandler: influx error: %v", err)
		http.Error(w, `{"error": "influxdb query error"}`, http.StatusInternalServerError)
		return
	}

	log.Printf("GetSiteHistoryHandler: site %s metric %s returning %d points", siteID, metric, len(data))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}
