package services

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

// ChatOpsIntent represents the structured output expected from the LLM
type ChatOpsIntent struct {
	Intent       string `json:"intent"`
	TargetDevice string `json:"target_device,omitempty"`
	Summary      string `json:"summary"`
}

// ChatOpsResponse represents the output back to the API/Frontend
type ChatOpsResponse struct {
	Summary string      `json:"summary"`
	Data    interface{} `json:"data"`
}

const chatOpsSystemPrompt = `You are the Sentinel Oraculo RAG, an AI network operations assistant.
Your job is to classify the user's natural language command into one of the following exact INTENTS:
1. "GET_DEVICE_STATUS" - When asked about router status, online devices, ping, or connectivity.
2. "GET_TRAFFIC_STATS" - When asked about bandwidth, traffic, data usage, or limits.
3. "GET_RECENT_THREATS" - When asked about security, IPS, attacks, or security incidents.
4. "EXECUTE_AUDIT" - When asked to run a security audit, compliance check, or scan a device.
5. "UNKNOWN" - If the request doesn't match any of the above.

You MUST also attempt to extract a 'target_device' if a specific MAC address or hostname is mentioned.
Provide a brief conversational 'summary' acknowledging the action.
Respond ONLY with a valid JSON block, using this strict schema:
{
  "intent": "GET_DEVICE_STATUS",
  "target_device": "A1:B2:C3",
  "summary": "Fetching the status of all requested devices..."
}`

// ProcessChatOpsQuery takes natural language from the user, determines the intent via the LLM, and explicitly maps it to safe data functions execution.
func ProcessChatOpsQuery(query string) (*ChatOpsResponse, error) {
	settings := database.GetPlatformSettings()
	ollamaHost := settings.OllamaHost
	if ollamaHost == "" {
		ollamaHost = "127.0.0.1:11434"
	}
	model := settings.OllamaModel
	if model == "" {
		model = "llama3"
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": chatOpsSystemPrompt},
			{"role": "user", "content": "OPERATOR QUERY:\n" + query},
		},
		"stream": false,
		"format": "json", // Enforce JSON output for ChatOps intent parsing
	}

	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("http://%s/api/chat", ollamaHost)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ollama response: %w", err)
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ollama base response: %w", err)
	}

	content := strings.TrimSpace(result.Message.Content)
	var intent ChatOpsIntent
	
	// Fallback cleanly if LLM hallucinated markdown code blocks
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		log.Printf("[CHATOPS] Failed to parse intent JSON: %v, Raw: %s", err, content)
		return &ChatOpsResponse{
			Summary: "Error processing the request as the cognitive engine returned invalid intent format.",
			Data:    nil,
		}, nil
	}

	log.Printf("[CHATOPS] Parsed Intent: %s, Target: %s", intent.Intent, intent.TargetDevice)

	response := &ChatOpsResponse{
		Summary: intent.Summary,
	}

	// Safely map intent to existing database/service routines 
	switch intent.Intent {
	case "GET_DEVICE_STATUS":
		q := "SELECT id, name, model, status, last_seen_at FROM devices"
		args := []interface{}{}
		
		target := strings.ToLower(intent.TargetDevice)
		if target != "" && target != "all" && target != "todos" && target != "none" {
			q += " WHERE id = $1 OR name ILIKE $2"
			args = append(args, intent.TargetDevice, "%"+intent.TargetDevice+"%")
		} else {
			q += " ORDER BY last_seen_at DESC LIMIT 20"
		}
		
		rows, err := database.DB.Query(q, args...)
		if err != nil {
			return nil, fmt.Errorf("database err: %w", err)
		}
		defer rows.Close()

		var results []map[string]interface{}
		for rows.Next() {
			var id string
			var name, model, status, lastSeen sql.NullString
			if err := rows.Scan(&id, &name, &model, &status, &lastSeen); err == nil {
				results = append(results, map[string]interface{}{
					"id":           id,
					"name":         name.String,
					"model":        model.String,
					"status":       status.String,
					"last_seen_at": lastSeen.String,
				})
			}
		}
		response.Data = results

	case "GET_TRAFFIC_STATS":
		// Query shaping limits from PostgreSQL
		q := "SELECT device_id, mac, rate_mbytes, created_at FROM shaping_rules ORDER BY created_at DESC LIMIT 20"
		rows, err := database.DB.Query(q)
		if err != nil {
			return nil, fmt.Errorf("database err: %w", err)
		}
		defer rows.Close()

		var results []map[string]interface{}
		for rows.Next() {
			var deviceID, mac string
			var rateMBytes int
			var createdAt string
			if err := rows.Scan(&deviceID, &mac, &rateMBytes, &createdAt); err == nil {
				results = append(results, map[string]interface{}{
					"device_id":   deviceID,
					"mac":         mac,
					"rate_mbytes": rateMBytes,
					"created_at":  createdAt,
				})
			}
		}
		response.Data = results

	case "GET_RECENT_THREATS":
		q := "SELECT id, device_id, incident_type, severity, status, created_at FROM incidents ORDER BY created_at DESC LIMIT 10"
		rows, err := database.DB.Query(q)
		if err != nil {
			return nil, fmt.Errorf("database err: %w", err)
		}
		defer rows.Close()

		var results []map[string]interface{}
		for rows.Next() {
			var id, incidentType, severity, status, created string
			var deviceID sql.NullString
			if err := rows.Scan(&id, &deviceID, &incidentType, &severity, &status, &created); err == nil {
				results = append(results, map[string]interface{}{
					"id":            id,
					"device_id":     deviceID.String,
					"incident_type": incidentType,
					"severity":      severity,
					"status":        status,
					"created_at":    created,
				})
			}
		}
		response.Data = results

	case "EXECUTE_AUDIT":
		if intent.TargetDevice == "" {
			response.Summary = "Please specify a Target Device MAC address to run the Vault Audit."
			response.Data = nil
		} else {
			// Trigger Vault Audit securely
			result, err := RunVaultAudit(intent.TargetDevice)
			if err != nil {
				response.Summary = fmt.Sprintf("Failed to run audit on %s: %v", intent.TargetDevice, err)
			} else {
				response.Data = result
			}
		}

	case "UNKNOWN":
		response.Data = nil
	default:
		response.Data = nil
		response.Summary = "Unrecognized intent mapped by the cognitive engine."
	}

	return response, nil
}
