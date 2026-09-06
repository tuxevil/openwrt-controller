package handlers

import (
	"testing"

	"openwrt-controller/internal/services"
)

func TestParseUciShowPreservesInlineLists(t *testing.T) {
	sections := parseUciShow(`dhcp.@dnsmasq[0]=dnsmasq
dhcp.@dnsmasq[0].server='9.9.9.9' '1.1.1.1'
`, "dhcp")
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	servers, ok := sections[0].Options["server"].([]string)
	if !ok || len(servers) != 2 || servers[0] != "9.9.9.9" || servers[1] != "1.1.1.1" {
		t.Fatalf("got %#v, want two DNS servers", sections[0].Options["server"])
	}
}

func TestUciCommandMatchesObservedIsActionAndSectionAware(t *testing.T) {
	sections := parseUciShow(`dhcp.@dnsmasq[0]=dnsmasq
dhcp.@dnsmasq[0].server='9.9.9.9' '1.1.1.1'
dhcp.@host[0]=host
dhcp.@host[0].name='uapmesh'
dhcp.@host[0].mac='78:8A:20:2C:C1:12'
dhcp.@host[0].ip='192.0.2.10'
wireless.radio0=wifi-device
wireless.radio0.ssid='other'
wireless.radio1=wifi-device
wireless.radio1.ssid='desired'
`, "dhcp")

	if !uciCommandMatchesObserved(services.UciCommand{
		Action: "add_list", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: "9.9.9.9",
	}, sections) {
		t.Fatal("inline list value should match an add_list command")
	}
	if !uciCommandMatchesObserved(services.UciCommand{
		Action: "del_list", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: "8.8.8.8",
	}, sections) {
		t.Fatal("an absent list value should match a del_list command")
	}
	if !uciCommandMatchesObserved(services.UciCommand{
		Action: "ensure_host", Config: "dhcp", Section: "uapmesh", Option: "78:8A:20:2C:C1:12", Value: "192.0.2.10",
	}, sections) {
		t.Fatal("existing DHCP host should match ensure_host")
	}
	multiMACSections := parseUciShow(`dhcp.@host[0]=host
dhcp.@host[0].name='uapmesh'
dhcp.@host[0].mac='78:8A:20:2C:C1:12' 'A0:63:91:72:11:30'
dhcp.@host[0].ip='192.0.2.10'
`, "dhcp")
	if !uciCommandMatchesObserved(services.UciCommand{
		Action: "ensure_host", Config: "dhcp", Section: "uapmesh", Option: "78:8A:20:2C:C1:12' 'A0:63:91:72:11:30", Value: "192.0.2.10",
	}, multiMACSections) {
		t.Fatal("existing multi-MAC DHCP host should match ensure_host")
	}
	if !uciCommandMatchesObserved(services.UciCommand{
		Action: "delete", Config: "dhcp", Section: "@dnsmasq[0]", Option: "missing_option",
	}, sections) {
		t.Fatal("missing delete target should be considered satisfied")
	}

	wirelessSections := parseUciShow(`wireless.radio0=wifi-device
wireless.radio0.ssid='other'
wireless.radio1=wifi-device
wireless.radio1.ssid='desired'
`, "wireless")
	if uciCommandMatchesObserved(services.UciCommand{
		Action: "set", Config: "wireless", Section: "radio0", Option: "ssid", Value: "desired",
	}, wirelessSections) {
		t.Fatal("a value from another section must not satisfy the command")
	}

	dropbearSections := parseUciShow(`dropbear.main=dropbear
dropbear.main.Port='22'
dropbear.main.PasswordAuth='off'
`, "dropbear")
	if !uciCommandMatchesObserved(services.UciCommand{
		Action: "set", Config: "dropbear", Section: "@dropbear[0]", Option: "PasswordAuth", Value: "off",
	}, dropbearSections) {
		t.Fatal("an anonymous section reference should match a named section of the same type")
	}

	listSections := parseUciShow(`network.lan=interface
network.lan.dhcp_option='6,192.0.2.1'
network.lan.dhcp_option='3,192.0.2.1'
`, "network")
	if uciCommandMatchesObserved(services.UciCommand{
		Action: "set", Config: "network", Section: "lan", Option: "dhcp_option", Value: "3,192.0.2.1",
	}, listSections) {
		t.Fatal("set must not treat one item of a multi-value option as the whole value")
	}
}

func TestUciCommandPlanTreatsListResetAsFinalState(t *testing.T) {
	commands := []services.UciCommand{
		{Action: "delete", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server"},
		{Action: "add_list", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: "9.9.9.9"},
		{Action: "add_list", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: "1.1.1.1"},
	}
	exact := parseUciShow(`dhcp.@dnsmasq[0]=dnsmasq
dhcp.@dnsmasq[0].server='9.9.9.9' '1.1.1.1'
`, "dhcp")
	if !uciCommandMatchesObservedInPlan(0, commands[0], commands, exact) {
		t.Fatal("delete followed by add_list should match the final observed list")
	}

	withExtra := parseUciShow(`dhcp.@dnsmasq[0]=dnsmasq
dhcp.@dnsmasq[0].server='9.9.9.9' '1.1.1.1' '8.8.8.8'
`, "dhcp")
	if uciCommandMatchesObservedInPlan(0, commands[0], commands, withExtra) {
		t.Fatal("an extra list item should keep the reset plan in drift")
	}
}
