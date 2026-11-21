package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ─── WIFI_SURVEY domain types ───────────────────────────────────────────────

type Survey struct {
	ID               string  `json:"id"`
	SiteID           string  `json:"site_id"`
	Name             string  `json:"name"`
	SurveyorMAC      *string `json:"surveyor_mac,omitempty"`
	SurveyorLabel    *string `json:"surveyor_label,omitempty"`
	Status           string  `json:"status"` // pending|active|completed|aborted
	AccessMode       string  `json:"access_mode"`
	SurveyTokenHash  *string `json:"-"`
	TokenFirstUsedAt *string `json:"token_first_used_at,omitempty"`
	TokenFirstIP     *string `json:"token_first_ip,omitempty"`
	TokenFirstUA     *string `json:"token_first_ua,omitempty"`
	TokenRevokedAt   *string `json:"token_revoked_at,omitempty"`
	TokenRotatedAt   *string `json:"token_rotated_at,omitempty"`
	StartedAt        *string `json:"started_at,omitempty"`
	EndedAt          *string `json:"ended_at,omitempty"`
	CreatedBy        *string `json:"created_by,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	// Aggregated fields populated on list queries
	PointCount int      `json:"point_count,omitempty"`
	ProbeCount int      `json:"probe_count,omitempty"`
	MinDBM     *float64 `json:"min_dbm,omitempty"`
	MaxDBM     *float64 `json:"max_dbm,omitempty"`
	AvgDBM     *float64 `json:"avg_dbm,omitempty"`
	// Returned only on POST/rotate so the admin can hand the URL to the phone
	SurveyToken string `json:"survey_token,omitempty"`
	SurveyURL   string `json:"survey_url,omitempty"`
	// Immutable snapshot of placed AP positions at the time this survey ran
	APPositionsSnapshot []SurveyAPPosition `json:"ap_positions_snapshot,omitempty"`
}

type SurveyAPPosition struct {
	ID         string  `json:"id"`
	Name       string  `json:"name,omitempty"`
	LastIP     string  `json:"last_ip,omitempty"`
	Label      string  `json:"label"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Note       string  `json:"note,omitempty"`
	CapturedAt string  `json:"captured_at,omitempty"`
}

type SurveyPoint struct {
	ID                int64    `json:"id"`
	SurveyID          string   `json:"survey_id"`
	APID              string   `json:"ap_id"`
	Lat               *float64 `json:"lat,omitempty"`
	Lon               *float64 `json:"lon,omitempty"`
	AccuracyM         *float32 `json:"accuracy_m,omitempty"`
	SignalDBM         *float32 `json:"signal_dbm,omitempty"`
	NoiseDBM          *float32 `json:"noise_dbm,omitempty"`
	SNR               *float32 `json:"snr,omitempty"`
	BSSID             *string  `json:"bssid,omitempty"`
	NeighborAPs       string   `json:"neighbor_aps"` // raw JSON string
	CapturedAt        string   `json:"captured_at"`
	ClientSampleID    *string  `json:"client_sample_id,omitempty"`
	MissingReason     string   `json:"missing_reason,omitempty"`
	ReportingAPsCount *int     `json:"reporting_aps_count,omitempty"`
	TotalAPsCount     *int     `json:"total_aps_count,omitempty"`
	SilentAPs         string   `json:"silent_aps,omitempty"` // raw JSON array string
	ProbeRTTMs        *float32 `json:"probe_rtt_ms,omitempty"`
	ProbeJitterMs     *float32 `json:"probe_jitter_ms,omitempty"`
	ProbeLossPct      *float32 `json:"probe_loss_pct,omitempty"`
}

type SiteAPRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	LastIP string `json:"last_ip"`
}

func (a SiteAPRef) DisplayLabel() string {
	name := strings.TrimSpace(a.Name)
	ip := strings.TrimSpace(a.LastIP)
	switch {
	case name != "" && ip != "":
		return fmt.Sprintf("%s (%s)", name, ip)
	case name != "":
		return name
	case ip != "":
		return ip
	default:
		return a.ID
	}
}

