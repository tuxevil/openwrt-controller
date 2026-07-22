package main

import "testing"

func TestQuoteIdentifier(t *testing.T) {
	tests := map[string]string{
		"public":           `"public"`,
		"tenant_dragontec": `"tenant_dragontec"`,
		`a"b`:              `"a""b"`,
	}

	for input, want := range tests {
		if got := quoteIdentifier(input); got != want {
			t.Errorf("quoteIdentifier(%q) = %q, want %q", input, got, want)
		}
	}
}
