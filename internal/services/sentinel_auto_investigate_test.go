package services

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"openwrt-controller/internal/database"
)

func TestSentinelAutoInvestigateMinSeverityPolicy(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		severity string
		want     bool
	}{
		{name: "default allows critical", envVal: "", severity: "CRITICAL", want: true},
		{name: "default allows high", envVal: "", severity: "HIGH", want: true},
		{name: "default blocks warning", envVal: "", severity: "WARNING", want: false},
		{name: "default blocks medium", envVal: "", severity: "MEDIUM", want: false},
		{name: "critical only blocks high", envVal: "CRITICAL", severity: "HIGH", want: false},
		{name: "critical only allows critical", envVal: "CRITICAL", severity: "CRITICAL", want: true},
		{name: "medium allows warning", envVal: "MEDIUM", severity: "WARNING", want: true},
		{name: "off blocks critical", envVal: "OFF", severity: "CRITICAL", want: false},
		{name: "false blocks critical", envVal: "false", severity: "CRITICAL", want: false},
		{name: "all allows info", envVal: "ALL", severity: "INFO", want: true},
		{name: "invalid falls back to high", envVal: "BOGUS", severity: "MEDIUM", want: false},
		{name: "invalid still allows high", envVal: "BOGUS", severity: "HIGH", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == "SENTINEL_AUTO_INVESTIGATE_MIN_SEVERITY" {
					return tt.envVal
				}
				return ""
			}
			got := shouldAutoInvestigateSentinelCaseWithEnv(tt.severity, getenv)
			if got != tt.want {
				t.Fatalf("shouldAutoInvestigate(%q, env=%q) = %v, want %v", tt.severity, tt.envVal, got, tt.want)
			}
		})
	}
}

func TestSentinelAutoInvestigateCooldownEnvAndSlotDeduplication(t *testing.T) {
	if got := sentinelAutoInvestigateCooldownWithEnv(func(string) string { return "" }); got != 60*time.Minute {
		t.Fatalf("default cooldown = %v, want 60m", got)
	}
	if got := sentinelAutoInvestigateCooldownWithEnv(func(string) string { return "15" }); got != 15*time.Minute {
		t.Fatalf("custom cooldown = %v, want 15m", got)
	}
	if got := sentinelAutoInvestigateCooldownWithEnv(func(string) string { return "0" }); got != 60*time.Minute {
		t.Fatalf("invalid cooldown = %v, want 60m fallback", got)
	}

	sentinelCaseRunMu.Lock()
	previous := sentinelCaseRuns
	sentinelCaseRuns = map[string]time.Time{}
	sentinelCaseRunMu.Unlock()
	t.Cleanup(func() {
		sentinelCaseRunMu.Lock()
		sentinelCaseRuns = previous
		sentinelCaseRunMu.Unlock()
	})

	base := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	if !claimSentinelAutoInvestigationSlot("tenant_test", "case-1", "incident:dev1:SIGNAL_CRITICAL", base, time.Hour) {
		t.Fatal("first claim should succeed")
	}
	// A flapping incident opens a new case ID with the same fingerprint: must still be suppressed within cooldown.
	if claimSentinelAutoInvestigationSlot("tenant_test", "case-2", "incident:dev1:SIGNAL_CRITICAL", base.Add(10*time.Minute), time.Hour) {
		t.Fatal("flapping incident within cooldown must be suppressed by cooldownKey")
	}
	if !claimSentinelAutoInvestigationSlot("tenant_test", "case-2", "incident:dev1:SIGNAL_CRITICAL", base.Add(61*time.Minute), time.Hour) {
		t.Fatal("claim after cooldown should succeed")
	}
}

func TestSentinelCaseRecentlyAutoInvestigatedUsesLedger(t *testing.T) {
	now := time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)
	ledger := SentinelCaseEvidenceLedger{
		Version: sentinelCaseEvidenceLedgerVersion,
		Runs: []SentinelCaseEvidenceRun{
			{ID: "r1", Origin: "operator_chat", CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339)},
			{ID: "r2", Origin: "automation", CreatedAt: now.Add(-20 * time.Minute).Format(time.RFC3339)},
		},
	}
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !sentinelCaseRecentlyAutoInvestigated(raw, now, time.Hour) {
		t.Fatal("expected recent automation run to satisfy cooldown")
	}
	if sentinelCaseRecentlyAutoInvestigated(raw, now, 10*time.Minute) {
		t.Fatal("expected older automation run to allow re-investigation after shorter cooldown")
	}
}

