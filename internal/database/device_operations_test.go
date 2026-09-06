package database

import (
	"encoding/json"
	"testing"
)

func TestParseDeviceOperationEnvelopeRequiresStableIdentity(t *testing.T) {
	valid := json.RawMessage(`{"operation_id":"operation-1","plan_hash":"operation-1"}`)
	got, err := parseDeviceOperationEnvelope(valid)
	if err != nil || got != "operation-1" {
		t.Fatalf("valid envelope = %q, error %v", got, err)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"operation_id":"operation-1"}`),
		json.RawMessage(`{"operation_id":"operation-1","plan_hash":"different"}`),
		json.RawMessage(`{"operation_id":"operation;1","plan_hash":"operation;1"}`),
	} {
		if _, err := parseDeviceOperationEnvelope(raw); err == nil {
			t.Fatalf("invalid envelope was accepted: %s", raw)
		}
	}
}

func TestValidDeviceOperationStateUsesKnownVocabulary(t *testing.T) {
	for _, state := range []string{"APPLYING", "PENDING_CONFIRM", "ROLLING_BACK", "COMMITTED", "RESTORED", "RECOVERY_REQUIRED"} {
		if !validDeviceOperationState(state) {
			t.Errorf("known operation state %q was rejected", state)
		}
	}
	for _, state := range []string{"", "succeeded", "DROP TABLE devices"} {
		if validDeviceOperationState(state) {
			t.Errorf("unknown operation state %q was accepted", state)
		}
	}
}
