package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"openwrt-controller/internal/database"
)

// AnalyzeLogs hooks into the LogHarvester ingestion stream. Reactive AI work
// is represented as a durable Case before any model call so automation and
// operator chat share the same evidence/context path.
func AnalyzeLogs(schema, deviceID string, logs []database.LogEntry) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil || database.DB == nil {
		return
	}
	var siteID string
	var stateJSON []byte
	// #nosec G201 -- safeSchema is validated by sentinelSchema; deviceID is parameterized.
	_ = database.DB.QueryRow(fmt.Sprintf("SELECT COALESCE(site_id::text,''), state_json FROM %s.devices WHERE id = $1", safeSchema), deviceID).Scan(&siteID, &stateJSON)

	// Preserve the more specific brute-force Case when the source can be
	// resolved to a local endpoint. Any response remains proposal/approval
	// gated; this detector only creates evidence and schedules investigation.
	triggerSniper := false
	var targetIP string
	for _, l := range logs {
		msgLower := strings.ToLower(l.Message)
		if strings.Contains(msgLower, "bad password") && strings.Contains(msgLower, "from") {
			parts := strings.Split(msgLower, "from ")
			if len(parts) == 2 {
				ipPort := strings.Fields(parts[1])[0]
				ipPort = strings.Trim(ipPort, "<>")
				ipOnly := strings.Split(ipPort, ":")[0]
				if strings.HasPrefix(ipOnly, "192.168.") || strings.HasPrefix(ipOnly, "10.") {
					targetIP = ipOnly
					triggerSniper = true
					break
				}
			}
		}
	}

	if triggerSniper && targetIP != "" && len(stateJSON) > 0 {
		var state map[string]interface{}
		if json.Unmarshal(stateJSON, &state) == nil {
			if arp, ok := state["arp_table"].([]interface{}); ok {
				var targetMAC string
				for _, entry := range arp {
					if e, ok := entry.(map[string]interface{}); ok {
						if ip, ok := e["ip"].(string); ok && ip == targetIP {
							if mac, ok := e["mac"].(string); ok {
								targetMAC = mac
								break
							}
						}
					}
				}
				if targetMAC != "" {
					caseID, caseErr := OpenSentinelCasePreservingEvidence(schema, "brute_force", siteID, deviceID, "HIGH",
						"Local brute force detected", fmt.Sprintf("Failed authentication from %s resolved to %s", targetIP, targetMAC), map[string]string{
							"source_ip":  targetIP,
							"source_mac": targetMAC,
						})
					if caseErr == nil {
						log.Printf("[SENTINEL_AI] Local brute force detected from %s. Case %s queued.", targetIP, caseID)
						QueueSentinelCaseContextInvestigation(schema, caseID)
					} else {
						log.Printf("[SENTINEL_AI] failed to persist brute-force case: %v", caseErr)
					}
				}
			}
		}
	}

	triggers := []string{"panic", "oom", "segfault", "auth.error", "denied", "hostapd: deauthenticated", "refused", "bad password", "exit before auth"}
	matched := make([]string, 0, 10)
	for _, l := range logs {
		messageLower := strings.ToLower(l.Message)
		for _, trigger := range triggers {
			if strings.Contains(messageLower, trigger) {
				matched = append(matched, redactSentinelSecrets(l.Message))
				break
			}
		}
		if len(matched) == 10 {
			break
		}
	}
	if len(matched) == 0 {
		return
	}

	caseID, caseErr := OpenSentinelCasePreservingEvidence(schema, "log_anomaly", siteID, deviceID, "MEDIUM",
		"Reactive log anomaly", "A Sentinel log trigger matched on the device. Current state must be re-read from OMEGA during investigation.", map[string]interface{}{
			"matched_log_samples": matched,
			"sample_count":        len(matched),
		})
	if caseErr != nil {
		log.Printf("[SENTINEL_AI] failed to persist reactive log Case: %v", caseErr)
		return
	}
	QueueSentinelCaseContextInvestigation(schema, caseID)
}
