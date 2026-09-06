package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

type aiCompletion struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func aiEnginePassphrase() string { return os.Getenv("AI_ENGINE_ENCRYPTION_KEY") }

func SealAIKey(key string) (string, error) {
	passphrase := aiEnginePassphrase()
	if passphrase == "" {
		return "", fmt.Errorf("AI_ENGINE_ENCRYPTION_KEY is not set")
	}
	env, err := SealWithPassphrase(key, passphrase)
	if err != nil {
		return "", err
	}
	return EncodeEnvelope(env), nil
}

func openAIKey(raw string) string {
	if raw == "" {
		return ""
	}
	if passphrase := aiEnginePassphrase(); passphrase != "" {
		if env, err := DecodeEnvelope(raw); err == nil {
			if key, err := OpenWithPassphrase(env, passphrase); err == nil {
				return key
			}
		}
	}
	return raw
}

func ParseAICompletion(content string) (string, error) {
	var intent ChatOpsIntent
	content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```json"), "```"))
	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		return "", err
	}
	return content, nil
}

func completeAI(systemPrompt, userPrompt string, jsonMode bool) (string, string, int, error) {
	return completeAIContext(context.Background(), systemPrompt, userPrompt, jsonMode)
}

func completeAIContext(ctx context.Context, systemPrompt, userPrompt string, jsonMode bool) (string, string, int, error) {
	settings := database.GetPlatformSettings()
	baseURL := strings.TrimRight(settings.AIEngineBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if settings.AIEngineModel == "" {
		settings.AIEngineModel = "gpt-4o-mini"
	}
	if settings.AIEngineAPIKey == "" {
		return "", "", 0, fmt.Errorf("AI engine API key is not configured")
	}
	payload := map[string]interface{}{
		"model": settings.AIEngineModel,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	if jsonMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+openAIKey(settings.AIEngineAPIKey))
	resp, err := (&http.Client{Timeout: 300 * time.Second}).Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("AI engine request failed: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", 0, fmt.Errorf("AI engine returned status %d: %s", resp.StatusCode, string(responseBody))
	}
	var result aiCompletion
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", "", 0, fmt.Errorf("decode AI engine response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", result.Model, result.Usage.TotalTokens, fmt.Errorf("AI engine returned no choices")
	}
	return result.Choices[0].Message.Content, result.Model, result.Usage.TotalTokens, nil
}

func AnalyzeFleetContext(contextLogs string) (string, string, []string, string, int, error) {
	settings := database.GetPlatformSettings()
	prompt := settings.SentinelPrompt
	if prompt == "" {
		prompt = defaultSentinelPrompt
	}
	content, model, tokens, err := completeAI(prompt, "LOG STREAM:\n"+contextLogs, false)
	if err != nil {
		return "", "Low", []string{}, model, tokens, err
	}
	return parseSentinelResult(content, model, tokens)
}

func parseSentinelResult(content, model string, tokens int) (string, string, []string, string, int, error) {
	severity := "Low"
	devices := []string{}
	for _, line := range strings.Split(content, "\n") {
		upper := strings.ToUpper(strings.TrimSpace(line))
		if idx := strings.Index(upper, "SEVERITY:"); idx >= 0 {
			severity = strings.Trim(strings.TrimSpace(line[idx+len("SEVERITY:"):]), "[]*`_")
		}
		if idx := strings.Index(upper, "DEVICES:"); idx >= 0 {
			for _, device := range strings.Split(strings.Trim(strings.TrimSpace(line[idx+len("DEVICES:"):]), "[]*`"), ",") {
				if device = strings.TrimSpace(device); device != "" {
					devices = append(devices, device)
				}
			}
		}
	}
	return strings.TrimSpace(content), severity, devices, model, tokens, nil
}

const defaultSentinelPrompt = `You are a Fleet Security Analyst. Analyze this cross-device log stream. Look for coordinated attacks, lateral movements, or cascading hardware failures. If Device A shows a login failure and Device B shows a login success from the same IP, flag it as CRITICAL SUSPICION. Be technical, concise, and provide a Recommended Action.

End your report with these exact lines:
SEVERITY: [Critical, High, Medium, Low]
DEVICES: [Device_Name_1, Device_Name_2]`
