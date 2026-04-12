package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

type AIInsightResponse struct {
	ID              string   `json:"id"`
	CorrelationID   string   `json:"correlation_id"`
	Diagnosis       string   `json:"diagnosis"`
	Severity        string   `json:"severity"`
	InvolvedDevices []string `json:"involved_devices"`
	CreatedAt       string   `json:"created_at"`
}

func GetSentinelInsightsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT id, correlation_id, diagnosis, severity, involved_devices, created_at
		FROM ai_insights
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := database.DB.Query(query)
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
			&insight.CreatedAt,
		)
		if err == nil {
			// Extract JSON array
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
