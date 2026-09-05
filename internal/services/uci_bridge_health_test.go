package services

import "testing"

func TestValidateHealthTargets(t *testing.T) {
	got, err := ValidateHealthTargets([]string{"1.1.1.1", "router.local", "1.1.1.1"})
	if err != nil {
		t.Fatalf("ValidateHealthTargets returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d targets, want duplicate removed", len(got))
	}
}

func TestValidateHealthTargetsRejectsShellSyntax(t *testing.T) {
	if _, err := ValidateHealthTargets([]string{"1.1.1.1; reboot"}); err == nil {
		t.Fatal("expected shell syntax to be rejected")
	}
}
