package database

import (
	"encoding/json"
	"testing"
)

func TestParseDeviceOperationEnvelopeSeparatesAttemptAndContentIdentity(t *testing.T) {
	valid := json.RawMessage(`{"operation_id":"operation-1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)
	got, planHash, err := parseDeviceOperationEnvelope(valid)
	if err != nil || got != "operation-1" {
		t.Fatalf("valid envelope = %q, error %v", got, err)
	}
	if planHash != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("valid plan hash = %q", planHash)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`not-json`),
		json.RawMessage(`{"operation_id":"operation-1"}`),
		json.RawMessage(`{"operation_id":"operation-1","plan_hash":"different"}`),
		json.RawMessage(`{"operation_id":"operation;1","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
	} {
		if _, _, err := parseDeviceOperationEnvelope(raw); err == nil {
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
