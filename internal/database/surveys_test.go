package database

import (
	"strings"
	"testing"
)

func TestHashToken_Deterministic(t *testing.T) {
	a := HashToken("hello-world")
	b := HashToken("hello-world")
	if a != b {
		t.Errorf("HashToken not deterministic: %q vs %q", a, b)
	}
}

func TestHashToken_64CharHex(t *testing.T) {
	h := HashToken("anything")
	if len(h) != 64 {
		t.Errorf("HashToken length = %d, want 64", len(h))
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("HashToken contains non-hex char %q in %q", c, h)
			break
		}
	}
}

func TestHashToken_DifferentInputsDifferentHashes(t *testing.T) {
	a := HashToken("token-a")
	b := HashToken("token-b")
	if a == b {
		t.Errorf("HashToken collision: %q == %q", a, b)
	}
}

func TestGenerateSurveyToken_FormatAndUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tok, h, err := GenerateSurveyToken()
		if err != nil {
			t.Fatalf("GenerateSurveyToken error: %v", err)
		}
		if len(tok) != 64 {
			t.Errorf("token length = %d, want 64", len(tok))
		}
		if strings.ToLower(tok) != tok {
			t.Errorf("token not lowercase hex: %q", tok)
		}
		if HashToken(tok) != h {
			t.Errorf("HashToken(tok) != h; verify the contract")
		}
		if seen[tok] {
			t.Errorf("duplicate token generated: %q", tok)
		}
		seen[tok] = true
	}
}

func TestNullString(t *testing.T) {
	if nullString("") != nil {
		t.Errorf("nullString(\"\") should be nil")
	}
	if nullString("x") != "x" {
		t.Errorf("nullString(\"x\") should be \"x\"")
	}
}
