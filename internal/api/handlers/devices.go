package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func GetDevicesHandler(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	query := `SELECT id, site_id, name, model, status, last_seen_at FROM devices LIMIT 1000`
	args := []interface{}{}

	if statusFilter == "pending" {
		query = `SELECT id, site_id, name, model, status, last_seen_at FROM devices WHERE site_id IS NULL LIMIT 1000`
	}

	rows, err := database.Tx(r.Context()).Query(query, args...)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var id string
		var siteID, name, model, status sql.NullString
		var lastSeen sql.NullTime
		if err := rows.Scan(&id, &siteID, &name, &model, &status, &lastSeen); err == nil {
			var lastSeenStr string
			if lastSeen.Valid {
				lastSeenStr = lastSeen.Time.Format(time.RFC3339)
			}
			dev := map[string]interface{}{
				"id":           id,
				"name":         name.String,
				"model":        model.String,
				"status":       status.String,
				"last_seen_at": lastSeenStr,
			}
			if siteID.Valid {
				dev["site_id"] = siteID.String
			}
			devices = append(devices, dev)
		}
	}

	if devices == nil {
		devices = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  devices,
		"error": nil,
	})
}

func GetSiteDevicesHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	query := `SELECT id, site_id, name, model, status, last_seen_at, last_config_pulled_at, last_ip, agent_version, state_json FROM devices WHERE site_id = $1`
	rows, err := database.Tx(r.Context()).Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	now := time.Now()

	var devices []map[string]interface{}
	var deviceIDs []string
	// First pass: read the device rows, capture IDs for the batched
	// incident lookup below. The previous N+1 ran one SELECT per device;
	// with even 50 devices that was 50 round-trips on every dashboard
	// refresh. We now do exactly one.
	for rows.Next() {
		var id string
		var sID, name, model, status, lastIP, agentVersion sql.NullString
		var lastSeen, lastPulled sql.NullTime
		var stateJSON []byte
		if err := rows.Scan(&id, &sID, &name, &model, &status, &lastSeen, &lastPulled, &lastIP, &agentVersion, &stateJSON); err == nil {
			var lastSeenStr, lastPulledStr string
			if lastSeen.Valid {
				lastSeenStr = lastSeen.Time.Format(time.RFC3339)
			}
			if lastPulled.Valid {
				lastPulledStr = lastPulled.Time.Format(time.RFC3339)
			}
			deviceIDs = append(deviceIDs, id)

			dev := map[string]interface{}{
				"id":                    id,
				"site_id":               siteID,
				"name":                  name.String,
				"model":                 model.String,
				"status":                status.String,
				"last_seen_at":          lastSeenStr,
				"last_config_pulled_at": lastPulledStr,
				"last_ip":               lastIP.String,
				"agent_version":         agentVersion.String,
				"open_incidents":        []map[string]string{},
			}
			if len(stateJSON) > 0 {
				var parsedState map[string]interface{}
				if json.Unmarshal(stateJSON, &parsedState) == nil {
					dev["state_json"] = parsedState
				}
			}
			devices = append(devices, dev)
		}
	}

	// Second pass: one batched query for all open incidents across all
	// devices in this site. device_id = ANY($1) uses the index on
	// incidents.device_id (added by RunTenantMigrations). Builds a
	// map[deviceID][]IncidentSummary so per-device health classification
	// stays a hash lookup away.
	incidentsByDevice := make(map[string][]services.IncidentSummary)
	if len(deviceIDs) > 0 {
		if rows2, err2 := loadOpenIncidentsBatch(r, deviceIDs); err2 == nil {
			incidentsByDevice = rows2
		}
	}

	// Third pass: stamp health + incidents onto each device row.
	for _, dev := range devices {
		id, _ := dev["id"].(string)
		incs := incidentsByDevice[id]
		lastSeenStr, _ := dev["last_seen_at"].(string)
		lastSeenTime, _ := time.Parse(time.RFC3339Nano, lastSeenStr)

		dev["health"] = string(services.ClassifyDeviceHealth(
			sql.NullTime{Valid: !lastSeenTime.IsZero(), Time: lastSeenTime},
			incs,
			now,
		))
		dev["open_incidents"] = incidentsToMap(incs)
	}

	if devices == nil {
		devices = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  devices,
		"error": nil,
	})
}

