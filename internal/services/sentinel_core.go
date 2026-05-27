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
func AnalyzeLogs(schema, deviceID string, logs []database.LogEntry) {
	// 1. Evaluate Sniper Shaping (Active Defense) BEFORE any debounce or AI trigger logic
	triggerSniper := false
	var targetIP string
	for _, l := range logs {
		msg := l.Message
		msgLower := strings.ToLower(msg)
		// E.g. Bad password attempt for 'root' from 192.168.1.100:1234
		// Or: Exit before auth from <10.0.0.144:46794>
		if strings.Contains(msgLower, "bad password") && strings.Contains(msgLower, "from") {
			parts := strings.Split(msgLower, "from ")
			// parts[1] will be something like "10.0.0.144:46794" or "<10.0.0.144:46794>..."
			if len(parts) == 2 {
				ipPort := strings.Fields(parts[1])[0]
				ipPort = strings.Trim(ipPort, "<>") // Strip potential < > brackets from dropbear logs
				ipOnly := strings.Split(ipPort, ":")[0]
				if strings.HasPrefix(ipOnly, "192.168.") || strings.HasPrefix(ipOnly, "10.") {
					targetIP = ipOnly
					triggerSniper = true
					break
				}
			}
		}
	}

	if triggerSniper && targetIP != "" {
		log.Printf("[SENTINEL_AI] Local Brute Force detected from %s. Deploying Preventive Sniper.", targetIP)
		
		// Attempt to resolve MAC from ARP table
		var stateJSON []byte
		err := database.DB.QueryRow(fmt.Sprintf("SELECT state_json FROM %s.devices WHERE id = $1", schema), deviceID).Scan(&stateJSON)
		if err == nil && len(stateJSON) > 0 {
			var state map[string]interface{}
			if json.Unmarshal(stateJSON, &state) == nil {
				if arp, ok := state["arp_table"].([]interface{}); ok {
					var targetMac string
					for _, entry := range arp {
						if e, ok := entry.(map[string]interface{}); ok {
							if ip, ok := e["ip"].(string); ok && ip == targetIP {
								if m, ok := e["mac"].(string); ok {
									targetMac = m
									break
								}
							}
						}
					}
					
					if targetMac != "" {
						err := ApplySniperShaping(schema, deviceID, targetMac, 64, 5) // Hard limit 64 KB/s for 5 mins
						if err == nil {
							log.Printf("[SENTINEL_AI] Sniper Shaping applied to %s (%s)", targetIP, targetMac)
							notifyTelegram(fmt.Sprintf("🛡️ *ACTIVE DEFENSE TRIGGERED*\n\nLocal brute force detected from %s (%s).\nSniper Shaping deployed for 5 minutes.", targetIP, targetMac))
						}
					}
				}
			}
		}
	}

	// 2. Evaluate AI Inference triggers
	triggers := []string{"panic", "OOM", "segfault", "auth.error", "denied", "hostapd: deauthenticated", "refused", "bad password", "exit before auth"}

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

		contextLogs := database.GetGlobalContext(schema, targetTime, 100)
		if contextLogs == "" {
			return
		}

		diagnosis, severity, involvedDevices, _, _, err := AnalyzeFleetContext(contextLogs)
		if err != nil {
			log.Printf("[SENTINEL_AI] Inference engine error: %v", err)
			return
		}

		// Save to ai_insights
		correlationID := fmt.Sprintf("AI-CORR-%d", targetTime.Unix())
		involvedJSON, _ := json.Marshal(involvedDevices)

		_, err = database.DB.Exec(fmt.Sprintf(`
			INSERT INTO %s.ai_insights (correlation_id, diagnosis, severity, involved_devices)
			VALUES ($1, $2, $3, $4)
		`, schema), correlationID, diagnosis, severity, string(involvedJSON))

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
