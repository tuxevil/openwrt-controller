package database

import (
	"strings"
)

type AuditLog struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Payload      string `json:"payload"`
	IPAddr       string `json:"ip_addr"`
	CreatedAt    string `json:"created_at"`
}

func InsertAuditLog(username, action, resourceType, resourceID, payload, ipAddr string) error {
	// Clean payload of special control characters if necessary, but TEXT handles it.
	// We'll trust the caller passes a sanitized string or raw dump.
	query := `
		INSERT INTO audit_logs (username, action, resource_type, resource_id, payload, ip_addr)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := DB.Exec(query, username, action, resourceType, resourceID, payload, ipAddr)
	return err
}

func GetAuditLogs(limit, offset int) ([]AuditLog, error) {
	query := `
		SELECT id, username, action, COALESCE(resource_type, ''), COALESCE(resource_id, ''), COALESCE(payload, ''), COALESCE(ip_addr, ''), created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := DB.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.Username, &l.Action, &l.ResourceType, &l.ResourceID, &l.Payload, &l.IPAddr, &l.CreatedAt); err != nil {
			return nil, err
		}
		
		// Remove null characters that might break JSON serialization
		l.Payload = strings.ReplaceAll(l.Payload, "\x00", "")

		logs = append(logs, l)
	}
	return logs, nil
}
