package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func DeleteWLANHandler(w http.ResponseWriter, r *http.Request) {
	wlanID := r.PathValue("wlan_id")
	if wlanID == "" {
		http.Error(w, `{"error": "wlan_id is required"}`, http.StatusBadRequest)
		return
	}

	res, err := database.DB.Exec("DELETE FROM wlans WHERE id = $1", wlanID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "wlan not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
}

type createWLANRequest struct {
	SSID           string `json:"ssid"`
	Security       string `json:"security"`
	Password       string `json:"password"`
	Enabled        *bool  `json:"enabled"`
	RoamingEnabled *bool  `json:"roaming_enabled"`
}

func CreateWLANHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	var req createWLANRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.SSID == "" || req.Security == "" {
		http.Error(w, `{"error": "ssid and security are required"}`, http.StatusBadRequest)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	roamingEnabled := false
	if req.RoamingEnabled != nil {
		roamingEnabled = *req.RoamingEnabled
	}

	var newID string
	err := database.DB.QueryRow(
		"INSERT INTO wlans (site_id, ssid, security, password, enabled, roaming_enabled) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id",
		siteID, req.SSID, req.Security, req.Password, enabled, roamingEnabled,
	).Scan(&newID)

	if err != nil {
		http.Error(w, `{"error": "failed to create wlan, ensure site_id exists"}`, http.StatusInternalServerError)
		return
	}

	go services.AddWLANConfig(siteID, req.SSID, req.Security, req.Password, roamingEnabled)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  map[string]interface{}{"id": newID},
		"error": nil,
	})
}

func GetWLANsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	query := `SELECT id, site_id, ssid, security, enabled, COALESCE(roaming_enabled, false) FROM wlans WHERE site_id = $1`
	rows, err := database.DB.Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var wlans []map[string]interface{}
	for rows.Next() {
		var id, sID, ssid, security string
		var enabled, roaming bool
		if err := rows.Scan(&id, &sID, &ssid, &security, &enabled, &roaming); err == nil {
			wlans = append(wlans, map[string]interface{}{
				"id":              id,
				"site_id":         sID,
				"ssid":            ssid,
				"security":        security,
				"enabled":         enabled,
				"roaming_enabled": roaming,
			})
		}
	}

	if wlans == nil {
		wlans = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  wlans,
		"error": nil,
	})
}
