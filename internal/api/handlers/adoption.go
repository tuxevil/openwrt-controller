package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

type adoptRequest struct {
	SiteID string `json:"site_id"`
}

func AdoptDeviceHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	var req adoptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.SiteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	var exists bool
	err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM devices WHERE id = $1)", deviceID).Scan(&exists)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	}

	_, err = database.DB.Exec(
		"UPDATE devices SET site_id = $1, status = 'Adopted' WHERE id = $2",
		req.SiteID, deviceID,
	)
	if err != nil {
		http.Error(w, `{"error": "failed to adopt device"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"id":      deviceID,
			"site_id": req.SiteID,
			"status":  "Adopted",
		},
		"error": nil,
	})
}
