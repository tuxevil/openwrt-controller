package rolloutworker

import (
	"testing"

	"openwrt-controller/internal/database"
)

func TestCanAdvancePhaseRequiresSuccessfulTerminalProgress(t *testing.T) {
	lease := database.RolloutLease{Cursor: 1}
	if canAdvancePhase(lease, database.RolloutProgress{FailureCount: 1}) {
		t.Fatal("worker advanced after a terminal failure")
	}
	if canAdvancePhase(lease, database.RolloutProgress{FailureCount: 0}) != true {
		t.Fatal("worker did not advance after successful terminal progress")
	}
	if canAdvancePhase(database.RolloutLease{}, database.RolloutProgress{}) {
		t.Fatal("worker advanced without a completed phase")
	}
}
