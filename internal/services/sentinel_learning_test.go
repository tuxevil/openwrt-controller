package services

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSentinelSkillRejectsUnknownPrimitive(t *testing.T) {
	spec := SentinelSkillSpec{
		SchemaVersion: 1,
		Name:          "unsafe-skill",
		EvidenceTools: []string{"run_shell"},
	}
	validation := ValidateSentinelSkillSpec(spec)
	if validation.Valid || !validation.Unsafe {
		t.Fatalf("validation = %#v, want invalid+unsafe", validation)
	}
}

func TestSentinelSkillStrictSchemaRejectsCodeFields(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version":1,
		"name":"bad-skill",
		"evidence_tools":["get_device_status"],
		"shell":"uci set wireless.foo=bar"
	}`)
	if _, err := decodeSentinelSkillSpec(raw); err == nil {
		t.Fatal("skill schema must reject arbitrary executable fields")
	}
}

func TestSentinelSkillAllowsOnlyRegisteredReadTools(t *testing.T) {
	spec := SentinelSkillSpec{
		SchemaVersion: 1,
		Name:          "wan-rca",
		EvidenceTools: []string{"get_device_status", "get_incidents"},
	}
	validation := ValidateSentinelSkillSpec(spec)
	if !validation.Valid || validation.Unsafe {
		t.Fatalf("validation = %#v", validation)
	}
	if len(validation.ValidatedTools) != 2 {
		t.Fatalf("validated tools = %v", validation.ValidatedTools)
	}
}

func TestSentinelHistoricalEvidenceHonorsAsOf(t *testing.T) {
	before := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	after := before.Add(time.Hour)
	ledger := SentinelCaseEvidenceLedger{
		Version: sentinelCaseEvidenceLedgerVersion,
		Runs: []SentinelCaseEvidenceRun{{Records: []SentinelCaseEvidenceRecord{
			{SentinelEvidenceReference: SentinelEvidenceReference{Tool: "get_device_status", CapturedAt: before.Format(time.RFC3339Nano)}},
			{SentinelEvidenceReference: SentinelEvidenceReference{Tool: "get_incidents", CapturedAt: after.Format(time.RFC3339Nano)}},
		}}},
	}
	available := sentinelHistoricalEvidenceToolsAt(ledger, before.Add(time.Minute))
	if !available["get_device_status"] {
		t.Fatal("evidence available before replay timestamp was omitted")
	}
	if available["get_incidents"] {
		t.Fatal("historical replay observed evidence from the future")
	}
}

func TestSentinelSkillPromotionThresholdsAreDeterministic(t *testing.T) {
	stats := SentinelSkillPromotionStats{
		ReplayCount: SentinelSkillPromotionMinReplay,
		ReplaySuccessRate: SentinelSkillPromotionMinSuccessRatio,
		ShadowCount: SentinelSkillPromotionMinShadow,
		ShadowSuccessRate: SentinelSkillPromotionMinSuccessRatio,
	}
	if !sentinelSkillCanEnterShadow(stats) || !sentinelSkillCanPromote(stats) {
		t.Fatalf("threshold stats should promote: %#v", stats)
	}
	stats.UnsafeCount = 1
	if sentinelSkillCanEnterShadow(stats) || sentinelSkillCanPromote(stats) {
		t.Fatal("any unsafe evaluation must block promotion")
	}
}

func TestSentinelSkillMatchIsDeclarative(t *testing.T) {
	spec := SentinelSkillSpec{
		SchemaVersion: 1,
		Name:          "wan-loss",
		Match: SentinelSkillMatch{
			Sources:    []string{"log_anomaly"},
			Severities: []string{"high"},
			Keywords:   []string{"packet loss"},
		},
		EvidenceTools: []string{"get_device_status"},
	}
	item := SentinelCase{Source: "log_anomaly", Severity: "HIGH", Title: "WAN packet loss detected"}
	if !sentinelSkillMatches(spec, item, "investigate packet loss") {
		t.Fatal("expected declarative selector to match")
	}
	item.Severity = "LOW"
	if sentinelSkillMatches(spec, item, "investigate packet loss") {
		t.Fatal("severity selector should prevent match")
	}
}

func TestSentinelLearningTTLIsBounded(t *testing.T) {
	if _, err := sentinelLearningTTL(int(SentinelLearningMaxTTL/time.Hour) + 1); err == nil {
		t.Fatal("TTL above controller maximum should be rejected")
	}
	if ttl, err := sentinelLearningTTL(0); err != nil || ttl != SentinelLearningDefaultTTL {
		t.Fatalf("default ttl=%v err=%v", ttl, err)
	}
}
