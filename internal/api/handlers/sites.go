package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

type createSiteRequest struct {
	Name         string `json:"name"`
	ControllerID string `json:"controller_id,omitempty"`
}

func GetSitesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query("SELECT id, controller_id, name, COALESCE(auto_adopt, false), created_at, updated_at FROM sites")
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sites []map[string]interface{}
	for rows.Next() {
		var id, name string
		var controllerID *string
		var autoAdopt bool
		var created, updated string
		if err := rows.Scan(&id, &controllerID, &name, &autoAdopt, &created, &updated); err == nil {
			site := map[string]interface{}{
				"id":         id,
				"name":       name,
				"auto_adopt": autoAdopt,
				"created_at": created,
				"updated_at": updated,
			}
			if controllerID != nil {
				site["controller_id"] = *controllerID
			}
			sites = append(sites, site)
		}
	}

	if sites == nil {
		sites = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  sites,
		"error": nil,
	})
}

func CreateSiteHandler(w http.ResponseWriter, r *http.Request) {
	var req createSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error": "name is required"}`, http.StatusBadRequest)
		return
	}

	var newID string
	var err error

	if req.ControllerID == "" {
		err = database.DB.QueryRow(
			"INSERT INTO sites (name) VALUES ($1) RETURNING id",
			req.Name,
		).Scan(&newID)
	} else {
		err = database.DB.QueryRow(
			"INSERT INTO sites (controller_id, name) VALUES ($1, $2) RETURNING id",
			req.ControllerID, req.Name,
		).Scan(&newID)
	}

	if err != nil {
		http.Error(w, `{"error": "failed to create site"}`, http.StatusInternalServerError)
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

// ToggleAutoAdoptHandler enables or disables Zero-Touch Provisioning for a site.
func ToggleAutoAdoptHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"site_id required"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	_, err := database.DB.Exec(
		"UPDATE sites SET auto_adopt = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		body.Enabled, siteID,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"auto_adopt": body.Enabled,
		"site_id":    siteID,
	})
}
