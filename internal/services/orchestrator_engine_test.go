package services

import "testing"

func TestResolveResourcesUsesReportedCapabilities(t *testing.T) {
	radioSection, radioDevice, sqmInterface := resolveResources(DeviceCapabilities{
		Radios:                 []string{"radio1"},
		Interfaces:             []string{"br-wan", "lan"},
		WirelessDeviceSections: []string{"radio1"},
		WirelessIfaceSections:  []string{"default_radio1"},
		LogicalNetworks:        map[string]string{"lan": "home"},
		SQMCandidates:          []string{"br-wan"},
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

func TestLogicalNetworkSectionUsesExplicitMapping(t *testing.T) {
	capabilities := DeviceCapabilities{LogicalNetworks: map[string]string{"lan": "home"}}
	if got := logicalNetworkSection(capabilities, "lan"); got != "home" {
		t.Fatalf("got %q, want home", got)
	}
	if got := logicalNetworkSection(DeviceCapabilities{}, "lan"); got != "lan" {
		t.Fatalf("got %q, want legacy lan", got)
	}
}
