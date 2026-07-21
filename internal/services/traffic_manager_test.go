package services

import "testing"

func TestBuildBandwidthCommandBoundsNumericInputs(t *testing.T) {
	if _, err := buildBandwidthCommand(5000, 5000); err != nil {
		t.Fatalf("valid bandwidth values rejected: %v", err)
	}
	for _, values := range [][2]int{{0, 5000}, {5000, 0}, {-1, 5000}, {1_000_001, 5000}} {
		if _, err := buildBandwidthCommand(values[0], values[1]); err == nil {
			t.Errorf("accepted unsafe bandwidth values: download=%d upload=%d", values[0], values[1])
		}
	}
}
