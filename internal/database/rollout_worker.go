package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrRolloutLeaseUnavailable = errors.New("rollout worker lease unavailable")
	ErrRolloutLeaseLost        = errors.New("rollout worker lease lost")
)

// RolloutLease is the fencing identity held by one worker while it advances a
// durable rollout. The token must be supplied to every mutating worker call.
type RolloutLease struct {
	RolloutID string
	SiteID    string
	Token     string
	Cursor    int
}

type RolloutProgress struct {
	Status        string
	TerminalCount int
	ResultCount   int
	Plan          []byte
	Results       []byte
}

// ActiveTenantSchemas returns validated tenant schemas for background work.
func ActiveTenantSchemas(ctx context.Context) ([]string, error) {
	rows, err := DB.QueryContext(ctx, "SELECT schema_alias FROM tenants WHERE is_active = true")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schemas []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		schema, err := SafeTenantSchema(alias)
		if err != nil {
			continue
		}
		schemas = append(schemas, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return schemas, nil
}

// GetRolloutProgress reads only durable execution results. It intentionally
// does not load or render the original SiteConfig.
func GetRolloutProgress(ctx context.Context, schema, rolloutID, siteID string) (RolloutProgress, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return RolloutProgress{}, err
	}
	var progress RolloutProgress
	var raw []byte
	if err := DB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT status, plan, results FROM %s.rollout_runs WHERE id = $1 AND site_id = $2", safeSchema,
	), rolloutID, siteID).Scan(&progress.Status, &progress.Plan, &raw); err != nil {
		return RolloutProgress{}, err
	}
	progress.Results = append([]byte(nil), raw...)
	var results []struct {
		Status         string `json:"status"`
		ChangeSetState string `json:"change_set_state"`
	}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &results); err != nil {
			return RolloutProgress{}, err
		}
	}
	progress.ResultCount = len(results)
	for _, result := range results {
		if result.Status == "SUCCESS" || result.Status == "SKIPPED" ||
			result.ChangeSetState == "COMMITTED" || result.ChangeSetState == "RESTORED" ||
			result.ChangeSetState == "RECOVERY_REQUIRED" || result.ChangeSetState == "REJECTED" {
			progress.TerminalCount++
		}
	}
	return progress, nil
}

// UpdateRolloutWorkerCursor persists progress only for the current lease.
func UpdateRolloutWorkerCursor(ctx context.Context, schema string, lease RolloutLease, cursor int) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	result, err := DB.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs AS candidate
		   SET worker_cursor = GREATEST(worker_cursor, $1), updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND site_id = $3 AND status = 'RUNNING'
		   AND worker_token::text = $4 AND worker_lease_until >= CURRENT_TIMESTAMP
	`, safeSchema), cursor, lease.RolloutID, lease.SiteID, lease.Token)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrRolloutLeaseLost
	}
	return nil
}

// UpdateRolloutWorkerResults atomically replaces mutable execution results
// under the worker fence. The immutable rollout plan is never rewritten.
func UpdateRolloutWorkerResults(ctx context.Context, schema string, lease RolloutLease, results []byte) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	result, err := DB.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs
		   SET results = $1::jsonb, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND site_id = $3 AND status = 'RUNNING'
		   AND worker_token::text = $4 AND worker_lease_until >= CURRENT_TIMESTAMP
	`, safeSchema), results, lease.RolloutID, lease.SiteID, lease.Token)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrRolloutLeaseLost
	}
	return nil
}

// ClaimQueuedRollout claims one queued rollout for leaseDuration. Expired
// leases are reclaimable, so a controller restart does not strand work.
func ClaimQueuedRollout(ctx context.Context, schema string, leaseDuration time.Duration) (RolloutLease, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return RolloutLease{}, err
	}
	if leaseDuration <= 0 {
		return RolloutLease{}, fmt.Errorf("lease duration must be positive")
	}
	var lease RolloutLease
	err = DB.QueryRowContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs
		   SET status = 'RUNNING', worker_token = gen_random_uuid(),
		       worker_lease_until = CURRENT_TIMESTAMP + ($1::double precision * INTERVAL '1 second'),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = (
			SELECT id FROM %s.rollout_runs
				 WHERE (status = 'QUEUED'
			    OR (status = 'RUNNING' AND worker_lease_until < CURRENT_TIMESTAMP))
			   AND NOT EXISTS (
					SELECT 1 FROM %s.rollout_runs active
					 WHERE active.site_id = candidate.site_id
					   AND active.status = 'RUNNING'
					   AND active.worker_lease_until >= CURRENT_TIMESTAMP
				)
			 ORDER BY generation
			 FOR UPDATE SKIP LOCKED LIMIT 1
		 )
		RETURNING id::text, site_id::text, worker_token::text, worker_cursor
	`, safeSchema, safeSchema, safeSchema), leaseDuration.Seconds()).Scan(&lease.RolloutID, &lease.SiteID, &lease.Token, &lease.Cursor)
	if err == sql.ErrNoRows {
		return RolloutLease{}, ErrRolloutLeaseUnavailable
	}
	if err != nil {
		return RolloutLease{}, err
	}
	return lease, nil
}

// RenewRolloutLease extends a lease only when its fencing token is still
// current and the rollout remains RUNNING.
func RenewRolloutLease(ctx context.Context, schema string, lease RolloutLease, leaseDuration time.Duration) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	result, err := DB.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs
		   SET worker_lease_until = CURRENT_TIMESTAMP + ($1::double precision * INTERVAL '1 second'),
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND site_id = $3 AND status = 'RUNNING'
		   AND worker_token::text = $4 AND worker_lease_until >= CURRENT_TIMESTAMP
	`, safeSchema), leaseDuration.Seconds(), lease.RolloutID, lease.SiteID, lease.Token)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrRolloutLeaseLost
	}
	return nil
}