type SiteAPTelemetry struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	LastIP       string   `json:"last_ip"`
	Label        string   `json:"label"`
	AgeSeconds   int      `json:"age_s"`
	Reporting    bool     `json:"reporting"`
	SeesSurveyor bool     `json:"sees_surveyor"`
	SurveyorDBM  *float64 `json:"surveyor_dbm,omitempty"`
}

type SurveyFloorplan struct {
	ImageDataURL string        `json:"image_data_url"`
	Filename     string        `json:"filename,omitempty"`
	Bounds       [2][2]float64 `json:"bounds"`
	Opacity      float64       `json:"opacity"`
	UpdatedAt    string        `json:"updated_at,omitempty"`
}

// ─── CRUD helpers ────────────────────────────────────────────────────────────

func GetSiteSurveys(ctx context.Context, schema, siteID string) ([]Survey, error) {
	rows, err := Tx(ctx).Query(fmt.Sprintf(`
		SELECT s.id, s.site_id, s.name, s.status, s.access_mode,
		       s.started_at, s.ended_at, s.created_at,
		       COALESCE(p.cnt, 0),
		       COALESCE(p.probe_cnt, 0),
		       p.min_dbm, p.max_dbm, p.avg_dbm,
		       COALESCE(s.ap_positions_snapshot::text, '[]')
		FROM %[1]s.wifi_surveys s
		LEFT JOIN (
			SELECT survey_id,
			       COUNT(*) AS cnt,
			       COUNT(*) FILTER (WHERE probe_rtt_ms IS NOT NULL OR probe_loss_pct IS NOT NULL) AS probe_cnt,
			       MIN(signal_dbm) AS min_dbm,
			       MAX(signal_dbm) AS max_dbm,
			       AVG(signal_dbm) AS avg_dbm
			FROM %[1]s.wifi_survey_points
			GROUP BY survey_id
		) p ON p.survey_id = s.id
		WHERE s.site_id = $1
		ORDER BY s.created_at DESC
	`, schema), siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Survey
	for rows.Next() {
		var s Survey
		var started, ended, created *string
		var apSnapRaw string
		if err := rows.Scan(&s.ID, &s.SiteID, &s.Name, &s.Status, &s.AccessMode,
			&started, &ended, &created, &s.PointCount, &s.ProbeCount, &s.MinDBM, &s.MaxDBM, &s.AvgDBM, &apSnapRaw); err != nil {
			return nil, err
		}
		if started != nil {
			s.StartedAt = started
		}
		if ended != nil {
			s.EndedAt = ended
		}
		if created != nil {
			s.CreatedAt = *created
		}
		if apSnapRaw != "" && apSnapRaw != "[]" {
			_ = json.Unmarshal([]byte(apSnapRaw), &s.APPositionsSnapshot)
		}
		out = append(out, s)
	}
	return out, nil
}

func GetSurvey(ctx context.Context, schema, surveyID string) (*Survey, error) {
	row := Tx(ctx).QueryRow(fmt.Sprintf(`
		SELECT id, site_id, name, status, access_mode,
		       started_at, ended_at, created_at, updated_at,
		       surveyor_mac, surveyor_label,
		       token_first_used_at, token_first_ip, token_first_ua,
		       token_revoked_at, token_rotated_at,
		       created_by, survey_token_hash,
		       COALESCE(ap_positions_snapshot::text, '[]')
		FROM %s.wifi_surveys WHERE id = $1
	`, schema), surveyID)

	var s Survey
	var started, ended, created, updated *string
	var smac, slabel, tfu, tfip, tfua, trev, trot, cby *string
	var apSnapRaw string
	if err := row.Scan(&s.ID, &s.SiteID, &s.Name, &s.Status, &s.AccessMode,
		&started, &ended, &created, &updated,
		&smac, &slabel, &tfu, &tfip, &tfua, &trev, &trot, &cby, &s.SurveyTokenHash, &apSnapRaw); err != nil {
		return nil, err
	}
	if started != nil {
		s.StartedAt = started
	}
	if ended != nil {
		s.EndedAt = ended
	}
	if created != nil {
		s.CreatedAt = *created
	}
	if updated != nil {
		s.UpdatedAt = *updated
	}
	if apSnapRaw != "" && apSnapRaw != "[]" {
		_ = json.Unmarshal([]byte(apSnapRaw), &s.APPositionsSnapshot)
	}
	s.SurveyorMAC = smac
	s.SurveyorLabel = slabel
	s.TokenFirstUsedAt = tfu
	s.TokenFirstIP = tfip
	s.TokenFirstUA = tfua
	s.TokenRevokedAt = trev
	s.TokenRotatedAt = trot
	s.CreatedBy = cby
	return &s, nil
}

func CreateSurvey(ctx context.Context, schema, siteID, name, surveyorLabel, surveyorMAC, accessMode, createdBy string) (string, error) {
	if accessMode == "" {
		accessMode = "authenticated"
	}
	var id string
	err := Tx(ctx).QueryRow(fmt.Sprintf(`
		INSERT INTO %s.wifi_surveys (site_id, name, surveyor_label, surveyor_mac, access_mode, created_by, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, schema), siteID, name, nullString(surveyorLabel), nullString(surveyorMAC), accessMode, nullString(createdBy)).Scan(&id)
	return id, err
}

func DeleteSurvey(ctx context.Context, schema, surveyID string) error {
	_, err := Tx(ctx).Exec(fmt.Sprintf(`DELETE FROM %s.wifi_surveys WHERE id = $1`, schema), surveyID)
	return err
}

func SetSurveyToken(ctx context.Context, schema, surveyID, tokenHash string) error {
	_, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.wifi_surveys
		   SET survey_token_hash = $2,
		       token_rotated_at = COALESCE(token_rotated_at, CURRENT_TIMESTAMP),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1
	`, schema), surveyID, tokenHash)
	return err
}

// SetSurveyStatus transitions a survey between pending/active/completed/aborted
// and stamps the corresponding timestamp exactly once (started_at on the
// first transition to active, ended_at on the first transition to
// completed/aborted).
//
// Two separate Exec calls (start vs end) instead of one with CASE on $2.
// Postgres' prepared-statement protocol refuses to plan a query that
// uses the same $2 parameter in multiple CASE branches with different
// expected types (SQLSTATE 42P08 "inconsistent types deduced for
// parameter $2"). Splitting avoids the conflict and makes the intent
// self-documenting. Also fast: both branches use the indexed pkey.
func SetSurveyStatus(ctx context.Context, schema, surveyID, status string) error {
	switch status {
	case "active":
		_, err := Tx(ctx).Exec(fmt.Sprintf(`
			UPDATE %s.wifi_surveys
			   SET status = $2::varchar,
			       started_at = CASE WHEN started_at IS NULL THEN CURRENT_TIMESTAMP ELSE started_at END,
			       updated_at = CURRENT_TIMESTAMP
			 WHERE id = $1::uuid
		`, schema), surveyID, status)
		return err
	case "completed", "aborted":
		_, err := Tx(ctx).Exec(fmt.Sprintf(`
			UPDATE %s.wifi_surveys
			   SET status = $2::varchar,
			       ended_at = CASE WHEN ended_at IS NULL THEN CURRENT_TIMESTAMP ELSE ended_at END,
			       updated_at = CURRENT_TIMESTAMP
			 WHERE id = $1::uuid
		`, schema), surveyID, status)
		return err
	default:
		_, err := Tx(ctx).Exec(fmt.Sprintf(`
			UPDATE %s.wifi_surveys
			   SET status = $2::varchar,
			       updated_at = CURRENT_TIMESTAMP
			 WHERE id = $1::uuid
		`, schema), surveyID, status)
		return err
	}
}

func SetSurveyTokenFirstUse(ctx context.Context, schema, surveyID, ip, ua string) error {
	_, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.wifi_surveys
		   SET token_first_used_at = COALESCE(token_first_used_at, CURRENT_TIMESTAMP),
		       token_first_ip      = COALESCE(token_first_ip, $2::inet),
		       token_first_ua      = COALESCE(token_first_ua, $3),
		       updated_at          = CURRENT_TIMESTAMP
		 WHERE id = $1
	`, schema), surveyID, ip, ua)
	return err
}

func RevokeSurveyToken(ctx context.Context, schema, surveyID string) error {
	_, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.wifi_surveys
		   SET token_revoked_at = CURRENT_TIMESTAMP,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1
	`, schema), surveyID)
	return err
}

