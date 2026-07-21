package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
)

type AgentVersion struct {
	ID            string    `json:"id"`
	VersionHash   string    `json:"version_hash"`
	ScriptContent string    `json:"script_content"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
}

// resolveSiteByKey looks up a site by its api_key header value.
// Returns the site UUID or an empty string if not found.
func resolveSiteByKey(siteKey string) (string, error) {
	schema, err := database.GetTenantSchemaForSiteKey(siteKey)
	if err != nil {
		return "", err
	}
	var siteID string
	err = database.DB.QueryRow(
		"SELECT id FROM "+schema+".sites WHERE api_key = $1", siteKey,
	).Scan(&siteID)
	return siteID, err
}

// GetLatestAgentHandler returns the latest active agent version metadata for
// the site identified by the X-Site-Key header. Device-facing, no JWT needed.
func GetLatestAgentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	siteKey := r.Header.Get("X-Site-Key")
	if siteKey == "" {
		http.Error(w, "Forbidden: missing X-Site-Key", http.StatusForbidden)
		return
	}

	siteID, err := resolveSiteByKey(siteKey)
	if err != nil || siteID == "" {
		http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
		return
	}

	tenantSchema, err := database.GetTenantSchemaForSiteKey(siteKey)
	if err != nil {
		http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
		return
	}

	var version AgentVersion
	err = database.DB.QueryRow(fmt.Sprintf(`
		SELECT id, version_hash, script_content, is_active, created_at 
		FROM %s.agent_versions 
		WHERE is_active = true AND site_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, tenantSchema), siteID).Scan(&version.ID, &version.VersionHash, &version.ScriptContent, &version.IsActive, &version.CreatedAt)

	if err != nil {
		// No active version for this site — agent should do nothing
		http.Error(w, "No active agent version found for this site", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(version)
}

// GetLatestAgentRawHandler returns the raw script content for the site
// identified by the X-Site-Key header. Device-facing, no JWT needed.
func GetLatestAgentRawHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	siteKey := r.Header.Get("X-Site-Key")
	if siteKey == "" {
		http.Error(w, "Forbidden: missing X-Site-Key", http.StatusForbidden)
		return
	}

	siteID, err := resolveSiteByKey(siteKey)
	if err != nil || siteID == "" {
		http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
		return
	}

	tenantSchema, err := database.GetTenantSchemaForSiteKey(siteKey)
	if err != nil {
		http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
		return
	}

	var scriptContent string
	err = database.DB.QueryRow(fmt.Sprintf(`
		SELECT script_content 
		FROM %s.agent_versions 
		WHERE is_active = true AND site_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, tenantSchema), siteID).Scan(&scriptContent)

	if err != nil {
		http.Error(w, "No active agent version found for this site", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(scriptContent))
}

// DeployAgentHandler stores a new agent script and marks it as the active
// version for the given site_id. Requires JWT (admin dashboard use only).
func DeployAgentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var req struct {
		SiteID        string `json:"site_id"`
		ScriptContent string `json:"script_content"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.SiteID == "" {
		http.Error(w, "site_id is required", http.StatusBadRequest)
		return
	}
	if req.ScriptContent == "" {
		http.Error(w, "Script content cannot be empty", http.StatusBadRequest)
		return
	}

	schema, schemaErr := getTenantSchema(r)
	if schemaErr != nil {
		http.Error(w, "invalid tenant context", http.StatusInternalServerError)
		return
	}

	// Validate that the site exists
	var exists bool
	if err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM "+schema+".sites WHERE id = $1)", req.SiteID).Scan(&exists); err != nil || !exists {
		http.Error(w, "site_id not found", http.StatusBadRequest)
		return
	}

	// Calculate SHA256 of the new script
	hash := sha256.Sum256([]byte(req.ScriptContent))
	hashStr := hex.EncodeToString(hash[:])

	tx, err := database.DB.Begin()
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Deactivate all other versions for THIS site only
	_, err = tx.Exec("UPDATE "+schema+".agent_versions SET is_active = false WHERE site_id = $1", req.SiteID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if this hash+site already exists to reactivate, else insert
	var existingID string
	err = tx.QueryRow(
		"SELECT id FROM "+schema+".agent_versions WHERE version_hash = $1 AND site_id = $2",
		hashStr, req.SiteID,
	).Scan(&existingID)

	if err == nil {
		// Exists for this site — reactivate it
		_, err = tx.Exec("UPDATE "+schema+".agent_versions SET is_active = true WHERE id = $1", existingID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else {
		// Insert new version scoped to this site
		_, err = tx.Exec(`
			INSERT INTO `+schema+`.agent_versions (version_hash, script_content, is_active, site_id) 
			VALUES ($1, $2, true, $3)
		`, hashStr, req.ScriptContent, req.SiteID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Commit error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"success", "version_hash":"` + hashStr + `"}`))
}

// GetAgentVersionsStatusHandler returns agent version status per device,
// grouped with each site's active hash so the frontend can compare correctly.
func GetAgentVersionsStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	schema, schemaErr := getTenantSchema(r)
	if schemaErr != nil {
		http.Error(w, "invalid tenant context", http.StatusInternalServerError)
		return
	}

	// Per-site active hashes
	siteHashRows, err := database.DB.Query(`
		SELECT site_id, version_hash 
		FROM ` + schema + `.agent_versions 
		WHERE is_active = true
	`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer siteHashRows.Close()

	siteHashes := map[string]string{}
	for siteHashRows.Next() {
		var siteID, hash string
		if err := siteHashRows.Scan(&siteID, &hash); err == nil {
			siteHashes[siteID] = hash
		}
	}

	// Device list with their current agent version
	rows, err := database.DB.Query(`
		SELECT d.id, d.name, s.id, s.name, d.agent_version, d.last_seen_at 
		FROM ` + schema + `.devices d
		LEFT JOIN ` + schema + `.sites s ON d.site_id = s.id
	`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type DeviceAgentStatus struct {
		DeviceID     string     `json:"device_id"`
		DeviceName   *string    `json:"device_name"`
		SiteID       *string    `json:"site_id"`
		SiteName     *string    `json:"site_name"`
		AgentVersion *string    `json:"agent_version"`
		LatestHash   *string    `json:"latest_hash"` // the active hash for this device's site
		LastSeenAt   *time.Time `json:"last_seen_at"`
	}

	statusList := []DeviceAgentStatus{}
	for rows.Next() {
		var st DeviceAgentStatus
		if err := rows.Scan(&st.DeviceID, &st.DeviceName, &st.SiteID, &st.SiteName, &st.AgentVersion, &st.LastSeenAt); err != nil {
			continue
		}
		// Attach the active hash for this device's site
		if st.SiteID != nil {
			if h, ok := siteHashes[*st.SiteID]; ok {
				st.LatestHash = &h
			}
		}
		statusList = append(statusList, st)
	}

	response := map[string]interface{}{
		"site_hashes": siteHashes,
		"devices":     statusList,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSiteAgentRawHandler is a JWT-authenticated endpoint for the admin
// dashboard to retrieve the active agent script for a given site.
// Query param: ?site_id=<uuid>
func GetSiteAgentRawHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	siteID := r.URL.Query().Get("site_id")
	if siteID == "" {
		http.Error(w, "site_id query param is required", http.StatusBadRequest)
		return
	}

	schema, schemaErr := getTenantSchema(r)
	if schemaErr != nil {
		http.Error(w, "invalid tenant context", http.StatusInternalServerError)
		return
	}

	var scriptContent string
	err := database.DB.QueryRow(`
		SELECT script_content 
		FROM `+schema+`.agent_versions 
		WHERE is_active = true AND site_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, siteID).Scan(&scriptContent)

	if err != nil {
		http.Error(w, "No active agent version found for this site", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(scriptContent))
}
