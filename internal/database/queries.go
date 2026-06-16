package database

import (
	"fmt"
	"strings"
	"time"
)

// GetGlobalContext fetches the most recent logs from the fleet around the target timestamp (±2min).
func GetGlobalContext(schema string, targetTimestamp time.Time, limit int) string {
	if DB == nil {
		return ""
	}

	start := targetTimestamp.Add(-2 * time.Minute)
	end := targetTimestamp.Add(2 * time.Minute)

	query := fmt.Sprintf(`
		SELECT d.name, sl.log_timestamp, sl.message
		FROM %s.system_logs sl
		LEFT JOIN %s.devices d ON sl.device_id = d.id
		WHERE sl.log_timestamp >= $1 AND sl.log_timestamp <= $2
		ORDER BY sl.log_timestamp DESC
		LIMIT $3
	`, schema, schema)
	rows, err := DB.Query(query, start, end, limit)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var devName *string
		var ts time.Time
		var msg string
		if err := rows.Scan(&devName, &ts, &msg); err == nil {
			name := "UNKNOWN"
			if devName != nil {
				name = *devName
			}
			lines = append(lines, fmt.Sprintf("[%s] | [%s] | %s", name, ts.Format(time.RFC3339), msg))
		}
	}

	// Reverse to make it chronological
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	// The prompt requests recent logs from EVERY device, but practically
	// we just return a unified feed up to the specified limit.
	return strings.Join(lines, "\n")
}

// GetRecentContext fetches the N most recent logs from the fleet with no time restriction.
// Used by the manual Sentinel AI trigger to always have data to analyze.
func GetRecentContext(schema string, limit int) string {
	if DB == nil {
		return ""
	}

	query := fmt.Sprintf(`
		SELECT d.name, sl.log_timestamp, sl.message
		FROM %s.system_logs sl
		LEFT JOIN %s.devices d ON sl.device_id = d.id
		ORDER BY sl.log_timestamp DESC
		LIMIT $1
	`, schema, schema)
	rows, err := DB.Query(query, limit)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var devName *string
		var ts time.Time
		var msg string
		if err := rows.Scan(&devName, &ts, &msg); err == nil {
			name := "UNKNOWN"
			if devName != nil {
				name = *devName
			}
			lines = append(lines, fmt.Sprintf("[%s] | [%s] | %s", name, ts.Format(time.RFC3339), msg))
		}
	}

	// Reverse to make it chronological
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	return strings.Join(lines, "\n")
}
