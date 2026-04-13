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

// FlowEntry represents a single destination flow from nf_conntrack
type FlowEntry struct {
	Proto     string `json:"proto"`
	Dst       string `json:"dst"`
	Dport     int    `json:"dport"`
	Conns     int    `json:"conns"`
	SampleSrc string `json:"sample_src"`
	// Enriched server-side
	Flagged bool   `json:"flagged"`
	Reason  string `json:"reason,omitempty"`
}

// Standard ports considered safe for general traffic
var standardSafePorts = map[int]bool{
	80: true, 443: true, 53: true, 123: true, // HTTP, HTTPS, DNS, NTP
	25: true, 465: true, 587: true, 993: true, 995: true, // Mail
	22: true, // SSH (legitimate outbound)
}

// Per-device throttle: avoid hammering Sentinel AI with flow alerts
var (
	flowSentinelMu    sync.Mutex
	flowSentinelCache = map[string]time.Time{}
)

const flowSentinelCooldown = 10 * time.Minute
const threatConnectionThreshold = 50

// ProcessFlowSense is called from TelemetryHandler for each telemetry push.
// It enriches flows with threat flags and triggers Sentinel AI if warranted.
func ProcessFlowSense(deviceID string, rawFlows []interface{}, controllerIP string) []FlowEntry {
	if len(rawFlows) == 0 {
		return nil
	}

	// Build whitelist: controller IP + common internal DNS/NTP
	whitelist := map[string]bool{
		controllerIP: true,
		"8.8.8.8":    true,
		"8.8.4.4":    true,
		"1.1.1.1":    true,
		"1.0.0.1":    true,
		"9.9.9.9":    true,
		"208.67.222.222": true,
		"127.0.0.1":  true,
	}

	// Also whitelist the 10.8.0.x WireGuard range (internal)
	isWireGuardInternal := func(ip string) bool {
		return strings.HasPrefix(ip, "10.8.0.")
	}

	var enriched []FlowEntry
	var suspicious []FlowEntry

	for _, raw := range rawFlows {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		entry := FlowEntry{
			Proto:     getString(m, "proto"),
			Dst:       getString(m, "dst"),
			Dport:     getInt(m, "dport"),
			Conns:     getInt(m, "conns"),
			SampleSrc: getString(m, "sample_src"),
		}

		// Skip whitelisted destinations
		if whitelist[entry.Dst] || isWireGuardInternal(entry.Dst) {
			enriched = append(enriched, entry)
			continue
		}

		// Skip private / RFC1918 destinations (internal traffic, not external threats)
		if isPrivateIP(entry.Dst) {
			enriched = append(enriched, entry)
			continue
		}

		// ── Threat Detection Logic ─────────────────────────────────────────────
		isFlagged := false
		reason := ""

		// 1. High volume to non-standard port
		if entry.Conns > threatConnectionThreshold && !standardSafePorts[entry.Dport] {
			isFlagged = true
			reason = fmt.Sprintf("HIGH_CONN_COUNT (%d connections) to non-standard port %d", entry.Conns, entry.Dport)
		}

		// 2. Common P2P/Torrent ports
		if !isFlagged && (entry.Dport == 6881 || entry.Dport == 6889 ||
			(entry.Dport >= 51413 && entry.Dport <= 51415) ||
			entry.Dport == 4662 || entry.Dport == 4672) {
			isFlagged = true
			reason = fmt.Sprintf("SUSPECTED_P2P port %d", entry.Dport)
		}

		// 3. Common C2/Tunnel ports
		if !isFlagged && (entry.Dport == 1194 || entry.Dport == 4444 ||
			entry.Dport == 9001 || entry.Dport == 9030 || // Tor
			entry.Dport == 8888 || entry.Dport == 31337) {
			isFlagged = true
			reason = fmt.Sprintf("SUSPECTED_TUNNEL_C2 port %d", entry.Dport)
		}

		if isFlagged {
			entry.Flagged = true
			entry.Reason = reason
			suspicious = append(suspicious, entry)
		}

		enriched = append(enriched, entry)
	}

	// ── Sentinel AI Escalation ─────────────────────────────────────────────────
	if len(suspicious) > 0 {
		flowSentinelMu.Lock()
		lastRun, seen := flowSentinelCache[deviceID]
		shouldRun := !seen || time.Since(lastRun) > flowSentinelCooldown
		if shouldRun {
			flowSentinelCache[deviceID] = time.Now()
		}
		flowSentinelMu.Unlock()

		if shouldRun {
			go escalateFlowToSentinel(deviceID, suspicious)
		}
	}

	return enriched
}

func escalateFlowToSentinel(deviceID string, suspicious []FlowEntry) {
	// Build a readable context for the LLM
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[FLOW_SENSE ALERT] Device: %s\n", deviceID))
	sb.WriteString("Suspicious outbound connection patterns detected:\n\n")
	for _, f := range suspicious {
		sb.WriteString(fmt.Sprintf(
			"  • Proto: %s | Dst: %s | Port: %d | Conns: %d | Reason: %s | SrcClient: %s\n",
			f.Proto, f.Dst, f.Dport, f.Conns, f.Reason, f.SampleSrc,
		))
	}
	sb.WriteString("\nAnalyze if this represents a security threat: P2P abuse, botnet C2, unauthorized tunnels, or data exfiltration. Provide actionable recommendation.")

	contextStr := sb.String()
	log.Printf("[FLOW_SENSE] Escalating %d suspicious flows to Sentinel AI for device %s", len(suspicious), deviceID)

	diagnosis, severity, involvedDevices, llmModel, tokensUsed, err := AnalyzeFleetContext(contextStr)
	if err != nil {
		log.Printf("[FLOW_SENSE] Sentinel AI error: %v", err)
		return
	}

	correlationID := fmt.Sprintf("FLOW-%s-%d", deviceID[:8], time.Now().Unix())
	involvedJSON, _ := json.Marshal(append(involvedDevices, deviceID))
	_, err = database.DB.Exec(`
		INSERT INTO ai_insights (correlation_id, diagnosis, severity, involved_devices, llm_model, tokens_used)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, correlationID, diagnosis, severity, string(involvedJSON), llmModel, tokensUsed)
	if err != nil {
		log.Printf("[FLOW_SENSE] DB insert error: %v", err)
	}

	sevUpper := strings.ToUpper(severity)
	if sevUpper == "HIGH" || sevUpper == "CRITICAL" {
		notifyTelegram(fmt.Sprintf("🔴 *FLOW_SENSE ALERT (Severity: %s)*\n\nDevice: `%s`\n\n%s", severity, deviceID, diagnosis))
	}

	log.Printf("[FLOW_SENSE] Sentinel analysis complete. Severity: %s", severity)
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func isPrivateIP(ip string) bool {
	return strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "172.16.") ||
		strings.HasPrefix(ip, "172.17.") ||
		strings.HasPrefix(ip, "172.18.") ||
		strings.HasPrefix(ip, "172.19.") ||
		strings.HasPrefix(ip, "172.20.") ||
		strings.HasPrefix(ip, "172.21.") ||
		strings.HasPrefix(ip, "172.22.") ||
		strings.HasPrefix(ip, "172.23.") ||
		strings.HasPrefix(ip, "172.24.") ||
		strings.HasPrefix(ip, "172.25.") ||
		strings.HasPrefix(ip, "172.26.") ||
		strings.HasPrefix(ip, "172.27.") ||
		strings.HasPrefix(ip, "172.28.") ||
		strings.HasPrefix(ip, "172.29.") ||
		strings.HasPrefix(ip, "172.30.") ||
		strings.HasPrefix(ip, "172.31.")
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}
