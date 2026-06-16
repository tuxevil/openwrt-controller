package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
	"openwrt-controller/internal/services"
)

// ─── Profile CRUD ────────────────────────────────────────────────────────────

func ListProfilesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := database.Tx(r.Context()).Query(`SELECT id, name, description, config_json, created_at, updated_at FROM profiles ORDER BY created_at DESC`)
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
	err := database.Tx(r.Context()).QueryRow(
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
	database.Tx(r.Context()).Exec(`UPDATE sites SET profile_id = NULL WHERE profile_id = $1`, profileID)
	database.Tx(r.Context()).Exec(`DELETE FROM profiles WHERE id = $1`, profileID)
	w.WriteHeader(http.StatusNoContent)
}

func AssignSiteProfileHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	var body struct {
		ProfileID string `json:"profile_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	_, err := database.Tx(r.Context()).Exec(`UPDATE sites SET profile_id = $1 WHERE id = $2`, body.ProfileID, siteID)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─── Mass Command Execution ───────────────────────────────────────────────────

// allowedMassActions enumerates the pre-validated operations that may be
// dispatched fleet-wide. Each action is a (name → shell template) pair that
// the controller fills in with safe per-device parameters (e.g. internal IP).
// The list is intentionally small: arbitrary shell execution is not exposed
// over HTTP anymore.
var allowedMassActions = map[string]func(args map[string]string) (string, error){
	"reboot": func(args map[string]string) (string, error) {
		return "sync && /etc/init.d/logrotate restart >/dev/null 2>&1; reboot", nil
	},
	"collect_diagnostics": func(args map[string]string) (string, error) {
		return "logread > /tmp/diag-$(cat /proc/sys/kernel/hostname).log && echo OK", nil
	},
	"restart_network": func(args map[string]string) (string, error) {
		return "/etc/init.d/network restart", nil
	},
	"sync_time": func(args map[string]string) (string, error) {
		return "/etc/init.d/sysntpd restart", nil
	},
}

func MassCommandHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SiteID string            `json:"site_id"`
		Action string            `json:"action"`
		Args   map[string]string `json:"args"`
		Reason string            `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SiteID == "" || body.Action == "" {
		http.Error(w, `{"error":"site_id and action required"}`, http.StatusBadRequest)
		return
	}
	if len(body.Reason) < 8 {
		http.Error(w, `{"error":"reason required (>= 8 chars) for audit trail"}`, http.StatusBadRequest)
		return
	}
	if body.Reason != strings.TrimSpace(body.Reason) {
		http.Error(w, `{"error":"reason must not contain leading/trailing whitespace"}`, http.StatusBadRequest)
		return
	}

	builder, ok := allowedMassActions[body.Action]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "unknown action",
			"allowed": keysOfAllowedMassActions(),
		})
		return
	}

	script, err := builder(body.Args)
	if err != nil {
		http.Error(w, `{"error":"invalid action args"}`, http.StatusBadRequest)
		return
	}

	start := time.Now()
	results := services.RunMassCommand(r.Context(), body.SiteID, script)
	elapsed := time.Since(start).Milliseconds()

	username := GetUsernameFromReq(r)
	if username != "system" {
		database.InsertAuditLog(username, "MASS_COMMAND_DISPATCHED", "SITE", body.SiteID,
			fmt.Sprintf("action=%s reason=%q script=%q", body.Action, body.Reason, script), r.RemoteAddr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"action":     body.Action,
		"elapsed_ms": elapsed,
		"results":    results,
	})
}

func keysOfAllowedMassActions() []string {
	out := make([]string, 0, len(allowedMassActions))
	for k := range allowedMassActions {
		out = append(out, k)
	}
	return out
}
