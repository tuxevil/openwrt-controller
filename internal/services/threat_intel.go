package services

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/database"
)

const threatShieldListFile = "/tmp/threat_shield_combined.txt"

var (
	threatMu      sync.RWMutex
	threatList    []string
	threatLastUpd time.Time
	threatIPCount int
)

// threatSources defines the reputation lists to aggregate
var threatSources = []struct {
	name string
	url  string
}{
	{
		name: "Firehol Level 1",
		url:  "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset",
	},
	{
		name: "Emerging Threats Compromised",
		url:  "https://rules.emergingthreats.net/blockrules/compromised-ips.txt",
	},
	{
		name: "Spamhaus DROP",
		url:  "https://www.spamhaus.org/drop/drop.txt",
	},
}

// StartThreatIntelCron starts the 12-hour background threat intelligence worker.
func StartThreatIntelCron() {
	go func() {
		log.Println("[THREAT_SHIELD] Starting initial blocklist download...")
		fetchAndMergeLists()

		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("[THREAT_SHIELD] Scheduled 12h blocklist refresh...")
			fetchAndMergeLists()
		}
	}()
}

// fetchAndMergeLists downloads all configured threat feeds, deduplicates,
// writes a flat file, and updates the in-memory list + DB metadata.
func fetchAndMergeLists() {
	client := &http.Client{Timeout: 60 * time.Second}
	seen := map[string]bool{}
	var combined []string
	successfulSources := 0

	for _, source := range threatSources {
		log.Printf("[THREAT_SHIELD] Fetching: %s ...", source.name)

		resp, err := client.Get(source.url)
		if err != nil {
			log.Printf("[THREAT_SHIELD] ⚠ Fetch failed for %s: %v", source.name, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[THREAT_SHIELD] ⚠ Read failed for %s: %v", source.name, err)
			continue
		}

		added := 0
		scanner := bufio.NewScanner(bytes.NewReader(body))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip comments and empty lines
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
				continue
			}
			// Take only the first token (handles "CIDR ; comment" format)
			token := strings.Fields(line)[0]
			token = strings.TrimRight(token, ";,")

			// Basic sanity: must start with a digit (IPv4) or digit-like prefix
			if token == "" || token[0] < '0' || token[0] > '9' {
				continue
			}

			if !seen[token] {
				seen[token] = true
				combined = append(combined, token)
				added++
			}
		}
		log.Printf("[THREAT_SHIELD] %s: +%d unique IPs/CIDRs", source.name, added)
		successfulSources++
	}

	if len(combined) == 0 {
		log.Println("[THREAT_SHIELD] ⚠ No IPs retrieved this cycle. Keeping existing list.")
		return
	}

	// Persist flat file (one IP/CIDR per line)
	content := strings.Join(combined, "\n") + "\n"
	if err := os.WriteFile(threatShieldListFile, []byte(content), 0644); err != nil {
		log.Printf("[THREAT_SHIELD] ⚠ Failed to write list file: %v", err)
	}

	// Update in-memory state
	threatMu.Lock()
	threatList = combined
	threatIPCount = len(combined)
	threatLastUpd = time.Now()
	threatMu.Unlock()

	// Persist metadata to DB for all tenants
	tenants, _ := ListTenants()
	for _, t := range tenants {
		schema := "tenant_" + t.SchemaAlias
		_, _ = database.DB.Exec(fmt.Sprintf(
			"INSERT INTO %s.threat_intel_meta (fetched_at, ip_count, sources_count) VALUES ($1, $2, $3)",
			schema), time.Now(), len(combined), successfulSources,
		)
	}

	log.Printf("[THREAT_SHIELD] ✓ Blocklist ready: %d unique IPs/CIDRs from %d sources",
		len(combined), successfulSources)
}

// GetThreatListContent returns the current blocklist as plain text (one entry per line).
func GetThreatListContent() string {
	threatMu.RLock()
	defer threatMu.RUnlock()

	if len(threatList) > 0 {
		return strings.Join(threatList, "\n") + "\n"
	}

	// Fallback: read from disk
	data, err := os.ReadFile(threatShieldListFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// GetThreatIntelStatus returns current status metadata.
func GetThreatIntelStatus() map[string]interface{} {
	threatMu.RLock()
	defer threatMu.RUnlock()

	lastUpdStr := ""
	if !threatLastUpd.IsZero() {
		lastUpdStr = threatLastUpd.Format(time.RFC3339)
	}

	// Also query DB for the latest record
	var dbFetchedAt string
	var dbIPCount int
	// For global status, check the first tenant's meta as a proxy, or skip DB fallback. We'll skip DB fallback for simplicity.
	// We just use the in-memory variables.
	_ = dbFetchedAt

	count := threatIPCount
	if count == 0 {
		count = dbIPCount
	}

	return map[string]interface{}{
		"ip_count":     count,
		"last_updated": lastUpdStr,
		"db_record":    dbFetchedAt,
		"sources":      len(threatSources),
		"active":       count > 0,
		"list_file":    threatShieldListFile,
	}
}

// FormatThreatShieldNFT generates a self-contained nft script to flush and reload
// the denylist set. Used for debugging/manual apply; the agent generates its own.
func FormatThreatShieldNFT() string {
	content := GetThreatListContent()
	if content == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("flush set inet threat_shield denylist\n")
	sb.WriteString("add element inet threat_shield denylist {\n")
	lines := strings.Split(strings.TrimSpace(content), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i < len(lines)-1 {
			sb.WriteString(fmt.Sprintf("  %s,\n", line))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}
	sb.WriteString("}\n")
	return sb.String()
}
