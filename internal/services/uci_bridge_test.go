package services

import "testing"

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
