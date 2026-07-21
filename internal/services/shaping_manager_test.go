package services

import (
	"strings"
	"testing"
)

func TestBuildSniperShapingCommandQuotesMAC(t *testing.T) {
	cmd := buildSniperShapingCommand("aa:bb'; reboot #", 64)

	if strings.Contains(cmd, `MAC="aa:bb'; reboot #"`) {
		t.Fatalf("MAC is interpolated into a shell command without quoting: %s", cmd)
	}
	if !strings.Contains(cmd, `MAC='aa:bb'\''; reboot #'`) {
		t.Fatalf("MAC is not shell-quoted: %s", cmd)
	}
}

func TestValidMACAddress(t *testing.T) {
	if !validMACAddress("aa:bb:cc:dd:ee:ff") {
		t.Fatal("expected a valid MAC address")
	}
	for _, invalid := range []string{"aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:ff;reboot", "not-a-mac"} {
		if validMACAddress(invalid) {
			t.Errorf("accepted invalid MAC address %q", invalid)
		}
	}
}
