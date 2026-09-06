package handlers

import "testing"

func TestIsAllowedUciConfig(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"network", true},
		{"wireless", true},
		{"dhcp", true},
		{"firewall", true},
		{"system", true},
		{"dropbear", true},
		{"uhttpd", true},
		{"openvpn", true},
		{"usteer", true},
		{"sqm", true},
		// Disallowed — these were the historical attack surface.
		{"public", false},
		{"", false},
		{"network; reboot", false},
		{"NETWORK", false},  // case-sensitive
		{"network ", false}, // trailing space
		{"../network", false},
	}
	for _, c := range cases {
		if got := isAllowedUciConfig(c.in); got != c.want {
			t.Errorf("isAllowedUciConfig(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRequiresDeviceOperationForExecutorNamespaces(t *testing.T) {
	for _, config := range []string{"network", "wireless", "dhcp", "firewall", "dropbear", "system", "sqm"} {
		if !requiresDeviceOperation(config) {
			t.Fatalf("%s must use the device operation path", config)
		}
	}
	if requiresDeviceOperation("uhttpd") || requiresDeviceOperation("openvpn") || requiresDeviceOperation("usteer") {
		t.Fatal("unsupported executor namespaces should remain outside this boundary")
	}
}
