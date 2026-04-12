package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

type createWLANRequest struct {
	SSID     string `json:"ssid"`
	Security string `json:"security"`
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled"`
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

	var newID string
	err := database.DB.QueryRow(
		"INSERT INTO wlans (site_id, ssid, security, password, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		siteID, req.SSID, req.Security, req.Password, enabled,
	).Scan(&newID)

	if err != nil {
		http.Error(w, `{"error": "failed to create wlan, ensure site_id exists"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"id": newID,
		},
		"error": nil,
	})
}

func GetWLANsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	query := `SELECT id, site_id, ssid, security, password, enabled FROM wlans WHERE site_id = $1`
	rows, err := database.DB.Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var wlans []map[string]interface{}
	for rows.Next() {
		var id, sID, ssid, security, password string
		var enabled bool
		if err := rows.Scan(&id, &sID, &ssid, &security, &password, &enabled); err == nil {
			wlans = append(wlans, map[string]interface{}{
				"id":       id,
				"site_id":  sID,
				"ssid":     ssid,
				"security": security,
				"password": password,
				"enabled":  enabled,
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
