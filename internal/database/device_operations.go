package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

var (
	// ErrDeviceNotFound indicates that an operation targeted a missing device.
	ErrDeviceNotFound = errors.New("device not found")
	// ErrDeviceOperationGenerationConflict indicates a stale or future revision.
	ErrDeviceOperationGenerationConflict = errors.New("device operation generation conflict")
	// ErrDeviceOperationStatusNotApplied indicates that a status matched no
	// current operation and therefore was not persisted.
	ErrDeviceOperationStatusNotApplied = errors.New("device operation status was not applied")
)

type deviceOperationIdentity struct {
	ID         string
	PlanHash   string
	Generation int64
}

func parseOperationIdentity(raw json.RawMessage, status bool) (deviceOperationIdentity, error) {
	var envelope struct {
		OperationID string `json:"operation_id"`
		ID          string `json:"id"`
		PlanHash    string `json:"plan_hash"`
		Generation  *int64 `json:"generation"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return deviceOperationIdentity{}, fmt.Errorf("invalid device operation envelope")
	}
	identity := deviceOperationIdentity{ID: envelope.OperationID, PlanHash: envelope.PlanHash}
	if status {
		identity.ID = envelope.ID
	}
	if identity.ID == "" || !validDeviceOperationID(identity.ID) {
		return deviceOperationIdentity{}, fmt.Errorf("invalid device operation identity")
	}
	if !status && !validDeviceOperationPlanHash(identity.PlanHash) {
		return deviceOperationIdentity{}, fmt.Errorf("invalid device operation identity")
	}
	if status && identity.PlanHash != "" && !validDeviceOperationPlanHash(identity.PlanHash) {
		return deviceOperationIdentity{}, fmt.Errorf("invalid device operation status")
	}
	if envelope.Generation != nil {
		if *envelope.Generation < 0 {
			return deviceOperationIdentity{}, fmt.Errorf("invalid operation generation")
		}
		identity.Generation = *envelope.Generation
	}
	return identity, nil
}

func parseDeviceOperationEnvelope(plan json.RawMessage) (string, string, error) {
	identity, err := parseOperationIdentity(plan, false)
	if err != nil {
		return "", "", err
	}
	return identity.ID, identity.PlanHash, nil
}

func validDeviceOperationID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') &&
			(char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validDeviceOperationPlanHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, char := range hash {
		if (char < 'A' || char > 'F') && (char < 'a' || char > 'f') &&
			(char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func validDeviceOperationState(state string) bool {
	switch state {
	case "APPLYING", "PENDING_CONFIRM", "ROLLING_BACK", "COMMITTED", "RESTORED", "RECOVERY_REQUIRED":
		return true
	default:
		return false
	}
}

// QueueDeviceOperation stores one durable, device-scoped operation. An
// unbound plan reserves the next device desired generation in the same UPDATE
// that writes pending_operation. A device cannot accept a second operation
// until the first one reaches a terminal state.
func QueueDeviceOperation(ctx context.Context, schema, deviceID string, plan json.RawMessage) (int64, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return 0, err
	}
	identity, err := parseOperationIdentity(plan, false)
	if err != nil {
		return 0, err
	}
	var queuedGeneration int64
	var queuedPlan []byte
	err = Tx(ctx).QueryRow(fmt.Sprintf(`
		UPDATE %s.devices
		SET pending_operation = CASE
		        WHEN $4::bigint > 0 THEN $1::jsonb
		        ELSE jsonb_set($1::jsonb, '{generation}', to_jsonb(desired_generation + 1), true)
		    END,
		    desired_generation = CASE WHEN $4::bigint > 0 THEN desired_generation ELSE desired_generation + 1 END,
		    last_operation = NULL,
		    last_rollout_status = 'QUEUED',
		    last_rollout_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND (
		    ($4::bigint > 0 AND desired_generation = $4::bigint AND (
		        (pending_operation IS NULL AND COALESCE(last_operation->>'id', '') <> $3)
		        OR (pending_operation->>'operation_id' = $3 AND pending_operation = $1::jsonb)
		    ))
		    OR ($4::bigint = 0 AND pending_operation IS NULL AND COALESCE(last_operation->>'id', '') <> $3)
		)
		RETURNING desired_generation, pending_operation
	`, safeSchema), plan, deviceID, identity.ID, identity.Generation).Scan(&queuedGeneration, &queuedPlan)
	if err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
	}
	if err == nil {
		return queuedGeneration, nil
	}

	var pending, last []byte
	var desiredGeneration int64
	lookupErr := Tx(ctx).QueryRow(fmt.Sprintf(
		`SELECT desired_generation, pending_operation, last_operation
			   FROM %s.devices WHERE id = $1`, safeSchema,
	), deviceID).Scan(&desiredGeneration, &pending, &last)
	if lookupErr == sql.ErrNoRows {
		return 0, ErrDeviceNotFound
	}
	if lookupErr != nil {
		return 0, lookupErr
	}
	if len(pending) > 0 {
		pendingIdentity, pendingErr := parseOperationIdentity(pending, false)
		if pendingErr == nil && pendingIdentity.ID == identity.ID {
			if pendingIdentity.PlanHash != identity.PlanHash {
				return 0, fmt.Errorf("operation id already used for a different plan")
			}
			if identity.Generation > 0 && pendingIdentity.Generation != identity.Generation {
				return 0, fmt.Errorf("%w: pending operation is generation %d, requested %d", ErrDeviceOperationGenerationConflict, pendingIdentity.Generation, identity.Generation)
			}
			return pendingIdentity.Generation, nil
		}
		return 0, fmt.Errorf("device already has a different pending operation")
	}
	if len(last) > 0 {
		lastIdentity, lastErr := parseOperationIdentity(last, true)
		if lastErr == nil && lastIdentity.ID == identity.ID {
			if lastIdentity.PlanHash != "" && lastIdentity.PlanHash != identity.PlanHash {
				return 0, fmt.Errorf("operation id already used for a different plan")
			}
			if identity.Generation > 0 && lastIdentity.Generation != identity.Generation {
				return 0, fmt.Errorf("%w: last operation is generation %d, requested %d", ErrDeviceOperationGenerationConflict, lastIdentity.Generation, identity.Generation)
			}
			return lastIdentity.Generation, nil
		}
	}
	if identity.Generation > 0 && desiredGeneration != identity.Generation {
		return 0, fmt.Errorf("%w: device is at generation %d, requested %d", ErrDeviceOperationGenerationConflict, desiredGeneration, identity.Generation)
	}
	return 0, fmt.Errorf("device operation could not be queued")
}

func GetPendingDeviceOperation(ctx context.Context, schema, deviceID string) (json.RawMessage, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = Tx(ctx).QueryRow(fmt.Sprintf(
		"SELECT pending_operation FROM %s.devices WHERE id = $1",
		safeSchema,
	), deviceID).Scan(&raw)
	if err == sql.ErrNoRows || len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// RecordDeviceOperationStatus persists the agent's durable status and clears
// only the matching pending operation after a terminal result.
func RecordDeviceOperationStatus(ctx context.Context, schema, deviceID string, status json.RawMessage) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	identity, err := parseOperationIdentity(status, true)
	if err != nil {
		return fmt.Errorf("invalid device operation status")
	}
	var stateEnvelope struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(status, &stateEnvelope); err != nil || !validDeviceOperationState(stateEnvelope.State) {
		return fmt.Errorf("invalid device operation status")
	}
	terminal := stateEnvelope.State == "COMMITTED" || stateEnvelope.State == "RESTORED"
	generationText := ""
	if identity.Generation > 0 {
		generationText = strconv.FormatInt(identity.Generation, 10)
	}
	pendingMatch := `(pending_operation->>'operation_id' = $3
	    AND (($6 <> '' AND pending_operation->>'plan_hash' = $6)
	         OR ($6 = '' AND COALESCE(pending_operation->>'generation', '') = ''))
	    AND ($7 = '' OR pending_operation->>'generation' = $7))`
	lastMatch := `(last_operation->>'id' = $3
	    AND (($6 <> '' AND last_operation->>'plan_hash' = $6)
	         OR ($6 = '' AND COALESCE(last_operation->>'generation', '') = ''))
	    AND ($7 = '' OR last_operation->>'generation' = $7))`
	result, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.devices
		SET last_operation = $1,
		    pending_operation = CASE
		        WHEN $2 = true AND %s THEN NULL
		        ELSE pending_operation
		    END,
		    last_rollout_status = $4,
		    last_health_check_at = CASE WHEN $2 = true THEN CURRENT_TIMESTAMP ELSE last_health_check_at END,
		    observed_generation = CASE
		        WHEN $2 = true AND $4 = 'COMMITTED' AND $8 > 0
		        THEN GREATEST(observed_generation, $8)
		        ELSE observed_generation
		    END,
		    last_successful_generation = CASE
		        WHEN $2 = true AND $4 = 'COMMITTED' AND $8 > 0
		        THEN GREATEST(last_successful_generation, $8)
		        ELSE last_successful_generation
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $5 AND (
		    %s OR %s
		)
	`, safeSchema, pendingMatch, pendingMatch, lastMatch), status, terminal, identity.ID, stateEnvelope.State, deviceID, identity.PlanHash, generationText, identity.Generation)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: device %s operation %s", ErrDeviceOperationStatusNotApplied, deviceID, identity.ID)
	}
	return nil
}

func GetLastDeviceOperation(ctx context.Context, schema, deviceID string) (json.RawMessage, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = Tx(ctx).QueryRow(fmt.Sprintf(
		"SELECT last_operation FROM %s.devices WHERE id = $1",
		safeSchema,
	), deviceID).Scan(&raw)
	if err == sql.ErrNoRows || len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}
