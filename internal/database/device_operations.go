package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func parseDeviceOperationEnvelope(plan json.RawMessage) (string, error) {
	var envelope struct {
		OperationID string `json:"operation_id"`
		PlanHash    string `json:"plan_hash"`
	}
	if err := json.Unmarshal(plan, &envelope); err != nil {
		return "", fmt.Errorf("invalid device operation envelope")
	}
	if !validDeviceOperationID(envelope.OperationID) || !validDeviceOperationID(envelope.PlanHash) || envelope.OperationID != envelope.PlanHash {
		return "", fmt.Errorf("invalid device operation identity")
	}
	return envelope.OperationID, nil
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

func validDeviceOperationState(state string) bool {
	switch state {
	case "APPLYING", "PENDING_CONFIRM", "ROLLING_BACK", "COMMITTED", "RESTORED", "RECOVERY_REQUIRED":
		return true
	default:
		return false
	}
}

// QueueDeviceOperation stores one durable, device-scoped operation. A device
// cannot accept a second operation until the first one reaches a terminal
// state, which is the controller-side serialization half of the executor.
func QueueDeviceOperation(ctx context.Context, schema, deviceID string, plan json.RawMessage) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	operationID, err := parseDeviceOperationEnvelope(plan)
	if err != nil {
		return err
	}
	result, err := Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.devices
		SET pending_operation = $1,
		    last_operation = NULL,
		    last_rollout_status = 'QUEUED',
		    last_rollout_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $2 AND (
		    (pending_operation IS NULL AND COALESCE(last_operation->>'id', '') <> $3)
		    OR (pending_operation->>'operation_id' = $3 AND pending_operation = $1::jsonb)
		)
	`, safeSchema), plan, deviceID, operationID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		var pending, last []byte
		var samePending bool
		lookupErr := Tx(ctx).QueryRow(fmt.Sprintf(
			`SELECT pending_operation, last_operation,
			        COALESCE(pending_operation = $1::jsonb, false)
			   FROM %s.devices WHERE id = $2`, safeSchema,
		), plan, deviceID).Scan(&pending, &last, &samePending)
		if lookupErr == sql.ErrNoRows {
			return fmt.Errorf("device not found")
		}
		if lookupErr != nil {
			return lookupErr
		}
		var pendingEnvelope struct {
			OperationID string `json:"operation_id"`
		}
		if len(pending) > 0 && json.Unmarshal(pending, &pendingEnvelope) == nil && pendingEnvelope.OperationID != "" {
			if pendingEnvelope.OperationID == operationID && samePending {
				return nil
			}
			return fmt.Errorf("device already has a different pending operation")
		}
		var lastEnvelope struct {
			ID string `json:"id"`
		}
		if len(last) > 0 && json.Unmarshal(last, &lastEnvelope) == nil && lastEnvelope.ID == operationID {
			return nil
		}
		return fmt.Errorf("device already has a pending operation")
	}
	return nil
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
	var envelope struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(status, &envelope); err != nil || !validDeviceOperationID(envelope.ID) || !validDeviceOperationState(envelope.State) {
		return fmt.Errorf("invalid device operation status")
	}
	terminal := envelope.State == "COMMITTED" || envelope.State == "RESTORED"
	_, err = Tx(ctx).Exec(fmt.Sprintf(`
		UPDATE %s.devices
		SET last_operation = $1,
		    pending_operation = CASE
		        WHEN $2 = true AND pending_operation->>'operation_id' = $3 THEN NULL
		        ELSE pending_operation
		    END,
		    last_rollout_status = $4,
		    last_health_check_at = CASE WHEN $2 = true THEN CURRENT_TIMESTAMP ELSE last_health_check_at END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $5 AND (
		    pending_operation->>'operation_id' = $3
		    OR last_operation->>'id' = $3
		    OR (pending_operation IS NULL AND last_operation IS NULL)
		)
	`, safeSchema), status, terminal, envelope.ID, envelope.State, deviceID)
	return err
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
