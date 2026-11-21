package services

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoamingEvidenceKeepsStationDestinationAndWindow(t *testing.T) {
	const data = `[
		{"timestamp":"2026-11-21T14:00:00Z","device_id":"ap-2","message":"hostapd: phy1-ap0: STA 02:00:00:00:00:01 IEEE 802.11: association rejected"},
		{"timestamp":"2026-11-21T14:00:00Z","device_id":"ap-2","message":"hostapd: phy1-ap0: AP-STA-CONNECTED 02:00:00:00:00:02 auth_alg=ft"},
		{"timestamp":"2026-11-21T14:00:01Z","device_id":"ap-1","message":"hostapd: phy1-ap0: AP-STA-CONNECTED 02:00:00:00:00:01 auth_alg=open"},
		{"timestamp":"2026-11-21T14:10:00Z","device_id":"ap-2","message":"hostapd: phy1-ap0: AP-STA-CONNECTED 02:00:00:00:00:01 auth_alg=ft"},
		{"timestamp":"2026-11-21T14:00:01Z","device_id":"ap-2","message":"kernel: copied hostapd: phy1-ap0: AP-STA-CONNECTED 02:00:00:00:00:01 auth_alg=ft"}
	]`
	result := sentinelRoamingEvidence("search_logs", data)
	var decoded struct {
		Observations json.RawMessage `json:"observations"`
		Roaming      []struct {
			Station           string
			Outcome           string
			FollowupAt        string `json:"followup_at"`
			TrafficContinuity string `json:"traffic_continuity"`
		} `json:"roaming_events"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Roaming) != 4 || decoded.Roaming[0].Outcome != "association_rejected" ||
		decoded.Roaming[0].FollowupAt != "" || decoded.Roaming[0].TrafficContinuity != "unknown" ||
		decoded.Roaming[1].Station != "02:00:00:00:00:02" || decoded.Roaming[1].Outcome != "ft_connected" ||
		decoded.Roaming[2].Outcome != "association_connected" {
		t.Fatalf("unexpected roaming assessment: %+v", decoded.Roaming)
	}
	if !json.Valid(decoded.Observations) || !strings.Contains(string(decoded.Observations), "ap-1") {
		t.Fatal("original observations must remain available")
	}
}

func TestRoamingEvidenceLinksLaterSuccessWithoutErasingRejection(t *testing.T) {
	const data = `[
		{"timestamp":"2026-11-21T14:00:01Z","device_id":"ap-2","message":"hostapd: phy1-ap0: AP-STA-CONNECTED 02:00:00:00:00:01 auth_alg=ft"},
		{"timestamp":"2026-11-21T14:00:00Z","device_id":"ap-2","message":"hostapd: phy1-ap0: STA 02:00:00:00:00:01 IEEE 802.11: association rejected"}
	]`
	got := sentinelRoamingEvidence("search_logs", data)
	if !strings.Contains(got, `"followup_at":"2026-11-21T14:00:01Z"`) || !strings.Contains(got, `"outcome":"association_rejected"`) {
		t.Fatalf("expected scoped follow-up and original rejection: %s", got)
	}
}

func TestRoamingQuestionPrefetchesHostapdLogs(t *testing.T) {
	plan := sentinelContextToolPlan("¿Falló el roaming del cliente al gateway?", SentinelCase{SiteID: "site"})
	if len(plan) == 0 || plan[0] != "search_logs" {
		t.Fatalf("expected roaming logs ahead of generic connectivity tools: %v", plan)
	}
}
