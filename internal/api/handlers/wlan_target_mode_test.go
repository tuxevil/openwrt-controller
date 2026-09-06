package handlers

import "testing"

func TestNormalizeWLANTargetMode(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "", want: "all"},
		{input: "all", want: "all"},
		{input: " ALL ", want: "all"},
		{input: "custom", want: "custom"},
		{input: "CuStOm", want: "custom"},
	} {
		got, err := normalizeWLANTargetMode(tt.input)
		if err != nil || got != tt.want {
			t.Fatalf("normalizeWLANTargetMode(%q) = %q, %v; want %q", tt.input, got, err, tt.want)
		}
	}

	if _, err := normalizeWLANTargetMode("site-wide"); err == nil {
		t.Fatal("unknown WLAN target mode should be rejected")
	}
}
