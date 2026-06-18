package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

// ─── WIFI_SURVEY domain types ───────────────────────────────────────────────

type Survey struct {
	ID              string   `json:"id"`
	SiteID          string   `json:"site_id"`
	Name            string   `json:"name"`
	SurveyorMAC     *string  `json:"surveyor_mac,omitempty"`
	SurveyorLabel   *string  `json:"surveyor_label,omitempty"`
	Status          string   `json:"status"` // pending|active|completed|aborted
	AccessMode      string   `json:"access_mode"`
	SurveyTokenHash *string  `json:"-"`
	TokenFirstUsedAt *string `json:"token_first_used_at,omitempty"`
	TokenFirstIP    *string  `json:"token_first_ip,omitempty"`
	TokenFirstUA    *string  `json:"token_first_ua,omitempty"`
	TokenRevokedAt  *string  `json:"token_revoked_at,omitempty"`
	TokenRotatedAt  *string  `json:"token_rotated_at,omitempty"`
	StartedAt       *string  `json:"started_at,omitempty"`
	EndedAt         *string  `json:"ended_at,omitempty"`
	CreatedBy       *string  `json:"created_by,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	// Aggregated fields populated on list queries
	PointCount int      `json:"point_count,omitempty"`
	MinDBM     *float64 `json:"min_dbm,omitempty"`
	MaxDBM     *float64 `json:"max_dbm,omitempty"`
	AvgDBM     *float64 `json:"avg_dbm,omitempty"`
	// Returned only on POST/rotate so the admin can hand the URL to the phone
	SurveyToken string `json:"survey_token,omitempty"`
	SurveyURL   string `json:"survey_url,omitempty"`
}

type SurveyPoint struct {
	ID          int64    `json:"id"`
	SurveyID    string   `json:"survey_id"`
	APID        string   `json:"ap_id"`
	Lat         *float64 `json:"lat,omitempty"`
	Lon         *float64 `json:"lon,omitempty"`
	AccuracyM   *float32 `json:"accuracy_m,omitempty"`
	SignalDBM   *float32 `json:"signal_dbm,omitempty"`
	NoiseDBM    *float32 `json:"noise_dbm,omitempty"`
	SNR         *float32 `json:"snr,omitempty"`
	BSSID       *string  `json:"bssid,omitempty"`
	NeighborAPs string   `json:"neighbor_aps"` // raw JSON string
	CapturedAt  string   `json:"captured_at"`
}

// ─── CRUD helpers ────────────────────────────────────────────────────────────

func GetSiteSurveys(ctx context.Context, schema, siteID string) ([]Survey, error) {
	rows, err := Tx(ctx).Query(fmt.Sprintf(`
		SELECT s.id, s.site_id, s.name, s.status, s.access_mode,
		       s.started_at, s.ended_at, s.created_at,
		       COALESCE(p.cnt, 0),
		       p.min_dbm, p.max_dbm, p.avg_dbm
		FROM %[1]s.wifi_surveys s
		LEFT JOIN (
			SELECT survey_id,
			       COUNT(*) AS cnt,
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
		if err := rows.Scan(&s.ID, &s.SiteID, &s.Name, &s.Status, &s.AccessMode,
			&started, &ended, &created, &s.PointCount, &s.MinDBM, &s.MaxDBM, &s.AvgDBM); err != nil {
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
		       created_by
		FROM %s.wifi_surveys WHERE id = $1
	`, schema), surveyID)

	var s Survey
	var started, ended, created, updated *string
	var smac, slabel, tfu, tfip, tfua, trev, trot, cby *string
	if err := row.Scan(&s.ID, &s.SiteID, &s.Name, &s.Status, &s.AccessMode,
		&started, &ended, &created, &updated,
		&smac, &slabel, &tfu, &tfip, &tfua, &trev, &trot, &cby); err != nil {
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

func CreateSurvey(ctx context.Context, schema, siteID, name, surveyorLabel, accessMode, createdBy string) (string, error) {
	if accessMode == "" {
		accessMode = "authenticated"
	}
	var id string
	err := Tx(ctx).QueryRow(fmt.Sprintf(`
		INSERT INTO %s.wifi_surveys (site_id, name, surveyor_label, access_mode, created_by, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, schema), siteID, name, nullString(surveyorLabel), accessMode, nullString(createdBy)).Scan(&id)
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
		       captured_at
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
			&p.NeighborAPs, &p.CapturedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func InsertSurveyPoint(ctx context.Context, schema string, p SurveyPoint) error {
	_, err := Tx(ctx).Exec(fmt.Sprintf(`
		INSERT INTO %s.wifi_survey_points
			(survey_id, ap_id, lat, lon, accuracy_m, signal_dbm, noise_dbm, snr, bssid, neighbor_aps, captured_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11)
	`, schema),
		p.SurveyID, p.APID, p.Lat, p.Lon, p.AccuracyM,
		p.SignalDBM, p.NoiseDBM, p.SNR, p.BSSID, p.NeighborAPs, p.CapturedAt)
	return err
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
