package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func GetLatestAgentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Agente sends the request, device authentication handled in routes via site-key (if needed)

	var version AgentVersion
	err := database.DB.QueryRow(`
		SELECT id, version_hash, script_content, is_active, created_at 
		FROM agent_versions 
		WHERE is_active = true 
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&version.ID, &version.VersionHash, &version.ScriptContent, &version.IsActive, &version.CreatedAt)

	if err != nil {
		// If no active version exists, just return 404 so agent doesn't do anything
		http.Error(w, "No active agent version found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(version)
}

func GetLatestAgentRawHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var scriptContent string
	err := database.DB.QueryRow(`
		SELECT script_content 
		FROM agent_versions 
		WHERE is_active = true 
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&scriptContent)

	if err != nil {
		http.Error(w, "No active agent version found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(scriptContent))
}

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
		ScriptContent string `json:"script_content"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ScriptContent == "" {
		http.Error(w, "Script content cannot be empty", http.StatusBadRequest)
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

	// Deactivate all others
	_, err = tx.Exec("UPDATE agent_versions SET is_active = false")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Check if this hash already exists to reactivate, else insert
	var existingID string
	err = tx.QueryRow("SELECT id FROM agent_versions WHERE version_hash = $1", hashStr).Scan(&existingID)
	
	if err == nil {
		// Exists, so set it as active
		_, err = tx.Exec("UPDATE agent_versions SET is_active = true WHERE id = $1", existingID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else {
		// Insert new
		_, err = tx.Exec(`
			INSERT INTO agent_versions (version_hash, script_content, is_active) 
			VALUES ($1, $2, true)
		`, hashStr, req.ScriptContent)
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

func GetAgentVersionsStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return what each device has vs what is the latest
	rows, err := database.DB.Query(`
		SELECT d.id, d.name, s.name, d.agent_version, d.last_seen_at 
		FROM devices d
		LEFT JOIN sites s ON d.site_id = s.id
	`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type DeviceAgentStatus struct {
		DeviceID     string  `json:"device_id"`
		DeviceName   *string `json:"device_name"`
		SiteName     *string `json:"site_name"`
		AgentVersion *string `json:"agent_version"`
		LastSeenAt   *time.Time `json:"last_seen_at"`
	}

	statusList := []DeviceAgentStatus{}
	for rows.Next() {
		var st DeviceAgentStatus
		if err := rows.Scan(&st.DeviceID, &st.DeviceName, &st.SiteName, &st.AgentVersion, &st.LastSeenAt); err != nil {
			continue
		}
		statusList = append(statusList, st)
	}

	var latestHash *string
	// Safely get latest hash just in case
	database.DB.QueryRow("SELECT version_hash FROM agent_versions WHERE is_active = true LIMIT 1").Scan(&latestHash)

	response := map[string]interface{}{
		"latest_hash": latestHash,
		"devices":     statusList,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
