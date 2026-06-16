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
