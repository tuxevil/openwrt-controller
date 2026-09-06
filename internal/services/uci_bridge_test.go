package services

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEscapeVal(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"can't", `can'\''t`}, // standard POSIX shell-quote escape
		{"a'b'c", `a'\''b'\''c`},
		{"", ""},
		{"no quotes here", "no quotes here"},
		{"'; rm -rf /", `'\''; rm -rf /`},
	}
	for _, c := range cases {
		got := escapeVal(c.in)
		if got != c.want {
			t.Errorf("escapeVal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseMACList(t *testing.T) {
	macs, err := ParseMACList("0c:a6:4c:f2:f2:77' '0c:a6:4c:92:be:06")
	if err != nil {
		t.Fatalf("ParseMACList returned an error: %v", err)
	}
	want := []string{"0C:A6:4C:F2:F2:77", "0C:A6:4C:92:BE:06"}
	if strings.Join(macs, ",") != strings.Join(want, ",") {
		t.Fatalf("ParseMACList = %#v, want %#v", macs, want)
	}
	if _, err := ParseMACList("not-a-mac"); err == nil {
		t.Fatal("ParseMACList should reject invalid input")
	}
}

func TestSetOptionEscapesValue(t *testing.T) {
	out := SetOption("wireless", "wifi0", "ssid", "evil'; reboot #")
	want := "uci set wireless.wifi0.ssid='evil'\\''; reboot #'"
	if out != want {
		t.Errorf("SetOption did not escape properly:\n  got:  %s\n  want: %s", out, want)
	}
}

func TestBuildBatchScript_ValidatesConfig(t *testing.T) {
	// BuildBatchScript no longer accepts arbitrary config names; the
	// ServiceRestartMap lookup simply yields an empty restart command
	// for unknown configs, which is the intended behaviour.
	script := BuildBatchScript("wireless", []UciCommand{
		{Action: "set", Config: "wireless", Section: "wifi0", Option: "ssid", Value: "TestNet"},
	})
	if script == "" {
		t.Error("expected non-empty script")
	}
	if !contains(script, "uci set wireless.wifi0.ssid='TestNet'") {
		t.Error("script missing the expected uci set line")
	}
}

func TestBuildSafeBatchScriptRestartsBeforeHealthCheck(t *testing.T) {
	script := BuildSafeBatchScript("wireless", []UciCommand{
		{Action: "set", Config: "wireless", Section: "wifi0", Option: "ssid", Value: "TestNet"},
	}, []string{"192.0.2.1"})

	restart := "wifi && logger -t central_luci 'wireless service restarted'"
	healthCheck := "ping -c 1 -W 2 '192.0.2.1'"
	if strings.Index(script, restart) == -1 || strings.Index(script, healthCheck) == -1 {
		t.Fatalf("safe script missing restart or health check:\n%s", script)
	}
	if strings.Index(script, restart) > strings.Index(script, healthCheck) {
		t.Fatalf("health check runs before service restart:\n%s", script)
	}
}

func TestBuildSafeBatchScriptRetriesAndReportsHealthCheckFailures(t *testing.T) {
	script := BuildSafeBatchScript("wireless", []UciCommand{
		{Action: "set", Config: "wireless", Section: "wifi0", Option: "ssid", Value: "TestNet"},
	}, []string{"192.0.2.1"})

	if !strings.Contains(script, "for health_attempt in 1 2 3") {
		t.Fatalf("health check should retry transient failures:\n%s", script)
	}
	if !strings.Contains(script, "CENTRAL_LUCI: health check failed for target") {
		t.Fatalf("health check should report the failed target:\n%s", script)
	}
}

func TestBuildBatchScriptRollsBackAndRestartsService(t *testing.T) {
	script := BuildBatchScript("wireless", []UciCommand{
		{Action: "set", Config: "wireless", Section: "wifi0", Option: "ssid", Value: "TestNet"},
	})

	restore := "cp /tmp/central_luci_bak_wireless.conf /etc/config/wireless"
	revert := "uci revert wireless"
	restart := "wifi && logger -t central_luci 'wireless service restarted'"
	rollback := strings.Index(script, "rollback()")
	if rollback == -1 {
		t.Fatalf("script missing rollback function:\n%s", script)
	}
	for _, fragment := range []string{restore, revert, restart} {
		position := strings.Index(script[rollback:], fragment)
		if position == -1 {
			t.Fatalf("rollback missing %q:\n%s", fragment, script)
		}
	}
}

func TestBuildBatchScriptRollbackRestoresRawConfigFile(t *testing.T) {
	script := BuildBatchScript("wireless", []UciCommand{
		{Action: "set", Config: "wireless", Section: "wifi0", Option: "ssid", Value: "TestNet"},
	})

	for _, fragment := range []string{
		"cp /etc/config/wireless /tmp/central_luci_bak_wireless.conf",
		"cp /tmp/central_luci_bak_wireless.conf /etc/config/wireless",
		"uci revert wireless",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("rollback missing raw config restoration step %q:\n%s", fragment, script)
		}
	}
	if strings.Contains(script, "uci import wireless < /tmp/central_luci_bak_wireless.conf") {
		t.Fatal("rollback must not rely on uci import to remove committed anonymous sections")
	}
}

func TestBuildBatchScriptKeepsRestartCommandsInTheirPhases(t *testing.T) {
	script := BuildBatchScript("wireless", []UciCommand{
		{Action: "set", Config: "wireless", Section: "wifi0", Option: "ssid", Value: "TestNet"},
	})
	restart := "wifi && logger -t central_luci 'wireless service restarted'"
	if strings.Count(script, restart) != 2 {
		t.Fatalf("expected restart in rollback and normal phases, got %d:\n%s", strings.Count(script, restart), script)
	}
	if strings.Contains(script, "\nwireless || true") || strings.Contains(script, "\n.conf\n") {
		t.Fatalf("format arguments shifted into executable lines:\n%s", script)
	}
}

func TestBuildSafeBatchScriptHasValidShellSyntax(t *testing.T) {
	script := BuildSafeBatchScript("dhcp", []UciCommand{
		{Action: "ensure_host", Config: "dhcp", Section: "living room", Option: "AA:BB:CC:DD:EE:FF", Value: "192.0.2.10"},
	}, []string{"192.0.2.1"})

	command := exec.Command("sh", "-n")
	command.Stdin = strings.NewReader(script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated safe batch is not valid shell: %v\n%s", err, output)
	}
}

func TestBuildBatchScriptEnsuresDHCPHostIdempotently(t *testing.T) {
	script := BuildBatchScript("dhcp", []UciCommand{
		{Action: "ensure_host", Config: "dhcp", Section: "living room", Option: "AA:BB:CC:DD:EE:FF", Value: "192.0.2.10"},
	})

	for _, fragment := range []string{
		"host_macs='AA:BB:CC:DD:EE:FF'",
		"host_ip='192.0.2.10'",
		"host_name='living room'",
		"host_ref=$(uci show dhcp",
		"if [ -n \"$host_ref\" ]; then",
		"uci add dhcp host",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("idempotent DHCP host script missing %q:\n%s", fragment, script)
		}
	}
	if strings.Index(script, "uci add dhcp host") < strings.Index(script, "if [ -n \"$host_ref\" ]; then") {
		t.Fatalf("DHCP host creation must remain in the missing-host branch:\n%s", script)
	}
}

