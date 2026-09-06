package database

import (
	"strings"
	"testing"
	"time"
)

func TestNetworkIdentityFallbackLabelNeverUsesUnknownWhenIdentityExists(t *testing.T) {
	identity := NetworkIdentity{Kind: "node", MAC: "AA:BB:CC:DD:EE:FF", Model: "Netgear WNDR3800CH"}
	if got := identity.DisplayLabel(); got != "Netgear WNDR3800CH" {
		t.Fatalf("DisplayLabel = %q, want model fallback", got)
	}

	identity.Model = ""
	identity.MAC = "AA:BB:CC:DD:EE:FF"
	if got := identity.DisplayLabel(); got != "node-EEFF" {
		t.Fatalf("DisplayLabel = %q, want stable short fallback", got)
	}
}

func TestAnnotateNetworkTextAddsHumanIdentityAndTrust(t *testing.T) {
	directory := map[string]NetworkIdentity{
		"AA:BB:CC:DD:EE:FF": {
			Kind:        "client",
			MAC:         "AA:BB:CC:DD:EE:FF",
			Label:       "operator-laptop",
			IP:          "192.168.1.44",
			UplinkLabel: "wndr3800ch",
			Trusted:     true,
			TrustReason: "operator workstation",
			TrustExpiresAt: func() *time.Time {
				value := time.Now().Add(time.Hour)
				return &value
			}(),
		},
	}

	annotated := AnnotateNetworkText("dropbear: root login from 192.168.1.44 (AA:BB:CC:DD:EE:FF)", directory)
	for _, expected := range []string{"operator-laptop", "AA:BB:CC:DD:EE:FF", "192.168.1.44", "wndr3800ch", "TRUSTED"} {
		if !strings.Contains(annotated, expected) {
			t.Fatalf("annotation missing %q: %s", expected, annotated)
		}
	}
}

func TestNetworkIdentityTrustExpires(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	identity := NetworkIdentity{Trusted: true, TrustExpiresAt: &past}
	if identity.IsTrusted(time.Now()) {
		t.Fatal("expired trust must not remain active")
	}
}