func GetSurveyPoints(ctx context.Context, schema, surveyID string, limit int) ([]SurveyPoint, error) {
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	rows, err := Tx(ctx).Query(fmt.Sprintf(`
		SELECT id, survey_id, ap_id, lat, lon, accuracy_m,
		       signal_dbm, noise_dbm, snr, COALESCE(bssid, ''),
		       COALESCE(neighbor_aps::text, '[]'),
		       captured_at,
		       client_sample_id,
		       COALESCE(missing_reason, ''),
		       reporting_aps_count,
		       total_aps_count,
		       COALESCE(silent_aps::text, '[]'),
		       probe_rtt_ms,
		       probe_jitter_ms,
		       probe_loss_pct
		FROM %s.wifi_survey_points
		WHERE survey_id = $1
		ORDER BY captured_at ASC
		LIMIT %s
	`, schema, strconv.Itoa(limit)), surveyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SurveyPoint
	for rows.Next() {
		var p SurveyPoint
		if err := rows.Scan(&p.ID, &p.SurveyID, &p.APID,
			&p.Lat, &p.Lon, &p.AccuracyM,
			&p.SignalDBM, &p.NoiseDBM, &p.SNR, &p.BSSID,
			&p.NeighborAPs, &p.CapturedAt,
			&p.ClientSampleID, &p.MissingReason,
			&p.ReportingAPsCount, &p.TotalAPsCount, &p.SilentAPs,
			&p.ProbeRTTMs, &p.ProbeJitterMs, &p.ProbeLossPct); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// InsertSurveyPoint inserts a correlated GPS survey point. When ClientSampleID
// is present, retries with the same (survey_id, client_sample_id) are ignored
// idempotently and return inserted=false, err=nil.
func InsertSurveyPoint(ctx context.Context, schema string, p SurveyPoint) (bool, error) {
	if p.NeighborAPs == "" {
		p.NeighborAPs = "[]"
	}
	if p.SilentAPs == "" {
		p.SilentAPs = "[]"
	}
	res, err := Tx(ctx).Exec(fmt.Sprintf(`
		INSERT INTO %s.wifi_survey_points
			(survey_id, ap_id, lat, lon, accuracy_m, signal_dbm, noise_dbm, snr, bssid, neighbor_aps, captured_at,
			 client_sample_id, missing_reason, reporting_aps_count, total_aps_count, silent_aps,
			 probe_rtt_ms, probe_jitter_ms, probe_loss_pct)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12, $13, $14, $15, $16::jsonb, $17, $18, $19)
		ON CONFLICT (survey_id, client_sample_id) WHERE client_sample_id IS NOT NULL DO NOTHING
	`, schema),
		p.SurveyID, p.APID, p.Lat, p.Lon, p.AccuracyM,
		p.SignalDBM, p.NoiseDBM, p.SNR, p.BSSID, p.NeighborAPs, p.CapturedAt,
		p.ClientSampleID, p.MissingReason, p.ReportingAPsCount, p.TotalAPsCount, p.SilentAPs,
		p.ProbeRTTMs, p.ProbeJitterMs, p.ProbeLossPct)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return true, nil
	}
	return affected > 0, nil
}

// GetSiteDeviceRefs returns the devices belonging to a site for telemetry diagnostics.
func GetSiteDeviceRefs(ctx context.Context, schema, siteID string) ([]SiteAPRef, error) {
	rows, err := Tx(ctx).Query(fmt.Sprintf(`
		SELECT id, COALESCE(name, ''), COALESCE(last_ip, '')
		FROM %s.devices
		WHERE site_id = $1
		ORDER BY last_ip, id
	`, schema), siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SiteAPRef
	for rows.Next() {
		var d SiteAPRef
		if err := rows.Scan(&d.ID, &d.Name, &d.LastIP); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetSiteAPTelemetryHealth inspects live AP telemetry freshness and checks
// whether any reporting AP currently sees surveyorMAC in wireless_stations.
func GetSiteAPTelemetryHealth(ctx context.Context, schema, siteID, surveyorMAC string) ([]SiteAPTelemetry, error) {
	rows, err := Tx(ctx).Query(fmt.Sprintf(`
		SELECT id, COALESCE(name, ''), COALESCE(last_ip, ''),
		       COALESCE(EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - updated_at))::int, 999999),
		       state_json
		FROM %s.devices
		WHERE site_id = $1
		ORDER BY last_ip, id
	`, schema), siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SiteAPTelemetry
	for rows.Next() {
		var d SiteAPTelemetry
		var stateRaw []byte
		if err := rows.Scan(&d.ID, &d.Name, &d.LastIP, &d.AgeSeconds, &stateRaw); err != nil {
			return nil, err
		}
		if d.AgeSeconds < 0 {
			d.AgeSeconds = 0
		}
		d.Label = SiteAPRef{ID: d.ID, Name: d.Name, LastIP: d.LastIP}.DisplayLabel()
		d.Reporting = d.AgeSeconds <= 45

		if d.Reporting && surveyorMAC != "" && len(stateRaw) > 0 {
			var state struct {
				Stations map[string][]struct {
					MAC    string  `json:"mac"`
					Signal float64 `json:"signal"`
				} `json:"wireless_stations"`
			}
			if json.Unmarshal(stateRaw, &state) == nil {
				for _, list := range state.Stations {
					for _, st := range list {
						if strings.EqualFold(st.MAC, surveyorMAC) {
							d.SeesSurveyor = true
							if st.Signal != 0 {
								sig := st.Signal
								d.SurveyorDBM = &sig
							}
							break
						}
					}
					if d.SeesSurveyor {
						break
					}
				}
			}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetSiteSurveyFloorplan returns the saved local floorplan for a site, or nil if unset.
func GetSiteSurveyFloorplan(ctx context.Context, schema, siteID string) (*SurveyFloorplan, error) {
	var raw string
	err := Tx(ctx).QueryRow(fmt.Sprintf(`
		SELECT COALESCE(survey_floorplan::text, '{}')
		FROM %s.site_configs
		WHERE site_id = $1
	`, schema), siteID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if raw == "" || raw == "{}" || raw == "null" {
		return nil, nil
	}
	var fp SurveyFloorplan
	if err := json.Unmarshal([]byte(raw), &fp); err != nil {
		return nil, err
	}
	if fp.ImageDataURL == "" {
		return nil, nil
	}
	return &fp, nil
}

// SaveSiteSurveyFloorplan persists (or clears when fp == nil) the site's local floorplan.
func SaveSiteSurveyFloorplan(ctx context.Context, schema, siteID string, fp *SurveyFloorplan) error {
	payload := "{}"
	if fp != nil && fp.ImageDataURL != "" {
		b, err := json.Marshal(fp)
		if err != nil {
			return err
		}
		payload = string(b)
	}
	_, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.site_configs
		   SET survey_floorplan = $2::jsonb
		 WHERE site_id = $1
	`, schema), siteID, payload)
	return err
}

// GetSitePlacedAPPositions returns all devices in the site that currently have
// geographic coordinates placed on the site map.
func GetSitePlacedAPPositions(ctx context.Context, schema, siteID string) ([]SurveyAPPosition, error) {
	rows, err := Tx(ctx).Query(fmt.Sprintf(`
		SELECT id, COALESCE(name, ''), COALESCE(last_ip, ''),
		       location_lat, location_lon, COALESCE(location_note, ''),
		       COALESCE(to_char(location_updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
		FROM %s.devices
		WHERE site_id = $1
		  AND location_lat IS NOT NULL
		  AND location_lon IS NOT NULL
		ORDER BY last_ip, id
	`, schema), siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SurveyAPPosition
	for rows.Next() {
		var p SurveyAPPosition
		if err := rows.Scan(&p.ID, &p.Name, &p.LastIP, &p.Lat, &p.Lon, &p.Note, &p.CapturedAt); err != nil {
			return nil, err
		}
		p.Label = SiteAPRef{ID: p.ID, Name: p.Name, LastIP: p.LastIP}.DisplayLabel()
		out = append(out, p)
	}
	return out, rows.Err()
}

// SnapshotSurveyAPPositions freezes the site's current placed AP positions onto
// a survey record so moving an AP later preserves historical survey layouts.
func SnapshotSurveyAPPositions(ctx context.Context, schema, surveyID, siteID string) ([]SurveyAPPosition, error) {
	positions, err := GetSitePlacedAPPositions(ctx, schema, siteID)
	if err != nil {
		return nil, err
	}
	if positions == nil {
		positions = []SurveyAPPosition{}
	}
	b, err := json.Marshal(positions)
	if err != nil {
		return nil, err
	}
	_, err = Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.wifi_surveys
		   SET ap_positions_snapshot = $2::jsonb,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1
	`, schema), surveyID, string(b))
	return positions, err
}

// UpdateDeviceLocation sets or clears a device's geographic coordinates on the site map.
func UpdateDeviceLocation(ctx context.Context, schema, siteID, deviceID string, lat, lon *float64, note string) (bool, error) {
	res, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.devices
		   SET location_lat = $1,
		       location_lon = $2,
		       location_note = $3,
		       location_updated_at = CASE WHEN $1 IS NULL THEN NULL ELSE CURRENT_TIMESTAMP END
		 WHERE lower(id) = lower($4) AND site_id = $5
	`, schema), lat, lon, note, deviceID, siteID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return true, nil
	}
	return affected > 0, nil
}

// GetActiveSurveyForSite returns the currently 'active' survey for a site, if any.
func GetActiveSurveyForSite(ctx context.Context, schema, siteID string) (*Survey, error) {
	row := Tx(ctx).QueryRow(fmt.Sprintf(`
		SELECT id FROM %s.wifi_surveys
		 WHERE site_id = $1 AND status = 'active'
		 ORDER BY started_at DESC NULLS LAST
		 LIMIT 1
	`, schema), siteID)
	var id string
	if err := row.Scan(&id); err != nil {
		return nil, err
	}
	return GetSurvey(ctx, schema, id)
}

// GetSiteConfigAllowPublicSurveys reads the allow_public_surveys flag for a site.
func GetSiteConfigAllowPublicSurveys(ctx context.Context, schema, siteID string) (bool, error) {
	var v bool
	err := Tx(ctx).QueryRow(fmt.Sprintf(`
		SELECT COALESCE(allow_public_surveys, false)
		FROM %s.site_configs WHERE site_id = $1
	`, schema), siteID).Scan(&v)
	if err != nil {
		return false, err
	}
	return v, nil
}

// IsGlobalSurveysPublicLockdown returns true if platform-level lockdown is enabled.
func IsGlobalSurveysPublicLockdown() (bool, error) {
	var v bool
	err := DB.QueryRow(`SELECT COALESCE(global_surveys_public_lockdown, false) FROM platform_settings WHERE id = 1`).Scan(&v)
	if err != nil {
		return false, err
	}
	return v, nil
}

// SetGlobalSurveysPublicLockdown toggles the global lockdown switch.
func SetGlobalSurveysPublicLockdown(enabled bool) error {
	_, err := DB.Exec(`
		UPDATE platform_settings
		   SET global_surveys_public_lockdown = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = 1
	`, enabled)
	return err
}

// ─── Token helpers ───────────────────────────────────────────────────────────

// HashToken returns the SHA-256 hex digest of the token. Tokens are 32 random
// bytes hex-encoded (64 chars). Hashed at rest so a DB dump doesn't leak
// surveyor URLs.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// GenerateSurveyToken returns a 64-char hex token and its hash.
func GenerateSurveyToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	hash = HashToken(token)
	return token, hash, nil
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
