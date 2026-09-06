package handlers

import (
	"strings"
	"testing"

	"openwrt-controller/internal/services"
)

func TestDecodePendingDeviceOperationAcceptsTypedPlan(t *testing.T) {
	raw := `{"operation_id":"operation-1","plan_hash":"operation-1","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`
	plan, err := decodePendingDeviceOperation([]byte(raw))
	if err != nil {
		t.Fatalf("valid operation was rejected: %v", err)
	}
	if plan.OperationID != "operation-1" || plan.Config != "system" {
		t.Fatalf("unexpected decoded plan: %#v", plan)
	}
}

func TestDecodePendingDeviceOperationRejectsInvalidPlans(t *testing.T) {
	cases := []string{
		"not-json",
		`{"operation_id":"operation-1","plan_hash":"different","config":"system","commands":[]}`,
		`{"operation_id":"operation-1","plan_hash":"operation-1","config":"openvpn","commands":[{"action":"set","config":"openvpn","section":"client","option":"enabled","value":"1"}]}`,
		`{"operation_id":"operation;1","plan_hash":"operation;1","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]}`,
	}
	for _, raw := range cases {
		if _, err := decodePendingDeviceOperation([]byte(raw)); err == nil {
			t.Fatalf("invalid operation was accepted: %s", raw)
		}
	}
}

func TestDecodePendingDeviceOperationPreservesHealthChecks(t *testing.T) {
	raw := `{"operation_id":"network-1","plan_hash":"network-1","config":"network","health_checks":["10.128.128.1"],"commands":[{"action":"set","config":"network","section":"lan","option":"metric","value":"10"}]}`
	plan, err := decodePendingDeviceOperation([]byte(raw))
	if err != nil {
		t.Fatalf("network operation was rejected: %v", err)
	}
	if got := strings.Join(plan.HealthChecks, ","); got != "10.128.128.1" {
		t.Fatalf("health checks changed during decode: %q", got)
	}
	if err := services.ValidateDeviceOperation(plan.Config, plan.Commands, plan.HealthChecks); err != nil {
		t.Fatalf("decoded plan no longer validates: %v", err)
	}
}
