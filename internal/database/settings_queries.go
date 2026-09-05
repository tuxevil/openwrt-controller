package database

type PlatformSettings struct {
	AIEngineBaseURL             string `json:"ai_engine_base_url"`
	AIEngineModel               string `json:"ai_engine_model"`
	AIEngineAPIKey              string `json:"ai_engine_api_key,omitempty"`
	AIEngineConfigured          bool   `json:"ai_engine_configured"`
	SentinelPrompt              string `json:"sentinel_prompt"`
	TelegramBotToken            string `json:"telegram_bot_token"`
	TelegramChatID              string `json:"telegram_chat_id"`
	GlobalSurveysPublicLockdown bool   `json:"global_surveys_public_lockdown"`
}

// GetPlatformSettings fetches global platform settings
func GetPlatformSettings() PlatformSettings {
	var s PlatformSettings
	err := DB.QueryRow(`
		SELECT ai_engine_base_url, ai_engine_model, ai_engine_api_key, sentinel_prompt, telegram_bot_token, telegram_chat_id,
		       COALESCE(global_surveys_public_lockdown, false)
		FROM platform_settings WHERE id = 1
	`).Scan(&s.AIEngineBaseURL, &s.AIEngineModel, &s.AIEngineAPIKey, &s.SentinelPrompt, &s.TelegramBotToken, &s.TelegramChatID,
		&s.GlobalSurveysPublicLockdown)

	if err != nil {
		// Provide basic defaults if the DB somehow fails
		s.AIEngineBaseURL = "https://api.openai.com/v1"
		s.AIEngineModel = "gpt-4o-mini"
		s.SentinelPrompt = "You are a Fleet Security Analyst. Analyze this cross-device log stream. Look for coordinated attacks, lateral movements, or cascading hardware failures. If Device A shows a login failure and Device B shows a login success from the same IP, flag it as CRITICAL SUSPICION. Be technical, concise, and provide a 'Recommended Action'. The output must look like a high-level SOC report. No fluff.\n\nEnd your report with these two exact lines at the bottom for parsing:\nSEVERITY: [Critical, High, Medium, Low]\nDEVICES: [Device_Name_1, Device_Name_2]"
	}
	s.AIEngineConfigured = s.AIEngineAPIKey != ""

	return s
}
