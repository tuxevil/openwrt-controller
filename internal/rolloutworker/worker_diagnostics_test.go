package rolloutworker

import (
	"strings"
	"testing"
)

func TestBoundedDiagnostic(t *testing.T) {
	short := "worker lease lost"
	if got := boundedDiagnostic(short); got != short {
		t.Fatalf("short diagnostic changed: %q", got)
	}
	got := boundedDiagnostic(strings.Repeat("x", maxDiagnostic+100))
	if len(got) <= maxDiagnostic || !strings.HasSuffix(got, "[diagnostic output truncated]") {
		t.Fatalf("diagnostic was not bounded: length=%d", len(got))
	}
}
