package handlers

import "testing"

func TestSanitiseFirmwareFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Happy path.
		{"simple", "openwrt-ar71xx-generic-squashfs-sysupgrade.bin", "openwrt-ar71xx-generic-squashfs-sysupgrade.bin"},
		// Strip directory traversal.
		{"traversal-relative", "../../etc/passwd", "passwd"},
		{"traversal-windows", `..\..\windows\system32\cmd.exe`, "cmd.exe"},
		{"abs-path", "/etc/shadow", "shadow"},
		// Reject control characters.
		{"newline", "foo\nbar", ""},
		{"nul", "foo\x00bar", ""},
		{"tab", "foo\tbar", ""}, // tab < 0x20
		// Reject empty / pathological.
		{"empty", "", ""},
		{"dot", ".", ""},
		{"slash", "/", ""},
		// Whitespace trimmed.
		{"padded", "   firmware.bin   ", "firmware.bin"},
		// Length cap.
		{"very-long", "a" + repeat("b", 300) + ".bin", ("a" + repeat("b", 254))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitiseFirmwareFilename(c.in)
			if got != c.want {
				t.Errorf("sanitiseFirmwareFilename(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
