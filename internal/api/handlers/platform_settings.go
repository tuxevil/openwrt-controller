package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

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
	apiKey := s.AIEngineAPIKey
	keyChanged := apiKey != "" && apiKey != "********"
	if !keyChanged {
		apiKey = current.AIEngineAPIKey
	}
	if s.AIEngineBaseURL == "" {
		s.AIEngineBaseURL = "https://api.openai.com/v1"
	}
	parsedURL, err := url.Parse(s.AIEngineBaseURL)
	localHTTP := parsedURL.Scheme == "http" && (parsedURL.Hostname() == "localhost" || parsedURL.Hostname() == "127.0.0.1" || parsedURL.Hostname() == "::1")
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "https" && !localHTTP) {
		http.Error(w, `{"error":"AI engine base URL must use HTTPS, except local providers"}`, http.StatusBadRequest)
		return
	}
	s.AIEngineBaseURL = strings.TrimRight(s.AIEngineBaseURL, "/")
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
