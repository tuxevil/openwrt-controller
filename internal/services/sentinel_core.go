package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/database"
)

var (
	lastSentinelRun time.Time
	sentinelMu      sync.Mutex
)

// AnalyzeLogs hooks into the LogHarvester ingestion stream to act as the Reactive Pipeline.
func AnalyzeLogs(deviceID string, logs []database.LogEntry) {
	triggers := []string{"panic", "OOM", "segfault", "auth.error", "denied", "hostapd: deauthenticated", "refused"}

	triggered := false
	for _, l := range logs {
		msgLower := strings.ToLower(l.Message)
		for _, t := range triggers {
			if strings.Contains(msgLower, strings.ToLower(t)) {
				triggered = true
				break
			}
		}
		if triggered {
			break
		}
	}

	if !triggered {
		return
	}

	sentinelMu.Lock()
	if time.Since(lastSentinelRun) < 5*time.Minute {
		sentinelMu.Unlock()
		return
	}
	lastSentinelRun = time.Now()
	sentinelMu.Unlock()

	go func(targetTime time.Time) {
		log.Println("[SENTINEL_AI] Critical trigger detected. Gathering fleet context...")

		contextLogs := database.GetGlobalContext(targetTime)
		if contextLogs == "" {
			return
		}

		diagnosis, severity, involvedDevices, err := AnalyzeFleetContext(contextLogs)
		if err != nil {
			log.Printf("[SENTINEL_AI] Inference engine error: %v", err)
			return
		}

		// Save to ai_insights
		correlationID := fmt.Sprintf("AI-CORR-%d", targetTime.Unix())
		involvedJSON, _ := json.Marshal(involvedDevices)

		_, err = database.DB.Exec(`
			INSERT INTO ai_insights (correlation_id, diagnosis, severity, involved_devices)
			VALUES ($1, $2, $3, $4)
		`, correlationID, diagnosis, severity, string(involvedJSON))

		if err != nil {
			log.Printf("[SENTINEL_AI] DB Insert error: %v", err)
			return
		}

		log.Printf("[SENTINEL_AI] Analysis complete. Severity: %s", severity)

		sevUpper := strings.ToUpper(severity)
		if sevUpper == "HIGH" || sevUpper == "CRITICAL" {
			msg := fmt.Sprintf("🚨 *SENTINEL ALERT (Severity: %s)*\n\n%s", severity, diagnosis)
			notifyTelegram(msg)
		}
	}(time.Now())
}
