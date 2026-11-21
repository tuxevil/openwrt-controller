package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"openwrt-controller/internal/database"
)

const sentinelBruteForceMinAttempts = 3

func detectLocalBruteForceIP(logs []database.LogEntry) string {
	attemptsByIP := make(map[string]int)
	for _, l := range logs {
		msgLower := strings.ToLower(l.Message)
		if !strings.Contains(msgLower, "bad password") || !strings.Contains(msgLower, "from ") {
			continue
		}
		parts := strings.SplitN(msgLower, "from ", 2)
		if len(parts) != 2 {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) == 0 {
			continue
		}
		ipPort := strings.Trim(fields[0], "<>")
		ipOnly := strings.Split(ipPort, ":")[0]
		if !strings.HasPrefix(ipOnly, "192.168.") && !strings.HasPrefix(ipOnly, "10.") {
			continue
		}
		attemptsByIP[ipOnly]++
		if attemptsByIP[ipOnly] >= sentinelBruteForceMinAttempts {
			return ipOnly
		}
	}
	return ""
}

func matchSentinelLogAnomalies(logs []database.LogEntry) (string, []string) {
	highTriggers := []string{"kernel panic", "panic:", "out of memory", "oom-killer", "segfault", "oops:", "call trace:"}
	mediumTriggers := []string{"auth.error", "authentication failure"}

	matched := make([]string, 0, 10)
	severity := ""
	for _, l := range logs {
		messageLower := strings.ToLower(l.Message)
		hit := false
		for _, trigger := range highTriggers {
			if strings.Contains(messageLower, trigger) {
				severity = "HIGH"
				hit = true
				break
			}
		}
		if !hit {
			for _, trigger := range mediumTriggers {
				if strings.Contains(messageLower, trigger) {
					if severity == "" {
						severity = "MEDIUM"
					}
					hit = true
					break
				}
			}
		}
		if hit {
			matched = append(matched, redactSentinelSecrets(l.Message))
			if len(matched) == 10 {
				break
			}
		}
	}
	return severity, matched
}

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
	if targetIP := detectLocalBruteForceIP(logs); targetIP != "" && len(stateJSON) > 0 {
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
						QueueAutoSentinelCaseInvestigation(schema, caseID, "HIGH", "brute_force:"+deviceID+":"+targetIP)
					} else {
						log.Printf("[SENTINEL_AI] failed to persist brute-force case: %v", caseErr)
					}
				}
			}
		}
	}

	severity, matched := matchSentinelLogAnomalies(logs)
	if len(matched) == 0 {
		return
	}

	caseID, caseErr := OpenSentinelCasePreservingEvidence(schema, "log_anomaly", siteID, deviceID, severity,
		"Reactive log anomaly", "A Sentinel log trigger matched on the device. Current state must be re-read from OMEGA during investigation.", map[string]interface{}{
			"matched_log_samples": matched,
			"sample_count":        len(matched),
		})
	if caseErr != nil {
		log.Printf("[SENTINEL_AI] failed to persist reactive log Case: %v", caseErr)
		return
	}
	QueueAutoSentinelCaseInvestigation(schema, caseID, severity, "log_anomaly:"+deviceID)
}
