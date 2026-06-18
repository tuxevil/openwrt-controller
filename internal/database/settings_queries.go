package database

type PlatformSettings struct {
	OllamaHost                  string `json:"ollama_host"`
	OllamaModel                 string `json:"ollama_model"`
	SentinelPrompt              string `json:"sentinel_prompt"`
	TelegramBotToken            string `json:"telegram_bot_token"`
	TelegramChatID              string `json:"telegram_chat_id"`
	GlobalSurveysPublicLockdown bool   `json:"global_surveys_public_lockdown"`
}

// GetPlatformSettings fetches global platform settings
func GetPlatformSettings() PlatformSettings {
	var s PlatformSettings
	err := DB.QueryRow(`
		SELECT ollama_host, ollama_model, sentinel_prompt, telegram_bot_token, telegram_chat_id,
		       COALESCE(global_surveys_public_lockdown, false)
		FROM platform_settings WHERE id = 1
	`).Scan(&s.OllamaHost, &s.OllamaModel, &s.SentinelPrompt, &s.TelegramBotToken, &s.TelegramChatID,
		&s.GlobalSurveysPublicLockdown)

	if err != nil {
		// Provide basic defaults if the DB somehow fails
		s.OllamaHost = "127.0.0.1:11434"
		s.OllamaModel = "llama3"
		s.SentinelPrompt = "You are a Fleet Security Analyst. Analyze this cross-device log stream. Look for coordinated attacks, lateral movements, or cascading hardware failures. If Device A shows a login failure and Device B shows a login success from the same IP, flag it as CRITICAL SUSPICION. Be technical, concise, and provide a 'Recommended Action'. The output must look like a high-level SOC report. No fluff.\n\nEnd your report with these two exact lines at the bottom for parsing:\nSEVERITY: [Critical, High, Medium, Low]\nDEVICES: [Device_Name_1, Device_Name_2]"
	}

	return s
}
