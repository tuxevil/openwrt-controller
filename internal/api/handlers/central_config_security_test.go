package handlers

import "testing"

func TestBuildCentralConfigCommandRejectsShellSyntax(t *testing.T) {
	if got, err := buildCentralConfigCommand("wireless", "wireless.radio0.channel"); err != nil || got != "uci show wireless.radio0.channel 2>&1" {
		t.Fatalf("valid UCI path = %q, %v", got, err)
	}

	for _, tc := range []struct {
		config string
		path   string
	}{
		{config: "wireless; reboot", path: ""},
		{config: "wireless", path: "wireless.radio0; reboot"},
		{config: "wireless", path: "network.lan"},
	} {
		if _, err := buildCentralConfigCommand(tc.config, tc.path); err == nil {
			t.Errorf("accepted unsafe central-config input: config=%q path=%q", tc.config, tc.path)
		}
	}
}
