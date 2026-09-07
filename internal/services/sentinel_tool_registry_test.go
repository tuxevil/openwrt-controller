package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSentinelToolRegistryContainsOnlyPhaseOneReadTools(t *testing.T) {
	descriptors := SentinelToolDescriptors()
	want := []string{
		"get_device_status",
		"get_incidents",
		"get_notes",
		"get_recent_changes",
		"get_site_clients",
		"get_topology",
		"search_logs",
	}
	if len(descriptors) != len(want) {
		t.Fatalf("registry contains %d tools, want %d: %#v", len(descriptors), len(want), descriptors)
	}
	for i, descriptor := range descriptors {
		if descriptor.Name != want[i] {
			t.Fatalf("descriptor[%d] = %q, want %q", i, descriptor.Name, want[i])
		}
		if descriptor.SideEffectClass != SentinelToolSideEffectNone {
			t.Fatalf("%s unexpectedly has side effects: %s", descriptor.Name, descriptor.SideEffectClass)
		}
		if descriptor.RiskClass != SentinelToolRiskLow {
			t.Fatalf("%s unexpectedly has phase-1 risk %s", descriptor.Name, descriptor.RiskClass)
		}
		if descriptor.ResultTrustClass != SentinelToolTrustUntrustedEvidence {
			t.Fatalf("%s result bypasses evidence boundary: %s", descriptor.Name, descriptor.ResultTrustClass)
		}
		if descriptor.RequiredScope != SentinelToolScopeTenant && descriptor.RequiredScope != SentinelToolScopeSite {
			t.Fatalf("%s has invalid scope %s", descriptor.Name, descriptor.RequiredScope)
		}
		if descriptor.TimeoutMillis <= 0 || descriptor.RateLimit.MaxCallsPerInvestigation <= 0 {
			t.Fatalf("%s is missing timeout/rate metadata: %#v", descriptor.Name, descriptor)
		}
		if descriptor.InputSchema.Type != "object" || descriptor.InputSchema.AdditionalProperties {
			t.Fatalf("%s input schema is not closed: %#v", descriptor.Name, descriptor.InputSchema)
		}
		if descriptor.OutputSchema.Type == "" {
			t.Fatalf("%s is missing output schema", descriptor.Name)
		}
	}
}

func TestSentinelToolRegistryRejectsUnknownAndControllerOwnedArguments(t *testing.T) {
	for _, call := range []SentinelToolCall{
		{Name: "run_shell", Arguments: json.RawMessage(`{"command":"id"}`)},
		{Name: "search_logs", Arguments: json.RawMessage(`{"schema":"tenant_other"}`)},
		{Name: "search_logs", Arguments: json.RawMessage(`{"command":"id"}`)},
		{Name: "search_logs", Arguments: json.RawMessage(`{"limit":1.5}`)},
		{Name: "search_logs", Arguments: json.RawMessage(`{"limit":1001}`)},
	} {
		if err := sentinelToolRegistry.ValidateModelCall(call); err == nil {
			t.Fatalf("unsafe call was accepted: %#v", call)
		}
	}
}

func TestSentinelToolRegistryAllowsControllerScopeInjection(t *testing.T) {
	call := SentinelToolCall{Name: "get_topology", Arguments: json.RawMessage(`{}`)}
	if err := sentinelToolRegistry.ValidateModelCall(call); err != nil {
		t.Fatalf("model call should allow the controller to inject the active site: %v", err)
	}
	bound := SentinelToolArgumentsWithSite(call.Arguments, "tenant_demo", "site-123")
	var args map[string]interface{}
	if err := json.Unmarshal(bound, &args); err != nil {
		t.Fatal(err)
	}
	if args["schema"] != "tenant_demo" || args["site_id"] != "site-123" {
		t.Fatalf("controller scope injection failed: %#v", args)
	}
}

func TestSentinelToolRegistryBudgetIsPerToolAndBounded(t *testing.T) {
	budget := NewSentinelToolBudget()
	descriptor, ok := sentinelToolRegistry.Descriptor("get_topology")
	if !ok {
		t.Fatal("get_topology missing from registry")
	}
	for i := 0; i < descriptor.RateLimit.MaxCallsPerInvestigation; i++ {
		if err := budget.Reserve("get_topology"); err != nil {
			t.Fatalf("reservation %d unexpectedly failed: %v", i, err)
		}
	}
	if err := budget.Reserve("get_topology"); err == nil {
		t.Fatal("per-tool investigation rate limit was not enforced")
	}
	if err := budget.Reserve("get_device_status"); err != nil {
		t.Fatalf("one tool exhausted another tool's budget: %v", err)
	}
}

func TestSentinelToolRegistryPromptCatalogIsDerivedFromRegistry(t *testing.T) {
	catalog := sentinelToolRegistry.PromptCatalog()
	for _, descriptor := range SentinelToolDescriptors() {
		if !strings.Contains(catalog, "- "+descriptor.Name+" [") {
			t.Fatalf("prompt catalog omitted %s: %s", descriptor.Name, catalog)
		}
	}
	for _, forbidden := range []string{"run_shell", "run_ssh", "raw_sql", "arbitrary_http"} {
		if strings.Contains(catalog, forbidden) {
			t.Fatalf("prompt catalog exposed forbidden primitive %q", forbidden)
		}
	}
}

func TestSentinelToolRegistryDescriptorsAreDefensiveCopies(t *testing.T) {
	first := SentinelToolDescriptors()
	first[0].InputSchema.Properties["evil"] = SentinelToolSchemaProperty{Type: "string"}
	first[0].RequiredDeviceCapabilities = append(first[0].RequiredDeviceCapabilities, "root-shell")

	second := SentinelToolDescriptors()
	if _, ok := second[0].InputSchema.Properties["evil"]; ok {
		t.Fatal("caller mutated trusted registry input schema")
	}
	if len(second[0].RequiredDeviceCapabilities) != 0 {
		t.Fatal("caller mutated trusted registry capability metadata")
	}
}

func TestSentinelToolRegistryFailsClosedOnMutationEntry(t *testing.T) {
	descriptor := sentinelToolDescriptor(
		"unsafe_mutator", "test", "must be rejected", SentinelToolScopeTenant,
		SentinelToolCostLow, 1000, 1, map[string]SentinelToolSchemaProperty{},
		SentinelToolSchema{Type: "object", AdditionalProperties: true},
	)
	descriptor.SideEffectClass = SentinelToolSideEffectClass("mutation")

	deferredPanic := false
	func() {
		defer func() {
			if recover() != nil {
				deferredPanic = true
			}
		}()
		_ = newSentinelToolRegistry([]sentinelRegisteredTool{{
			descriptor: descriptor,
			handler: func(context.Context, sentinelToolInvocation) (interface{}, error) {
				return nil, nil
			},
		}})
	}()
	if !deferredPanic {
		t.Fatal("registry accepted a mutation-capable tool")
	}
}
