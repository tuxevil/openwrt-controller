package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

// AnalyzeFleetContext sends the gathered logs to Ollama for analysis
func AnalyzeFleetContext(contextLogs string) (diagnosis string, severity string, involvedDevices []string, llmModel string, tokensUsed int, err error) {
	settings := database.GetPlatformSettings()

	ollamaHost := settings.OllamaHost
	if ollamaHost == "" {
		ollamaHost = "127.0.0.1:11434"
	}
	model := settings.OllamaModel
	if model == "" {
		model = "llama3" // or mistral
	}

	systemPrompt := settings.SentinelPrompt
	if systemPrompt == "" {
		systemPrompt = `You are a Fleet Security Analyst. Analyze this cross-device log stream. Look for coordinated attacks, lateral movements, or cascading hardware failures. If Device A shows a login failure and Device B shows a login success from the same IP, flag it as CRITICAL SUSPICION. Be technical, concise, and provide a 'Recommended Action'. The output must look like a high-level SOC report. No fluff.

End your report with these two exact lines at the bottom for parsing:
SEVERITY: [Critical, High, Medium, Low]
DEVICES: [Device_Name_1, Device_Name_2]
`
		log.Printf("[SENTINEL_AI] WARNING: No sentinel_prompt in DB, using hardcoded default.")
	} else {
		log.Printf("[SENTINEL_AI] Using DB sentinel_prompt: %.80q...", systemPrompt)
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": systemPrompt,
			},
			{
				"role":    "user",
				"content": "LOG STREAM:\n" + contextLogs,
			},
		},
		"stream": false,
	}

	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("http://%s/api/chat", ollamaHost)

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return "", "Low", []string{}, "", 0, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "Low", []string{}, "", 0, fmt.Errorf("failed to read ollama response: %w", err)
	}

	var result struct {
		Model   string `json:"model"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "Low", []string{}, "", 0, fmt.Errorf("failed to unmarshal ollama json: %w", err)
	}

	content := strings.TrimSpace(result.Message.Content)
	diagnosis = content
	severity = "Low"
	involvedDevices = []string{}
	llmModel = result.Model
	if result.Model == "" {
		llmModel = model
	}
	tokensUsed = result.PromptEvalCount + result.EvalCount

	// Parse out SEVERITY and DEVICES
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "SEVERITY:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				severity = strings.TrimSpace(strings.ReplaceAll(parts[1], "]", ""))
				severity = strings.ReplaceAll(severity, "[", "")
			}
		} else if strings.HasPrefix(strings.ToUpper(line), "DEVICES:") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				devStr := strings.TrimSpace(strings.ReplaceAll(parts[1], "]", ""))
				devStr = strings.ReplaceAll(devStr, "[", "")
				devs := strings.Split(devStr, ",")
				for _, d := range devs {
					d = strings.TrimSpace(d)
					if d != "" {
						involvedDevices = append(involvedDevices, d)
					}
				}
			}
		}
	}

	if severity == "" {
		severity = "Low"
	}

	return diagnosis, severity, involvedDevices, llmModel, tokensUsed, nil
}
