package services

import "testing"

func TestResolveResourcesUsesReportedCapabilities(t *testing.T) {
	radioSection, radioDevice, sqmInterface := resolveResources(DeviceCapabilities{
		Radios:           []string{"radio1"},
		Interfaces:       []string{"br-wan", "lan"},
		WirelessSections: []string{"default_radio1", "radio1"},
	})
	if radioSection != "default_radio1" || radioDevice != "radio1" || sqmInterface != "br-wan" {
		t.Fatalf("got %q, %q, %q", radioSection, radioDevice, sqmInterface)
	}
}

func TestResolveResourcesPreservesLegacyFallback(t *testing.T) {
	radioSection, radioDevice, sqmInterface := resolveResources(DeviceCapabilities{})
	if radioSection != "default_radio0" || radioDevice != "radio0" || sqmInterface != "eth1" {
		t.Fatalf("got %q, %q, %q", radioSection, radioDevice, sqmInterface)
	}
}
