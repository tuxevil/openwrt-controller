package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
	"openwrt-controller/internal/services"
)

// ─── Profile CRUD ────────────────────────────────────────────────────────────

func ListProfilesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query(`SELECT id, name, description, config_json, created_at, updated_at FROM profiles ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var profiles []models.Profile
	for rows.Next() {
		var p models.Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.ConfigJSON, &p.CreatedAt, &p.UpdatedAt); err == nil {
			profiles = append(profiles, p)
		}
	}
	if profiles == nil {
		profiles = []models.Profile{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": profiles})
}

func CreateProfileHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		ConfigJSON  json.RawMessage `json:"config_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	cfg := body.ConfigJSON
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	var id string
	err := database.DB.QueryRow(
		`INSERT INTO profiles (name, description, config_json) VALUES ($1, $2, $3) RETURNING id`,
		body.Name, body.Description, cfg,
	).Scan(&id)
	if err != nil {
		http.Error(w, `{"error":"insert failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func DeleteProfileHandler(w http.ResponseWriter, r *http.Request) {
	profileID := r.PathValue("profile_id")
	database.DB.Exec(`UPDATE sites SET profile_id = NULL WHERE profile_id = $1`, profileID)
	database.DB.Exec(`DELETE FROM profiles WHERE id = $1`, profileID)
	w.WriteHeader(http.StatusNoContent)
}

func AssignSiteProfileHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	var body struct {
		ProfileID string `json:"profile_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	_, err := database.DB.Exec(`UPDATE sites SET profile_id = $1 WHERE id = $2`, body.ProfileID, siteID)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─── Mass Command Execution ───────────────────────────────────────────────────

func MassCommandHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SiteID  string `json:"site_id"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SiteID == "" || body.Command == "" {
		http.Error(w, `{"error":"site_id and command required"}`, http.StatusBadRequest)
		return
	}

	// Set timeout header
	start := time.Now()
	results := services.RunMassCommand(body.SiteID, body.Command)
	elapsed := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"elapsed_ms": elapsed,
		"results":    results,
	})
}
