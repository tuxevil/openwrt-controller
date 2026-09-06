package services

import "testing"

func TestValidateEdgeUCIIdentifiers(t *testing.T) {
	if err := ValidateNetworkInterfaces([]NetworkInterface{{Name: "lan; reboot", Proto: "dhcp"}}); err == nil {
		t.Fatal("accepted a shell fragment as a network interface name")
	}
	if err := ValidateDHCPInterfaces([]DHCPInterface{{Interface: "lan$(id)"}}); err == nil {
		t.Fatal("accepted a shell fragment as a DHCP interface name")
	}
	if err := ValidateNetworkInterfaces([]NetworkInterface{{Name: "lan", Proto: "dhcp"}}); err != nil {
		t.Fatalf("rejected valid network interface: %v", err)
	}
}

func TestBuildNetworkCommandsUsesTypedValues(t *testing.T) {
	commands := BuildNetworkCommands([]NetworkInterface{{
		Name:    "lan",
		Proto:   "static",
		Device:  "br-lan",
		IPAddr:  "192.0.2.1",
		Netmask: "255.255.255.0",
	}})
	if len(commands) != 5 {
		t.Fatalf("got %d commands, want 5: %#v", len(commands), commands)
	}
	if commands[0] != (UciCommand{Action: "set", Config: "network", Section: "lan", Value: "interface"}) {
		t.Fatalf("unexpected section command: %#v", commands[0])
	}
	if commands[2].Option != "device" || commands[2].Value != "br-lan" {
		t.Fatalf("unexpected device command: %#v", commands[2])
	}
}

func TestBuildDHCPAndFirewallCommandsPreserveAnonymousOrder(t *testing.T) {
	dhcp := BuildDHCPCommands([]DHCPInterface{{
		Interface:   "lan",
		Enabled:     true,
		Start:       100,
		Limit:       50,
		UpstreamDNS: []string{"9.9.9.9"},
		StaticLeases: []StaticLease{{
			Name: "lab",
			MAC:  "AA:BB:CC:DD:EE:FF",
			IP:   "192.0.2.10",
		}},
	}})
	if len(dhcp) != 8 || dhcp[7].Action != "ensure_host" || dhcp[7].Section != "lab" {
		t.Fatalf("unexpected DHCP command sequence: %#v", dhcp)
	}

	firewall := BuildFirewallCommands([]PortForwardRule{{Name: "web", Proto: "tcp", SrcPort: 443, DestIP: "192.0.2.10", DestPort: 8443, Enabled: true}})
	if len(firewall) != 11 || firewall[0].Action != "delete_all" || firewall[1].Action != "add" || firewall[2].Section != "@redirect[-1]" || firewall[10].Value != "1" {
		t.Fatalf("unexpected firewall command sequence: %#v", firewall)
	}
	if empty := BuildFirewallCommands(nil); len(empty) != 1 || empty[0].Action != "delete_all" {
		t.Fatalf("empty firewall policy must clear redirects: %#v", empty)
	}
}

func TestValidateDHCPInterfacesValidatesStaticLeases(t *testing.T) {
	if err := ValidateDHCPInterfaces([]DHCPInterface{{
		Interface: "lan",
		StaticLeases: []StaticLease{{
			Name: "lab",
			MAC:  "AA:BB:CC:DD:EE:FF",
			IP:   "192.0.2.10",
		}},
	}}); err != nil {
		t.Fatalf("rejected valid static lease: %v", err)
	}
	if err := ValidateDHCPInterfaces([]DHCPInterface{{
		Interface: "lan",
		StaticLeases: []StaticLease{{
			Name: "lab",
			MAC:  "AA:BB:CC:DD:EE:FF;touch",
			IP:   "192.0.2.10",
		}},
	}}); err == nil {
		t.Fatal("accepted a shell fragment in a static lease MAC")
	}
}

func TestValidatePortForwardRulesRejectsInvalidRules(t *testing.T) {
	valid := PortForwardRule{Name: "web", Proto: "tcp", SrcPort: 443, DestIP: "192.0.2.10", DestPort: 8443}
	if err := ValidatePortForwardRules([]PortForwardRule{valid}); err != nil {
		t.Fatalf("rejected valid port forward: %v", err)
	}
	cases := []PortForwardRule{
		{Name: "web\nreboot", Proto: "tcp", SrcPort: 443, DestIP: "192.0.2.10", DestPort: 8443},
		{Name: "web", Proto: "icmp", SrcPort: 443, DestIP: "192.0.2.10", DestPort: 8443},
		{Name: "web", Proto: "tcp", SrcPort: 443, DestIP: "not-an-ip", DestPort: 8443},
		{Name: "web", Proto: "tcp", SrcPort: 443, DestIP: "192.0.2.10", DestPort: 0},
	}
	for _, rule := range cases {
		if err := ValidatePortForwardRules([]PortForwardRule{rule}); err == nil {
			t.Fatalf("invalid port forward was accepted: %#v", rule)
		}
	}
}
