package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"
)

type adoptRequest struct {
	SiteID string `json:"site_id"`
}

type migrateRequest struct {
	SiteID string `json:"site_id"`
}

func getTenantSchema(r *http.Request) (string, error) {
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		schema = "public"
	}
	return database.SafeSchemaIdent(schema)
}

func AdoptDeviceHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error": "invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
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
	err = database.Tx(r.Context()).QueryRow("SELECT EXISTS(SELECT 1 FROM "+schema+".devices WHERE id = $1)", deviceID).Scan(&exists)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	}

	_, err = database.Tx(r.Context()).Exec(
		"UPDATE "+schema+".devices SET site_id = $1, status = 'Adopted' WHERE id = $2",
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

func MigrateDeviceHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error": "invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	username := GetUsernameFromReq(r)
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	var req migrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.SiteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	var currentSiteID sql.NullString
	err = database.Tx(r.Context()).QueryRow("SELECT site_id FROM "+schema+".devices WHERE id = $1", deviceID).Scan(&currentSiteID)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	var siteExists bool
	err = database.Tx(r.Context()).QueryRow("SELECT EXISTS(SELECT 1 FROM "+schema+".sites WHERE id = $1)", req.SiteID).Scan(&siteExists)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	if !siteExists {
		http.Error(w, `{"error": "target site not found"}`, http.StatusNotFound)
		return
	}

	_, err = database.Tx(r.Context()).Exec(
		"UPDATE "+schema+".devices SET site_id = $1, status = 'Adopted' WHERE id = $2",
		req.SiteID, deviceID,
	)
	if err != nil {
		http.Error(w, `{"error": "failed to migrate device"}`, http.StatusInternalServerError)
		return
	}

	oldSiteStr := "None"
	if currentSiteID.Valid {
		oldSiteStr = currentSiteID.String
	}
	database.InsertAuditLog(username, "DEVICE_MIGRATED", "DEVICE", deviceID,
		fmt.Sprintf("Migrated device from site %s to site %s", oldSiteStr, req.SiteID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"id":          deviceID,
			"old_site_id": oldSiteStr,
			"new_site_id": req.SiteID,
			"status":      "Adopted",
		},
		"error": nil,
	})
}
