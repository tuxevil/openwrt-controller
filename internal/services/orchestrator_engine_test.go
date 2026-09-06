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
	if radioSection != "cfg_radio0_0" || radioDevice != "radio0" || sqmInterface != "eth1" {
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

func TestRenderSiteConfigDoesNotEmitUnsupportedDPIOption(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{DPIEnabled: true, GlobalSSID: "test"}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "firewall" && command.Option == "dpi_enabled" {
			t.Fatal("renderer emitted unsupported firewall dpi_enabled option")
		}
	}
}

func TestRenderGatewayRemovesLegacyDPIOptionWhenDisabled(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{DPIEnabled: false}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "firewall" && command.Section == "@defaults[0]" && command.Option == "dpi_enabled" {
			if command.Action != "delete" {
				t.Fatalf("legacy DPI option must be deleted, got action %q", command.Action)
			}
			return
		}
	}
	t.Fatal("renderer did not remove the legacy firewall dpi_enabled option")
}

func TestRenderGatewaySkipsSQMWhenDisabled(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{SQMCakeEnabled: false}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "sqm" {
			t.Fatalf("disabled SQM must not mutate optional sqm config: %#v", command)
		}
	}
}

func TestRenderGatewayEnsuresDHCPReservationsWithoutUnconditionalAdds(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{
		DHCPReservations: []byte(`[{"name":"test-host","mac":"AA:BB:CC:DD:EE:FF","ip":"192.0.2.10"}]`),
	}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	found := false
	for _, command := range results[0].Commands {
		if command.Config != "dhcp" || command.Option != "AA:BB:CC:DD:EE:FF" {
			continue
		}
		if command.Action != "ensure_host" || command.Section != "test-host" || command.Value != "192.0.2.10" {
			t.Fatalf("unexpected DHCP reservation command: %#v", command)
		}
		found = true
	}
	if !found {
		t.Fatal("renderer did not emit an idempotent DHCP reservation command")
	}
}

func TestRenderGatewayUsesNamedSQMSectionWhenEnabled(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{SQMCakeEnabled: true}, []DeviceRoleInfo{{
		DeviceID: "gateway",
		Role:     "Gateway",
		Capabilities: DeviceCapabilities{
			SQMCandidates: []string{"eth1"},
		},
	}})
	found := false
	for _, command := range results[0].Commands {
		if command.Config != "sqm" {
			continue
		}
		if command.Section == "@queue[0]" || command.Section == "@sqm[0]" {
			t.Fatalf("SQM must target a named section, got %#v", command)
		}
		if command.Section == "eth1" {
			found = true
		}
	}
	if !found {
		t.Fatal("renderer did not target the reported SQM section")
	}
}
