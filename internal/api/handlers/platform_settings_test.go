package handlers

import (
	"testing"
)

func TestAIEnginePrivateHTTPURLIsAllowed(t *testing.T) {
	for _, raw := range []string{"http://localhost:51200/v1", "http://127.0.0.1:11434/v1"} {
		if got, err := validateAIEngineBaseURL(raw); err != nil || got != raw {
			t.Fatalf("validateAIEngineBaseURL(%q) = %q, %v", raw, got, err)
		}
	}
}

func TestAIEnginePublicHTTPURLIsNotAllowed(t *testing.T) {
	if _, err := validateAIEngineBaseURL("http://203.0.113.10/v1"); err == nil {
		t.Fatal("expected public HTTP provider URL to be rejected")
	}
}
