package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrRolloutDraftNotFound indicates that an ID is not part of the site.
	ErrRolloutDraftNotFound = errors.New("rollout draft not found")
	// ErrRolloutDraftNotAvailable indicates that a draft cannot transition.
	ErrRolloutDraftNotAvailable = errors.New("rollout draft is not available")
)

// RolloutDraftRecord is the durable metadata and immutable plan for one site
// rollout. Results are kept separately so execution cannot rewrite the plan.
type RolloutDraftRecord struct {
	ID     string
	SiteID string
	// Generation is the site-scoped rollout sequence. It is not a device
	// desired/observed generation and must never be copied to devices.
	Generation      int64
	Status          string
	ClaimToken      string
	PlanHash        string
	RequestedBy     string
	TargetDeviceIDs json.RawMessage
	Plan            json.RawMessage
	Results         json.RawMessage
}

// CreateRolloutDraft reserves the next site rollout sequence and stores a DRAFT
// plan in one transaction. The sequence is independent of device generations.
func CreateRolloutDraft(ctx context.Context, schema, siteID, requestedBy, planHash string, targetDeviceIDs, plan json.RawMessage) (RolloutDraftRecord, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", siteID); err != nil {
		return RolloutDraftRecord{}, err
	}
	var rolloutSequence int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COALESCE(MAX(generation), 0) + 1
		   FROM %s.rollout_runs
		  WHERE site_id = $1`, safeSchema,
	), siteID).Scan(&rolloutSequence); err != nil {
		return RolloutDraftRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(
		"UPDATE %s.rollout_runs SET status = 'STALE', claim_token = NULL, updated_at = CURRENT_TIMESTAMP WHERE site_id = $1 AND status = 'DRAFT' AND generation < $2",
		safeSchema,
	), siteID, rolloutSequence); err != nil {
		return RolloutDraftRecord{}, err
	}

	var id string
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
		INSERT INTO %s.rollout_runs
			(site_id, generation, status, plan_hash, requested_by, target_device_ids, plan)
		VALUES ($1, $2, 'DRAFT', $3, $4, $5, $6)
		RETURNING id::text
	`, safeSchema), siteID, rolloutSequence, planHash, requestedBy, targetDeviceIDs, plan).Scan(&id); err != nil {
		return RolloutDraftRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RolloutDraftRecord{}, err
	}
	return RolloutDraftRecord{
		ID:              id,
		SiteID:          siteID,
		Generation:      rolloutSequence,
		Status:          "DRAFT",
		PlanHash:        planHash,
		RequestedBy:     requestedBy,
		TargetDeviceIDs: targetDeviceIDs,
		Plan:            plan,
		Results:         json.RawMessage("[]"),
	}, nil
}

