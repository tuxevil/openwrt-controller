package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func validateAIEngineBaseURL(raw string) (string, error) {
	if raw == "" {
		raw = "https://api.openai.com/v1"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("AI engine base URL is invalid")
	}
	ip := net.ParseIP(parsed.Hostname())
	privateHTTP := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || (ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())))
	if parsed.Scheme != "https" && !privateHTTP {
		return "", fmt.Errorf("AI engine base URL must use HTTPS, except private providers")
	}
	return strings.TrimRight(raw, "/"), nil
}

func GetPlatformSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := database.GetPlatformSettings()
	settings.AIEngineAPIKey = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"data":   settings,
	})
}

func UpdatePlatformSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var s database.PlatformSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}

	current := database.GetPlatformSettings()
	var err error
	apiKey := s.AIEngineAPIKey
	keyChanged := apiKey != "" && apiKey != "********"
	if !keyChanged {
		apiKey = current.AIEngineAPIKey
	}
	s.AIEngineBaseURL, err = validateAIEngineBaseURL(s.AIEngineBaseURL)
	if err != nil {
		http.Error(w, `{"error":"invalid AI engine base URL"}`, http.StatusBadRequest)
		return
	}
	if keyChanged {
		sealed, err := services.SealAIKey(apiKey)
		if err != nil {
			http.Error(w, `{"error":"AI key encryption is not configured"}`, http.StatusInternalServerError)
			return
		}
		apiKey = sealed
	}

	query := `
		UPDATE platform_settings
		SET ai_engine_base_url = $1, ai_engine_model = $2, ai_engine_api_key = $3, sentinel_prompt = $4, telegram_bot_token = $5, telegram_chat_id = $6,
		    global_surveys_public_lockdown = $7, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`
	_, err = database.Tx(r.Context()).Exec(query, s.AIEngineBaseURL, s.AIEngineModel, apiKey, s.SentinelPrompt, s.TelegramBotToken, s.TelegramChatID, s.GlobalSurveysPublicLockdown)
	if err != nil {
		http.Error(w, `{"error": "db update error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}
