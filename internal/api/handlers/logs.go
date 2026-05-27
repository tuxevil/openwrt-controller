package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
)

func GetLogsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, "Missing site_id", http.StatusBadRequest)
		return
	}

	severityFilter := r.URL.Query().Get("severity")
	searchQuery := r.URL.Query().Get("search")

	// Base query joining devices and system_logs
	query := `
		SELECT l.log_timestamp, l.severity, l.message, d.id as device_id,
		       COALESCE(NULLIF(d.state_json->'board'->>'hostname',''), NULLIF(d.name,''), d.id) as device_name
		FROM system_logs l
		JOIN devices d ON d.id = l.device_id
		WHERE d.site_id = $1
	`
	args := []interface{}{siteID}
	argIdx := 2

	if severityFilter != "" && severityFilter != "ALL" {
		query += ` AND l.severity = $` + javaToPgIdx(argIdx)
		args = append(args, severityFilter)
		argIdx++
	}

	if searchQuery != "" {
		query += ` AND l.message ILIKE $` + javaToPgIdx(argIdx)
		args = append(args, "%"+searchQuery+"%")
		argIdx++
	}

	query += ` ORDER BY l.log_timestamp DESC LIMIT 1000`

	rows, err := database.Tx(r.Context()).Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []database.LogEntry
	for rows.Next() {
		var timestamp time.Time
		var entry database.LogEntry
		if err := rows.Scan(&timestamp, &entry.Level, &entry.Message, &entry.DeviceID, &entry.DeviceName); err != nil {
			continue
		}
		entry.Timestamp = timestamp.UTC().Format(time.RFC3339)
		logs = append(logs, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": logs})
}

// helper for pg indexed placeholder formats
func javaToPgIdx(i int) string {
	importStr := []string{"", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	if i < 10 {
		return importStr[i]
	}
	return "" // Just an inline hack for max 2 filters
}