// GetRolloutDraft loads a rollout draft scoped to one tenant site.
func GetRolloutDraft(ctx context.Context, schema, siteID, rolloutID string) (RolloutDraftRecord, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	var record RolloutDraftRecord
	var claimToken sql.NullString
	err = DB.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT id::text, site_id::text, generation, status, claim_token::text, plan_hash, requested_by,
		       target_device_ids, plan, results
		FROM %s.rollout_runs
		WHERE id = $1 AND site_id = $2
	`, safeSchema), rolloutID, siteID).Scan(
		&record.ID, &record.SiteID, &record.Generation, &record.Status, &claimToken, &record.PlanHash,
		&record.RequestedBy, &record.TargetDeviceIDs, &record.Plan, &record.Results,
	)
	if err == sql.ErrNoRows {
		return RolloutDraftRecord{}, fmt.Errorf("%w: %s", ErrRolloutDraftNotFound, rolloutID)
	}
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	if claimToken.Valid {
		record.ClaimToken = claimToken.String
	}
	return record, nil
}

// ClaimRolloutDraft atomically transitions the newest site DRAFT to RUNNING.
// Claims are serialized per site; expired RUNNING drafts and older DRAFTs are
// marked stale so a crashed or superseded plan cannot be applied later.
func ClaimRolloutDraft(ctx context.Context, schema, siteID, rolloutID string) (RolloutDraftRecord, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", siteID); err != nil {
		return RolloutDraftRecord{}, err
	}
	var record RolloutDraftRecord
	err = tx.QueryRowContext(ctx, fmt.Sprintf(`
		WITH expired AS (
			UPDATE %s.rollout_runs
			SET status = 'STALE', claim_token = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE site_id = $2 AND status = 'RUNNING'
			  AND updated_at < CURRENT_TIMESTAMP - INTERVAL '20 minutes'
			RETURNING site_id
		), expired_devices AS (
			UPDATE %s.devices devices
			SET last_rollout_status = 'FAILED', last_rollout_at = CURRENT_TIMESTAMP
			FROM expired
			WHERE devices.site_id = expired.site_id
			  AND devices.last_rollout_status = 'RUNNING'
		), superseded AS (
			UPDATE %s.rollout_runs stale
			SET status = 'STALE', claim_token = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE stale.site_id = $2 AND stale.status = 'DRAFT'
			  AND EXISTS (
				SELECT 1 FROM %s.rollout_runs newer
				WHERE newer.site_id = $2 AND newer.status = 'DRAFT'
				  AND newer.generation > stale.generation
			  )
		)
		UPDATE %s.rollout_runs candidate
		SET status = 'RUNNING', claim_token = gen_random_uuid(), updated_at = CURRENT_TIMESTAMP
		WHERE candidate.id = $1 AND candidate.site_id = $2 AND candidate.status = 'DRAFT'
		  AND NOT EXISTS (
			SELECT 1 FROM %s.rollout_runs newer
			WHERE newer.site_id = $2 AND newer.status = 'DRAFT'
			  AND newer.generation > candidate.generation
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM %s.rollout_runs active
			WHERE active.site_id = $2 AND active.status = 'RUNNING'
			  AND active.updated_at >= CURRENT_TIMESTAMP - INTERVAL '20 minutes'
		  )
		RETURNING id::text, site_id::text, generation, status, claim_token::text, plan_hash, requested_by,
		          target_device_ids, plan, results
	`, safeSchema, safeSchema, safeSchema, safeSchema, safeSchema, safeSchema, safeSchema), rolloutID, siteID).Scan(
		&record.ID, &record.SiteID, &record.Generation, &record.Status, &record.ClaimToken, &record.PlanHash,
		&record.RequestedBy, &record.TargetDeviceIDs, &record.Plan, &record.Results,
	)
	if err == sql.ErrNoRows {
		return RolloutDraftRecord{}, fmt.Errorf("%w: %s", ErrRolloutDraftNotAvailable, rolloutID)
	}
	if err != nil {
		return RolloutDraftRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return RolloutDraftRecord{}, err
	}
	return record, nil
}

// ReleaseRolloutDraft makes a claimed draft available again when a
// pre-execution dependency, such as transport or backup, is unavailable.
func ReleaseRolloutDraft(ctx context.Context, schema, siteID, rolloutID, claimToken string) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	result, err := DB.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs
		SET status = 'DRAFT', claim_token = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND site_id = $2 AND status = 'RUNNING' AND claim_token::text = $3
	`, safeSchema), rolloutID, siteID, claimToken)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: %s", ErrRolloutDraftNotAvailable, rolloutID)
	}
	return nil
}

// MarkRolloutDraftStale prevents a stale draft from being applied and closes
// any matching device rollout status left behind by the claimed run.
func MarkRolloutDraftStale(ctx context.Context, schema, siteID, rolloutID, claimToken string) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT status
		  FROM %s.rollout_runs
		 WHERE id = $1 AND site_id = $2
		   AND (status = 'DRAFT' OR (status = 'RUNNING' AND claim_token::text = $3))
		 FOR UPDATE
	`, safeSchema), rolloutID, siteID, claimToken).Scan(&previousStatus)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %s", ErrRolloutDraftNotAvailable, rolloutID)
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs
		   SET status = 'STALE', claim_token = NULL, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1 AND site_id = $2
	`, safeSchema), rolloutID, siteID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("%w: %s", ErrRolloutDraftNotAvailable, rolloutID)
	}
	if previousStatus == "RUNNING" {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.devices
		SET last_rollout_status = 'FAILED', last_rollout_at = CURRENT_TIMESTAMP
		WHERE site_id = $1 AND last_rollout_status = 'RUNNING'
	`, safeSchema), siteID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
