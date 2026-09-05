package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

// criticalUCIFiles are the config files we care about for security compliance
var criticalUCIFiles = []string{
	"etc/config/firewall",
	"etc/config/wireless",
	"etc/config/dropbear",
	"etc/config/network",
	"etc/config/uhttpd",
}

const vaultAuditSystemPrompt = `You are Sentinel AI acting as a Zero-Trust Compliance Auditor for OpenWrt network equipment.
Analyze the provided UCI configuration files from a production router.

Detect the following security issues:
1. SSH (dropbear) exposed on WAN zone or listening on all interfaces (option Interface '')
2. Weak Wi-Fi encryption: WEP, WPA-TKIP, 'none', or empty encryption field
3. Overly permissive firewall rules: ACCEPT policies on INPUT/OUTPUT from wan zone, or disabled syn_flood protection
4. Web interface (uhttpd) exposed externally with no redirect to HTTPS
5. Any obvious security misconfigurations in the network config

Your output MUST follow this strict format:
- If vulnerabilities found, for EACH vulnerability use EXACTLY:

[ VULNERABILITY FOUND ] <description>
[ RISK LEVEL ] <Critical | High | Medium | Low>
[ REMEDIATION ] <exact UCI commands to fix this>

- If the configuration is fully secure, output only: NOMINAL STATE

Be concise and precise. No disclaimers. Provide exact, runnable UCI commands only.`

// AuditResult holds the parsed output of a vault compliance audit
type AuditResult struct {
	DeviceID   string `json:"device_id"`
	BackupID   string `json:"backup_id"`
	Diagnosis  string `json:"diagnosis"`
	Severity   string `json:"severity"`
	IsNominal  bool   `json:"is_nominal"`
	LLMModel   string `json:"llm_model"`
	TokensUsed int    `json:"tokens_used"`
}

// RunVaultAudit extracts UCI configs from the latest backup for a device,
// sends them to Sentinel AI for compliance analysis, and persists the result.
func RunVaultAudit(schema, deviceID string) (*AuditResult, error) {
	// 1. Pull the latest backup content from The Vault
	var backupID string
	var content []byte
	err := database.DB.QueryRow(`
		SELECT id, content FROM backups
		WHERE device_id = $1 AND content IS NOT NULL
		ORDER BY created_at DESC LIMIT 1
	`, deviceID).Scan(&backupID, &content)
	if err != nil {
		return nil, fmt.Errorf("no backup found for device %s: %w", deviceID, err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("backup content is empty for device %s", deviceID)
	}

	// 2. Extract UCI configs from the sysupgrade tar.gz
	configDump, err := extractUCIConfigs(content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract UCI configs from backup: %w", err)
	}
	if strings.TrimSpace(configDump) == "" {
		return nil, fmt.Errorf("no critical UCI config files found in backup (firewall/wireless/dropbear/network)")
	}

	log.Printf("[VAULT_AUDIT] Extracted %d bytes of UCI config for device %s. Sending to Sentinel AI...", len(configDump), deviceID)

	// 3. Query Sentinel AI with the compliance auditor system prompt
	diagnosis, llmModel, tokensUsed, err := completeAI(vaultAuditSystemPrompt, "UCI CONFIGURATION DUMP:\n"+configDump, false)
	if err != nil {
		return nil, fmt.Errorf("Sentinel AI inference failed: %w", err)
	}
	severity := parseAuditSeverity(diagnosis)

	isNominal := strings.Contains(strings.ToUpper(diagnosis), "NOMINAL STATE")

	// 4. Persist the audit in ai_insights
	correlationID := fmt.Sprintf("VAULT-AUDIT-%s-%d", deviceID[:8], time.Now().Unix())
	devicesJSON := fmt.Sprintf(`["%s"]`, deviceID)
	_, _ = database.DB.Exec(`
		INSERT INTO ai_insights (correlation_id, diagnosis, severity, involved_devices, llm_model, tokens_used)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, correlationID, diagnosis, severity, devicesJSON, llmModel, tokensUsed)

	log.Printf("[VAULT_AUDIT] Complete for %s | Severity: %s | Nominal: %v", deviceID, severity, isNominal)

	// 5. Telegram alert for high-severity findings
	if !isNominal && (strings.EqualFold(severity, "critical") || strings.EqualFold(severity, "high")) {
		notifyTelegram(fmt.Sprintf(
			"🔐 *VAULT AUDIT ALERT*\n\nDevice: `%s`\nSeverity: *%s*\n\n%s",
			deviceID, severity, diagnosis,
		))
	}

	return &AuditResult{
		DeviceID:   deviceID,
		BackupID:   backupID,
		Diagnosis:  diagnosis,
		Severity:   severity,
		IsNominal:  isNominal,
		LLMModel:   llmModel,
		TokensUsed: tokensUsed,
	}, nil
}

// extractUCIConfigs decompresses a sysupgrade tar.gz and returns the content
// of critical security configuration files concatenated.
func extractUCIConfigs(tarGzData []byte) (string, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(tarGzData))
	if err != nil {
		return "", fmt.Errorf("gzip decompress failed: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	var sb strings.Builder

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // return whatever we have
		}

		// Normalize path: remove leading `./ ` or `/`
		name := strings.TrimLeft(header.Name, "./")
		name = strings.TrimLeft(name, "/")

		isCritical := false
		for _, cf := range criticalUCIFiles {
			if name == cf {
				isCritical = true
				break
			}
		}
		if !isCritical {
			continue
		}

		fileBytes, err := io.ReadAll(tarReader)
		if err != nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n\n=== %s ===\n", name))
		sb.Write(fileBytes)
	}

	return sb.String(), nil
}

func parseAuditSeverity(content string) string {
	upper := strings.ToUpper(content)
	if strings.Contains(upper, "NOMINAL STATE") {
		return "nominal"
	}
	if strings.Contains(upper, "RISK LEVEL ] CRITICAL") || strings.Contains(upper, "RISK LEVEL: CRITICAL") {
		return "Critical"
	}
	if strings.Contains(upper, "RISK LEVEL ] HIGH") || strings.Contains(upper, "RISK LEVEL: HIGH") {
		return "High"
	}
	if strings.Contains(upper, "RISK LEVEL ] MEDIUM") || strings.Contains(upper, "RISK LEVEL: MEDIUM") {
		return "Medium"
	}
	return "Low"
}