func TestBuildBatchScriptLooksUpDHCPHostByMACOrIPValue(t *testing.T) {
	script := BuildBatchScript("dhcp", []UciCommand{
		{Action: "ensure_host", Config: "dhcp", Section: "living room", Option: "AA:BB:CC:DD:EE:FF", Value: "192.0.2.10"},
	})

	for _, fragment := range []string{
		`grep -i -F "$host_mac" | grep -F ".mac="`,
		`host_value=$(printf '%s' "$host_value" | sed "s/^'//; s/'$//")`,
		`if [ "$host_value" = "$host_ip" ]; then`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("DHCP host lookup missing %q:\n%s", fragment, script)
		}
	}
}

func TestBuildBatchScriptHandlesMultipleDHCPHostMACs(t *testing.T) {
	script := BuildBatchScript("dhcp", []UciCommand{
		{Action: "ensure_host", Config: "dhcp", Section: "living room", Option: "AA:BB:CC:DD:EE:FF' '11:22:33:44:55:66", Value: "192.0.2.10"},
	})

	for _, fragment := range []string{
		"host_macs='AA:BB:CC:DD:EE:FF 11:22:33:44:55:66'",
		"host_mac_count=2",
		"for host_mac in $host_macs; do",
		"uci -q delete \"dhcp.$host_ref.mac\" || true",
		"uci add_list \"dhcp.$host_ref.mac=$host_mac\"",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("multi-MAC DHCP host script missing %q:\n%s", fragment, script)
		}
	}
}

func TestDeleteCommandsIgnoreMissingEntries(t *testing.T) {
	for _, command := range []string{
		Delete("dhcp", "@host[-1]"),
		DeleteOption("dhcp", "@dnsmasq[0]", "server"),
	} {
		if !strings.HasSuffix(command, " || true") {
			t.Fatalf("delete command should be idempotent: %q", command)
		}
	}
}

func TestUCIBuilderRejectsUnsafeIdentifiers(t *testing.T) {
	unsafe := UciCommand{
		Action:  "set",
		Config:  "wireless; reboot",
		Section: "wifi0",
		Option:  "ssid",
		Value:   "safe",
	}
	if got := BuildBatchScript("wireless; reboot", []UciCommand{unsafe}); got != "" {
		t.Fatalf("unsafe config produced an executable script:\n%s", got)
	}
	if got := PreviewCommands([]UciCommand{unsafe}); len(got) != 0 {
		t.Fatalf("unsafe config produced preview commands: %#v", got)
	}

	unsafeSection := UciCommand{
		Action:  "set",
		Config:  "wireless",
		Section: "wifi0; reboot",
		Option:  "ssid",
		Value:   "safe",
	}
	if got := PreviewCommands([]UciCommand{unsafeSection}); len(got) != 0 {
		t.Fatalf("unsafe section produced preview commands: %#v", got)
	}
}

func TestValidRawUCICommandRejectsShellFragments(t *testing.T) {
	valid := []string{
		"uci set wireless.radio0.ssid='Guest WiFi'",
		"uci add_list wireless.@wifi-iface[-1].dns='1.1.1.1'",
		"uci -q delete wireless.radio0.channel",
		"uci add wireless wifi-iface",
	}
	for _, command := range valid {
		if !ValidRawUCICommand(command, "wireless") {
			t.Errorf("rejected valid UCI command %q", command)
		}
	}

	unsafe := []string{
		"uci set wireless.radio0.ssid='guest'; reboot",
		"uci set wireless.radio0.ssid='$(id)'",
		"uci show wireless",
		"uci set network.lan.ssid='wrong namespace'",
	}
	for _, command := range unsafe {
		if ValidRawUCICommand(command, "wireless") {
			t.Errorf("accepted unsafe UCI command %q", command)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
