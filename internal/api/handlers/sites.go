package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

type createSiteRequest struct {
	Name         string   `json:"name"`
	ControllerID string   `json:"controller_id,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
}

func GetSitesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Tx(r.Context()).Query("SELECT id, controller_id, name, COALESCE(auto_adopt, false), latitude, longitude, created_at, updated_at FROM sites")
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
		var lat, lon *float64
		if err := rows.Scan(&id, &controllerID, &name, &autoAdopt, &lat, &lon, &created, &updated); err == nil {
			site := map[string]interface{}{
				"id":         id,
				"name":       name,
				"auto_adopt": autoAdopt,
				"latitude":   lat,
				"longitude":  lon,
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
		err = database.Tx(r.Context()).QueryRow(
			"INSERT INTO sites (name) VALUES ($1) RETURNING id",
			req.Name,
		).Scan(&newID)
	} else {
		err = database.Tx(r.Context()).QueryRow(
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

	_, err := database.Tx(r.Context()).Exec(
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

func DeleteSiteHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	schema := getTenantSchema(r)

	// Clean up related data manually since ON DELETE CASCADE is not ubiquitous.
	// Each Exec is checked so a failure aborts the cascade and surfaces a 500
	// to the operator (rather than silently orphaning rows in the tenant
	// schema).
	cleanups := []string{
		"DELETE FROM " + schema + ".guest_vouchers WHERE site_id = $1",
		"DELETE FROM " + schema + ".portal_settings WHERE site_id = $1",
		"DELETE FROM " + schema + ".client_hostnames WHERE site_id = $1",
		"DELETE FROM " + schema + ".incidents WHERE site_id = $1",
		"DELETE FROM " + schema + ".site_settings WHERE site_id = $1",
		"DELETE FROM " + schema + ".wlans WHERE site_id = $1",
	}
	for _, q := range cleanups {
		if _, err := database.Tx(r.Context()).Exec(q, siteID); err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to clean up site relations", err)
			return
		}
	}

	// Clean up or orphan agent versions
	if _, err := database.Tx(r.Context()).Exec("UPDATE "+schema+".agent_versions SET site_id = NULL WHERE site_id = $1", siteID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to orphan agent versions", err)
		return
	}

	// Orphan the devices
	if _, err := database.Tx(r.Context()).Exec("UPDATE "+schema+".devices SET site_id = NULL, status = 'Pending' WHERE site_id = $1", siteID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to orphan devices", err)
		return
	}

	res, err := database.Tx(r.Context()).Exec("DELETE FROM "+schema+".sites WHERE id = $1", siteID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete site", err)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "site not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
}

func UpdateSiteLocationHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	var req createSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}
	_, err := database.Tx(r.Context()).Exec(
		"UPDATE sites SET latitude = $1, longitude = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3",
		req.Latitude, req.Longitude, siteID,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
