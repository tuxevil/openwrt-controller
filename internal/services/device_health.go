package services

import (
	"database/sql"
	"time"
)

// DeviceHealth is the derived liveness/wellness classification for a single
// device. It is computed server-side from last_seen_at and the set of open
// incidents so the dashboard does not need to replicate the rule.
type DeviceHealth string

const (
	// HealthOnline: telemetry fresh (< staleAfter) and no incidents.
	HealthOnline DeviceHealth = "ONLINE"
	// HealthStale: telemetry between staleAfter and offlineAfter. The
	// alert engine has not yet escalated to NODE_DOWN but the link is
	// noticeably degraded.
	HealthStale DeviceHealth = "STALE"
	// HealthOffline: NODE_DOWN incident open OR no telemetry for
	// offlineAfter OR no telemetry ever (NULL last_seen_at).
	HealthOffline DeviceHealth = "OFFLINE"
	// HealthDegraded: telemetry fresh but one or more non-fatal
	// incidents open (CPU_OVERLOAD, SIGNAL_CRITICAL, …).
	HealthDegraded DeviceHealth = "DEGRADED"
)

const (
	// staleAfter matches the frontend threshold in
	// web/src/views/SiteDashboard.vue (getHealth, 120s). Keep the two
	// values in sync.
	staleAfter = 120 * time.Second
	// offlineAfter is 5x staleAfter. After this, the alert engine
	// would have already opened NODE_DOWN; we use the same value as
	// a fallback for the few seconds before the engine ticks.
	offlineAfter = 600 * time.Second
)

// ClassifyDeviceHealth returns the DeviceHealth for a device based on its
// last-seen timestamp and the slice of currently-open incidents. The now
// parameter is injected so tests are deterministic.
func ClassifyDeviceHealth(lastSeenAt sql.NullTime, openIncidents []IncidentSummary, now time.Time) DeviceHealth {
	hasNodeDown := false
	hasOther := false
	for _, inc := range openIncidents {
		if inc.IncidentType == "NODE_DOWN" {
			hasNodeDown = true
		} else {
			hasOther = true
		}
	}
	if hasNodeDown {
		return HealthOffline
	}
	if !lastSeenAt.Valid {
		return HealthOffline
	}
	age := now.Sub(lastSeenAt.Time)
	if age >= offlineAfter {
		return HealthOffline
	}
	if age >= staleAfter {
		return HealthStale
	}
	if hasOther {
		return HealthDegraded
	}
	return HealthOnline
}

// IncidentSummary is the minimal projection of a row in {schema}.incidents
// that the classifier needs. Defined here to keep the function decoupled
// from the storage layer and trivially testable. (Named with the
// "Summary" suffix to avoid collision with the OpenIncident function in
// alerts.go.)
type IncidentSummary struct {
	IncidentType string
	Severity     string
}