func TestMatchSentinelLogAnomaliesIgnoresRoutineNoise(t *testing.T) {
	noisy := []database.LogEntry{
		{Message: "daemon.notice hostapd: phy0-ap0: AP-STA-DISCONNECTED aa:bb:cc:dd:ee:ff"},
		{Message: "daemon.info hostapd: deauthenticated due to inactivity"},
		{Message: "daemon.err hostapd: rmdir[ctrl_interface=/var/run/hostapd]: Permission denied"},
		{Message: "daemon.warn dropbear[123]: Exit before auth from <10.0.0.5:41234>"},
		{Message: "daemon.warn uhttpd: connection refused by upstream"},
	}
	if sev, matched := matchSentinelLogAnomalies(noisy); sev != "" || len(matched) != 0 {
		t.Fatalf("routine noise should not trigger anomalies: sev=%q matched=%v", sev, matched)
	}

	severe := []database.LogEntry{
		{Message: "kern.alert kernel: Out of memory: Killed process 422 (AdGuardHome)"},
		{Message: "auth.err sshd: authentication failure for root"},
	}
	sev, matched := matchSentinelLogAnomalies(severe)
	if sev != "HIGH" || len(matched) != 2 {
		t.Fatalf("severe logs = (%q, %d), want (HIGH, 2)", sev, len(matched))
	}
}

func TestDetectLocalBruteForceIPRequiresRepeatedAttempts(t *testing.T) {
	single := []database.LogEntry{
		{Message: "authpriv.warn dropbear[10]: Bad password attempt for 'root' from 192.168.1.50:55210"},
	}
	if ip := detectLocalBruteForceIP(single); ip != "" {
		t.Fatalf("single typo should not trigger brute force, got %q", ip)
	}

	repeated := []database.LogEntry{
		{Message: "authpriv.warn dropbear[10]: Bad password attempt for 'root' from 192.168.1.50:55210"},
		{Message: "authpriv.warn dropbear[11]: Bad password attempt for 'root' from 192.168.1.50:55211"},
		{Message: "authpriv.warn dropbear[12]: Bad password attempt for 'root' from 192.168.1.50:55212"},
	}
	if ip := detectLocalBruteForceIP(repeated); ip != "192.168.1.50" {
		t.Fatalf("repeated brute force IP = %q, want 192.168.1.50", ip)
	}
}

func TestClassifySuspiciousFlowSuppressesSingleStrayP2POrVPNConnection(t *testing.T) {
	if flagged, _ := classifySuspiciousFlow(FlowEntry{Dst: "90.129.127.28", Dport: 6881, Conns: 1}); flagged {
		t.Fatal("single connection on port 6881 should not flag P2P")
	}
	if flagged, _ := classifySuspiciousFlow(FlowEntry{Dst: "90.129.127.28", Dport: 6881, Conns: 5}); !flagged {
		t.Fatal("sustained P2P connections on port 6881 should flag")
	}
	if flagged, _ := classifySuspiciousFlow(FlowEntry{Dst: "198.51.100.10", Dport: 1194, Conns: 2}); flagged {
		t.Fatal("low-volume OpenVPN port 1194 should not flag")
	}
	if flagged, _ := classifySuspiciousFlow(FlowEntry{Dst: "198.51.100.10", Dport: 4444, Conns: 1}); !flagged {
		t.Fatal("high-confidence C2 port 4444 should flag")
	}
}

func TestSentinelOptionalTaskPermanentFailureThresholdAndCurationPrompt(t *testing.T) {
	if sentinelOptionalTaskShouldFailPermanently(1) || sentinelOptionalTaskShouldFailPermanently(2) {
		t.Fatal("tasks below max attempts should remain retryable")
	}
	if !sentinelOptionalTaskShouldFailPermanently(3) {
		t.Fatal("task at max attempts must fail permanently to prevent infinite loops")
	}
	if strings.Contains(sentinelLearningCurationSystemPrompt, "optional counter_evidence_ids") {
		t.Fatal("curation prompt must not contain ambiguous wording that induces optional_counter_evidence_ids")
	}
	if !strings.Contains(sentinelLearningCurationSystemPrompt, `"counter_evidence_ids"`) {
		t.Fatal("curation prompt must specify exact JSON key counter_evidence_ids")
	}
}
