package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

type TriggerSentinelRequest struct {
	Limit int `json:"limit"`
}

func TriggerManualSentinelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TriggerSentinelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Limit = 100 // fallback
	}
	if req.Limit <= 0 || req.Limit > 1000 {
		req.Limit = 100 // bounds check fallback
	}

	targetTime := time.Now()
	contextLogs := database.GetRecentContext(req.Limit)
	if contextLogs == "" {
		http.Error(w, "No logs available for analysis", http.StatusInternalServerError)
		return
	}

	diagnosis, severity, involvedDevices, llmModel, tokensUsed, err := services.AnalyzeFleetContext(contextLogs)
	if err != nil {
		log.Printf("[SENTINEL_AI_MANUAL] Inference engine error: %v", err)
		http.Error(w, "AI inference failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Save to ai_insights
	correlationID := fmt.Sprintf("AI-MANUAL-%d", targetTime.Unix())
	involvedJSON, _ := json.Marshal(involvedDevices)

	var insightID string
	err = database.Tx(r.Context()).QueryRow(`
		INSERT INTO ai_insights (correlation_id, diagnosis, severity, involved_devices, llm_model, tokens_used)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, correlationID, diagnosis, severity, string(involvedJSON), llmModel, tokensUsed).Scan(&insightID)

	if err != nil {
		log.Printf("[SENTINEL_AI_MANUAL] DB Insert error: %v", err)
		http.Error(w, "Database persistence failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"insight": AIInsightResponse{
			ID:              insightID,
			CorrelationID:   correlationID,
			Diagnosis:       diagnosis,
			Severity:        severity,
			InvolvedDevices: involvedDevices,
			LLMModel:        llmModel,
			TokensUsed:      tokensUsed,
			CreatedAt:       targetTime.Format(time.RFC3339),
		},
	})
}

type AIInsightResponse struct {
	ID              string   `json:"id"`
	CorrelationID   string   `json:"correlation_id"`
	Diagnosis       string   `json:"diagnosis"`
	Severity        string   `json:"severity"`
	InvolvedDevices []string `json:"involved_devices"`
	LLMModel        string   `json:"llm_model"`
	TokensUsed      int      `json:"tokens_used"`
	CreatedAt       string   `json:"created_at"`
}

func GetSentinelInsightsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT id, correlation_id, diagnosis, severity, involved_devices,
		       COALESCE(llm_model, '') AS llm_model,
		       COALESCE(tokens_used, 0) AS tokens_used,
		       created_at
		FROM ai_insights
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := database.Tx(r.Context()).Query(query)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var insights []AIInsightResponse
	for rows.Next() {
		var insight AIInsightResponse
		var devicesJSON string
		err := rows.Scan(
			&insight.ID,
			&insight.CorrelationID,
			&insight.Diagnosis,
			&insight.Severity,
			&devicesJSON,
			&insight.LLMModel,
			&insight.TokensUsed,
			&insight.CreatedAt,
		)
		if err == nil {
			var devs []string
			if err := json.Unmarshal([]byte(devicesJSON), &devs); err == nil {
				insight.InvolvedDevices = devs
			} else {
				insight.InvolvedDevices = []string{}
			}
			insights = append(insights, insight)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"insights": insights,
	})
}
