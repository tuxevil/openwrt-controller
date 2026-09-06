package services

import "testing"

func TestWLANAppliesToDeviceUsesTargetMode(t *testing.T) {
	tests := []struct {
		name       string
		global     bool
		targetMode string
		assigned   bool
		want       bool
	}{
		{name: "global all", global: true, targetMode: "all", want: true},
		{name: "global custom only when assigned", global: true, targetMode: "custom", assigned: true, want: true},
		{name: "global custom not assigned", global: true, targetMode: "custom", want: false},
		{name: "disabled global only assigned custom", targetMode: "custom", assigned: true, want: true},
		{name: "disabled global excludes all", targetMode: "all", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WLANAppliesToDevice(tt.global, tt.targetMode, tt.assigned); got != tt.want {
				t.Fatalf("got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestResolveCanonicalWLANUsesWLANRowAsAuthority(t *testing.T) {
	resolution := ResolveCanonicalWLAN(
		"tuxcave2", "psk2", "site-secret",
		"tuxcave2", "sae-mixed", "wlan-secret",
	)

	if resolution.Security != "sae-mixed" || resolution.Password != "wlan-secret" {
		t.Fatalf("canonical WLAN row was not preserved: %#v", resolution)
	}
	if len(resolution.Conflicts) != 2 || resolution.Conflicts[0] != "global_encryption" || resolution.Conflicts[1] != "global_wpa_key" {
		t.Fatalf("got conflicts %v, want encryption and key", resolution.Conflicts)
	}
}

func TestResolveCanonicalWLANIgnoresUnrelatedLegacyFields(t *testing.T) {
	resolution := ResolveCanonicalWLAN(
		"legacy-ssid", "psk2", "site-secret",
		"tuxcave2", "psk2", "wlan-secret",
	)

	if len(resolution.Conflicts) != 0 {
		t.Fatalf("unrelated legacy fields should not conflict: %v", resolution.Conflicts)
	}
}
