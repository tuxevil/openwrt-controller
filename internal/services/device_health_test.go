package services

import (
	"database/sql"
	"testing"
	"time"
)

func TestClassifyDeviceHealth_OnlineWhenRecentAndNoIncidents(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	last := now.Add(-30 * time.Second)

	got := ClassifyDeviceHealth(sql.NullTime{Valid: true, Time: last}, nil, now)
	if got != HealthOnline {
		t.Errorf("expected HEALTH_ONLINE, got %q", got)
	}
}

func TestClassifyDeviceHealth_StaleBetweenThresholds(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	last := now.Add(-3 * time.Minute) // 180s, between stale (120) and offline (600)

	got := ClassifyDeviceHealth(sql.NullTime{Valid: true, Time: last}, nil, now)
	if got != HealthStale {
		t.Errorf("expected HEALTH_STALE for 3-minute-old telemetry, got %q", got)
	}
}

func TestClassifyDeviceHealth_OfflineAfterLongSilence(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	last := now.Add(-15 * time.Minute)

	got := ClassifyDeviceHealth(sql.NullTime{Valid: true, Time: last}, nil, now)
	if got != HealthOffline {
		t.Errorf("expected HEALTH_OFFLINE for 15-minute-old telemetry, got %q", got)
	}
}

func TestClassifyDeviceHealth_OfflineWhenLastSeenIsNull(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)

	got := ClassifyDeviceHealth(sql.NullTime{Valid: false}, nil, now)
	if got != HealthOffline {
		t.Errorf("expected HEALTH_OFFLINE for NULL last_seen_at, got %q", got)
	}
}

func TestClassifyDeviceHealth_OfflineWhenNodeDownIncidentOpen(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	last := now.Add(-10 * time.Second) // fresh telemetry but NODE_DOWN is open

	got := ClassifyDeviceHealth(
		sql.NullTime{Valid: true, Time: last},
		[]IncidentSummary{{IncidentType: "NODE_DOWN", Severity: "CRITICAL"}},
		now,
	)
	if got != HealthOffline {
		t.Errorf("expected HEALTH_OFFLINE when NODE_DOWN is open, got %q", got)
	}
}

func TestClassifyDeviceHealth_DegradedWhenFreshWithOtherIncident(t *testing.T) {
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	last := now.Add(-10 * time.Second)

	got := ClassifyDeviceHealth(
		sql.NullTime{Valid: true, Time: last},
		[]IncidentSummary{{IncidentType: "CPU_OVERLOAD", Severity: "WARNING"}},
		now,
	)
	if got != HealthDegraded {
		t.Errorf("expected HEALTH_DEGRADED when fresh telemetry with non-fatal incident, got %q", got)
	}
}

func TestClassifyDeviceHealth_NodeDownWinsOverDegraded(t *testing.T) {
	// Even with a fresh CPU_OVERLOAD, NODE_DOWN forces OFFLINE.
	now := time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)
	last := now.Add(-10 * time.Second)

	got := ClassifyDeviceHealth(
		sql.NullTime{Valid: true, Time: last},
		[]IncidentSummary{
			{IncidentType: "CPU_OVERLOAD", Severity: "WARNING"},
			{IncidentType: "NODE_DOWN", Severity: "CRITICAL"},
		},
		now,
	)
	if got != HealthOffline {
		t.Errorf("expected HEALTH_OFFLINE when both incidents present, got %q", got)
	}
}