// loadOpenIncidents queries the {tenant}.incidents table for OPEN rows
// belonging to the given device. Returns a flat []services.IncidentSummary
// that the health classifier consumes.
//
// Kept simple: one indexed query per device. For a site with N devices
// this is N small queries inside an already-issued transaction; for the
// dashboard fleet sizes (≤ a few hundred) this is well below the noise
// floor. If it ever becomes hot, switch to a single IN(…) batched query
// using WHERE device_id = ANY($1::text[]).
//
// The schema name is taken from the request's tenant context (set by the
// auth middleware) and is used as a fully-qualified prefix. We deliberately
// avoid relying on the tx's search_path here because the auth middleware
// sets it AFTER the auth resolution, and the request context a handler
// receives through r.Context() can sometimes carry a tx that was opened
// before the search_path was set on the same connection (the auth flow
// explicitly validates `tenants` first against `public` before flipping
// the path). Fully-qualifying eliminates that race entirely.
func loadOpenIncidents(r *http.Request, deviceID string) ([]services.IncidentSummary, error) {
	// GetTenantSchema returns the full schema name set by the auth
	// middleware (e.g. "tenant_dragontec"); use SafeSchemaIdent to
	// whitelist-validate the identifier without re-prefixing.
	schema, err := database.SafeSchemaIdent(middleware.GetTenantSchema(r))
	if err != nil || schema == "" {
		return nil, fmt.Errorf("no tenant schema in context: %w", err)
	}
	rows, err := database.Tx(r.Context()).Query(
		"SELECT incident_type, severity FROM "+schema+".incidents WHERE device_id = $1 AND status = 'OPEN'",
		deviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []services.IncidentSummary
	for rows.Next() {
		var t, s string
		if err := rows.Scan(&t, &s); err != nil {
			continue
		}
		out = append(out, services.IncidentSummary{IncidentType: t, Severity: s})
	}
	return out, nil
}

// loadOpenIncidentsBatch fetches OPEN incidents for many devices in a
// single query. Returns a map keyed by device_id for O(1) lookup. This
// replaces the per-device query loop and is the reason the dashboard
// site list now refreshes in tens of ms instead of seconds.
func loadOpenIncidentsBatch(r *http.Request, deviceIDs []string) (map[string][]services.IncidentSummary, error) {
	out := make(map[string][]services.IncidentSummary)
	if len(deviceIDs) == 0 {
		return out, nil
	}
	schema, err := database.SafeSchemaIdent(middleware.GetTenantSchema(r))
	if err != nil || schema == "" {
		return out, fmt.Errorf("no tenant schema in context: %w", err)
	}
	rows, err := database.Tx(r.Context()).Query(
		"SELECT device_id, incident_type, severity FROM "+schema+".incidents WHERE device_id = ANY($1::text[]) AND status = 'OPEN'",
		deviceIDsAsPGArray(deviceIDs),
	)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	for rows.Next() {
		var dev, t, s string
		if err := rows.Scan(&dev, &t, &s); err != nil {
			continue
		}
		out[dev] = append(out[dev], services.IncidentSummary{IncidentType: t, Severity: s})
	}
	return out, nil
}

// deviceIDsAsPGArray formats a []string as a Postgres text array literal
// (`{a,b,c}`). Used with the ANY($1::text[]) operator; safer than
// building a comma-separated IN-list because it handles commas and
// quotes inside MACs (which are illegal but we still defend in depth).
func deviceIDsAsPGArray(ids []string) string {
	// Empty literal would make ANY() always-false. Caller checks len.
	var b strings.Builder
	b.WriteByte('{')
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		// Backslash-escape any embedded braces or backslashes per
		// Postgres array-literal rules. MACs won't contain these, but
		// this keeps the helper safe for future non-MAC IDs.
		for _, r := range id {
			switch r {
			case '{', '}', '"', '\\':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('}')
	return b.String()
}

func incidentsToMap(in []services.IncidentSummary) []map[string]string {
	if len(in) == 0 {
		return []map[string]string{}
	}
	out := make([]map[string]string, 0, len(in))
	for _, i := range in {
		out = append(out, map[string]string{
			"type":     i.IncidentType,
			"severity": i.Severity,
		})
	}
	return out
}

func ForgetDeviceHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	// Clean up child tables to prevent foreign key constraint violations
	database.Tx(r.Context()).Exec("DELETE FROM backups WHERE device_id = $1", deviceID)
	database.Tx(r.Context()).Exec("DELETE FROM incidents WHERE device_id = $1", deviceID)

	res, err := database.Tx(r.Context()).Exec("DELETE FROM devices WHERE id = $1", deviceID)
	if err != nil {
		http.Error(w, `{"error": "database error: " + err.Error()}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
}
