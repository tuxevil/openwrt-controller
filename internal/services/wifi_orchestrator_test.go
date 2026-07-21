package services

import (
	"strings"
	"testing"
)

func TestBuildWLANConfigCommandQuotesUserValues(t *testing.T) {
	cmd := buildWLANConfigCommand(
		"guest'; reboot #",
		"wpa2",
		"secret'; touch /tmp/pwned #",
		true,
		true,
		true,
		"1",
		"10.0.0.1'; id #",
		"shared'; id #",
		"1",
	)

	for _, field := range []string{"ssid", "key", "auth_server", "auth_secret"} {
		if strings.Contains(cmd, field+"='guest';") || strings.Contains(cmd, field+"='secret';") {
			t.Fatalf("%s is interpolated as an executable shell fragment: %s", field, cmd)
		}
	}

	for _, want := range []string{
		"ssid='guest'\\''; reboot #'",
		"key='secret'\\''; touch /tmp/pwned #'",
		"auth_server='10.0.0.1'\\''; id #'",
		"auth_secret='shared'\\''; id #'",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command missing shell-quoted value %q:\n%s", want, cmd)
		}
	}
}
