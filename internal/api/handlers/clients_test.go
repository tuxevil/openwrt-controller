package handlers

import (
	"testing"
	"time"
)

func TestParseTrustedClientMACCanonicalizesSixOctetMAC(t *testing.T) {
	got, err := parseTrustedClientMAC("aa-bb-cc-dd-ee-ff")
	if err != nil {
		t.Fatalf("parseTrustedClientMAC returned error: %v", err)
	}
	if got != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("canonical MAC = %q", got)
	}
}

func TestParseTrustedClientMACRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"", "not-a-mac", "AA:BB:CC:DD:EE:FF:00"} {
		if _, err := parseTrustedClientMAC(raw); err == nil {
			t.Errorf("parseTrustedClientMAC(%q) accepted invalid input", raw)
		}
	}
}

func TestParseTrustedClientExpiryRejectsPastTimestamp(t *testing.T) {
	_, err := parseTrustedClientExpiry(time.Now().Add(-time.Minute).Format(time.RFC3339))
	if err == nil {
		t.Fatal("past trust expiry should be rejected")
	}
}
