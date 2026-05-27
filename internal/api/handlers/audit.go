package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func GetAuditLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	offset := 0
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}

	logs, err := database.GetAuditLogs(limit, offset)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []database.AuditLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// TriggerVaultAuditHandler triggers a compliance audit for the latest backup.
// POST /api/devices/{device_id}/audit
func TriggerVaultAuditHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error":"device_id required"}`, http.StatusBadRequest)
		return
	}

	go services.RunVaultAudit(deviceID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "AUDIT_STARTED",
		"message": "Sentinel AI is analyzing the latest vault snapshot. Results available in ai_insights within ~30s.",
	})
}

// GetDeviceAuditResultsHandler returns past VAULT_AUDIT results for a device.
// GET /api/devices/{device_id}/audit
func GetDeviceAuditResultsHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if len(deviceID) < 8 {
		http.Error(w, `{"error":"invalid device_id"}`, http.StatusBadRequest)
		return
	}

	prefix := "VAULT-AUDIT-" + deviceID[:8] + "%"

	rows, err := database.Tx(r.Context()).Query(`
		SELECT correlation_id, diagnosis, severity,
		       COALESCE(llm_model, '') as llm_model,
		       COALESCE(tokens_used, 0) as tokens_used,
		       created_at
		FROM ai_insights
		WHERE correlation_id LIKE $1
		ORDER BY created_at DESC
		LIMIT 10
	`, prefix)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AuditEntry struct {
		CorrelationID string `json:"correlation_id"`
		Diagnosis     string `json:"diagnosis"`
		Severity      string `json:"severity"`
		LLMModel      string `json:"llm_model"`
		TokensUsed    int    `json:"tokens_used"`
		CreatedAt     string `json:"created_at"`
	}

	var results []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.CorrelationID, &e.Diagnosis, &e.Severity,
			&e.LLMModel, &e.TokensUsed, &e.CreatedAt); err != nil {
			continue
		}
		results = append(results, e)
	}
	if results == nil {
		results = []AuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": results})
}
