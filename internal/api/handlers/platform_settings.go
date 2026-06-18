package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

func GetPlatformSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := database.GetPlatformSettings()

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

	query := `
		UPDATE platform_settings
		SET ollama_host = $1, ollama_model = $2, sentinel_prompt = $3, telegram_bot_token = $4, telegram_chat_id = $5,
		    global_surveys_public_lockdown = $6, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`
	_, err := database.Tx(r.Context()).Exec(query, s.OllamaHost, s.OllamaModel, s.SentinelPrompt, s.TelegramBotToken, s.TelegramChatID, s.GlobalSurveysPublicLockdown)
	if err != nil {
		http.Error(w, `{"error": "db update error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}
