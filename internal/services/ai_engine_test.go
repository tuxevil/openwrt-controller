package services

import "testing"

func TestParseAICompletionAcceptsJSONAndCodeFence(t *testing.T) {
	for _, content := range []string{
		`{"intent":"GET_DEVICE_STATUS","summary":"Checking devices"}`,
		"```json\n{\"intent\":\"GET_DEVICE_STATUS\",\"summary\":\"Checking devices\"}\n```",
	} {
		if _, err := ParseAICompletion(content); err != nil {
			t.Fatalf("ParseAICompletion rejected valid provider output: %v", err)
		}
	}
}

func TestParseAICompletionRejectsNonJSON(t *testing.T) {
	if _, err := ParseAICompletion("not JSON"); err == nil {
		t.Fatal("expected invalid provider output to be rejected")
	}
}
