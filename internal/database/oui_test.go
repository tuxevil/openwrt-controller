package database

import (
	"strings"
	"testing"
)

func TestLookupMACVendorOfflineIEEEAndLocallyAdministered(t *testing.T) {
	// Real IEEE OUI entries (FireTV, ESP32, Pixel, Proxmox, Raspberry Pi)
	cases := []struct {
		input      string
		wantSubstr string
		wantOUI    string
	}{
		{"90:39:5f:2f:c9:7f", "Amazon", "90:39:5F"},
		{"F0-9E-9E-21-C3-E4", "Espressif", "F0:9E:9E"},
		{"3c:28:6d:0f:85:db", "Google", "3C:28:6D"},
		{"bc:24:11:0b:ea:8f", "Proxmox", "BC:24:11"},
		{"E45F01", "Raspberry Pi", "E4:5F:01"},
	}
	for _, tc := range cases {
		got := LookupMACVendor(tc.input)
		if got.Source != "ieee_oui_offline" {
			t.Fatalf("LookupMACVendor(%q).Source = %q, want ieee_oui_offline", tc.input, got.Source)
		}
		if got.OUI != tc.wantOUI {
			t.Fatalf("LookupMACVendor(%q).OUI = %q, want %q", tc.input, got.OUI, tc.wantOUI)
		}
		if got.LocallyAdministered {
			t.Fatalf("LookupMACVendor(%q) unexpectedly marked LocallyAdministered", tc.input)
		}
		if !strings.Contains(got.Vendor, tc.wantSubstr) {
			t.Fatalf("LookupMACVendor(%q).Vendor = %q, want substring %q", tc.input, got.Vendor, tc.wantSubstr)
		}
	}

	// Locally administered / randomized Wi-Fi MACs (bit 1 of first byte = 1)
	for _, randomMAC := range []string{"3E:FB:EF:76:06:79", "3E:FB:EF", "96:d0:34:07:79:ca", "02:31:34:40:00:01"} {
		got := LookupMACVendor(randomMAC)
		if !got.LocallyAdministered || got.Source != "locally_administered" {
			t.Fatalf("LookupMACVendor(%q) = %+v, want locally_administered", randomMAC, got)
		}
		if got.Vendor != "" {
			t.Fatalf("LookupMACVendor(%q).Vendor = %q, want empty for locally administered MAC", randomMAC, got.Vendor)
		}
	}

	// Invalid input
	if got := LookupMACVendor("not-a-mac"); got.Source != "invalid" {
		t.Fatalf("LookupMACVendor(invalid) = %+v, want source=invalid", got)
	}
}
