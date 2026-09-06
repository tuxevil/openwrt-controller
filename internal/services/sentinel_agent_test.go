package services

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSentinelEnvelopeAcceptsToolCallsAndProposal(t *testing.T) {
	raw := `{"message":"I found a failed gateway login.","tool_calls":[{"name":"search_logs","arguments":{"query":"failed login","limit":20}}],"proposal":{"summary":"Block the source","device_id":"gw-1","config":"firewall","commands":[{"action":"set","config":"firewall","section":"rule_ai","option":"src","value":"192.0.2.10"}],"health_checks":["192.168.1.1"]}}`
	envelope, err := parseSentinelEnvelope(raw)
	if err != nil {
		t.Fatalf("parseSentinelEnvelope returned error: %v", err)
	}
	if len(envelope.ToolCalls) != 1 || envelope.ToolCalls[0].Name != "search_logs" {
		t.Fatalf("unexpected tool calls: %+v", envelope.ToolCalls)
	}
	if envelope.Proposal == nil || envelope.Proposal.Config != "firewall" {
		t.Fatalf("proposal was not parsed: %+v", envelope.Proposal)
	}
}

func TestValidateSentinelToolCallRejectsUnknownAndUnsafeArguments(t *testing.T) {
	unknown := SentinelToolCall{Name: "run_shell", Arguments: json.RawMessage(`{"command":"id"}`)}
	if err := validateSentinelToolCall(unknown); err == nil {
		t.Fatal("unknown tool should be rejected")
	}
	tooMany := SentinelToolCall{Name: "search_logs", Arguments: json.RawMessage(`{"limit":1001}`)}
	if err := validateSentinelToolCall(tooMany); err == nil {
		t.Fatal("tool limit should be bounded")
	}
	badDevice := SentinelToolCall{Name: "get_device_status", Arguments: json.RawMessage(`{"device_id":"gw-1; DROP TABLE devices"}`)}
	if err := validateSentinelToolCall(badDevice); err == nil {
		t.Fatal("device identifiers must reject SQL/shell syntax")
	}
	clients := SentinelToolCall{Name: "get_site_clients", Arguments: json.RawMessage(`{"site_id":"site-123"}`)}
	if err := validateSentinelToolCall(clients); err != nil {
		t.Fatalf("site client tool should be allowed: %v", err)
	}
}

func TestRedactSentinelSecretsRemovesCredentialsFromContext(t *testing.T) {
	input := `{"password":"secret-value","api_key":"key-value","nested":{"wg_privkey":"private-value"},"message":"normal"}`
	redacted := redactSentinelSecrets(input)
	for _, secret := range []string{"secret-value", "key-value", "private-value"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("secret %q leaked in %q", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("redaction marker missing: %q", redacted)
	}
}

func TestValidateSentinelProposalBlocksNetworkMutation(t *testing.T) {
	proposal := &SentinelProposalDraft{
		Summary:      "Change LAN address",
		DeviceID:     "gw-1",
		Config:       "network",
		Commands:     []UciCommand{{Action: "set", Config: "network", Section: "lan", Option: "ipaddr", Value: "192.168.2.1"}},
		HealthChecks: []string{"192.168.2.1"},
	}
	if err := validateSentinelProposal(proposal); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("network proposal should be blocked, got %v", err)
	}
}

func TestValidateSentinelProposalBlocksCredentialOptions(t *testing.T) {
	proposal := &SentinelProposalDraft{
		Summary:  "Change wireless key",
		DeviceID: "gw-1",
		Config:   "wireless",
		Commands: []UciCommand{{Action: "set", Config: "wireless", Section: "wifi-iface", Option: "key", Value: "super-secret"}},
	}
	if err := validateSentinelProposal(proposal); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("credential proposal should be blocked, got %v", err)
	}
}

func TestSentinelToolArgumentsWithSiteDefaultsSiteScope(t *testing.T) {
	args := SentinelToolArgumentsWithSite(json.RawMessage(`{"limit":25}`), "tenant_acme", "site-123")
	var decoded map[string]interface{}
	if err := json.Unmarshal(args, &decoded); err != nil {
		t.Fatalf("decode tool arguments: %v", err)
	}
	if decoded["schema"] != "tenant_acme" || decoded["site_id"] != "site-123" {
		t.Fatalf("site context was not injected: %#v", decoded)
	}
}

func TestSentinelToolArgumentsWithSitePreservesExplicitScope(t *testing.T) {
	args := SentinelToolArgumentsWithSite(json.RawMessage(`{"site_id":"site-other"}`), "tenant_acme", "site-current")
	var decoded map[string]interface{}
	if err := json.Unmarshal(args, &decoded); err != nil {
		t.Fatalf("decode tool arguments: %v", err)
	}
	if decoded["site_id"] != "site-other" {
		t.Fatalf("explicit site scope was overwritten: %#v", decoded)
	}
}

func TestBuildSentinelInvestigationPromptIncludesCurrentSite(t *testing.T) {
	prompt := buildSentinelInvestigationPromptForSite(nil, "How many clients are connected?", "site-123")
	if !strings.Contains(prompt, "CURRENT SITE CONTEXT") || !strings.Contains(prompt, "site-123") {
		t.Fatalf("site context missing from prompt: %s", prompt)
	}
}
