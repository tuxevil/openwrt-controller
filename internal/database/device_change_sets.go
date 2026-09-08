package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// ErrDeviceChangeSetStatusNotApplied indicates that a status was stale,
// mismatched, or did not target the device's current changeset.
var ErrDeviceChangeSetStatusNotApplied = errors.New("device change set status was not applied")

// ErrDeviceChangeSetCapability indicates that a device cannot execute the
// safe changeset contract advertised by the controller.
var ErrDeviceChangeSetCapability = errors.New("device does not support device changesets")

type deviceChangeSetIdentity struct {
	ID         string
	DeviceID   string
	PlanHash   string
	Generation int64
	HasGen     bool
}

type deviceChangeSetEnvelope struct {
	ChangeSetID        string `json:"change_set_id"`
	DeviceID           string `json:"device_id"`
	PlanHash           string `json:"plan_hash"`
	Generation         *int64 `json:"generation"`
	State              string `json:"state"`
	Failure            string `json:"failure"`
	ConfirmationPolicy string `json:"confirmation_policy"`
	Operations         []struct {
		OperationID       string                   `json:"operation_id"`
		Config            string                   `json:"config"`
		ObservedStateHash string                   `json:"observed_state_hash"`
		Commands          []deviceChangeSetCommand `json:"commands"`
	} `json:"operations"`
	HealthChecks []string `json:"health_checks"`
}

type deviceChangeSetCommand struct {
	Action  string `json:"action"`
	Config  string `json:"config"`
	Section string `json:"section"`
	Option  string `json:"option"`
	Value   string `json:"value"`
}

type deviceChangeSetContent struct {
	Operations         []deviceChangeSetOperationContent `json:"operations"`
	HealthChecks       []string                          `json:"health_checks"`
	ConfirmationPolicy string                            `json:"confirmation_policy"`
}

type deviceChangeSetOperationContent struct {
	Config            string                   `json:"config"`
	Commands          []deviceChangeSetCommand `json:"commands"`
	ObservedStateHash string                   `json:"observed_state_hash"`
}

var (
	deviceChangeSetNamePattern    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	deviceChangeSetSectionPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_-]+|@[A-Za-z0-9_-]+(?:\[-?[0-9]+\])?)$`)
	deviceChangeSetHealthPattern  = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
)

const deviceChangeSetCapabilityContract = `{"version":1,"namespaces":["system"],"max_operations":1,"confirmation_policies":["local_auto"]}`

func parseDeviceChangeSet(raw json.RawMessage, status bool) (deviceChangeSetIdentity, error) {
	var envelope deviceChangeSetEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set")
	}
	if !validDeviceChangeSetID(envelope.ChangeSetID) || !validDeviceChangeSetDeviceID(envelope.DeviceID) ||
		!validDeviceOperationPlanHash(envelope.PlanHash) {
		return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set identity")
	}
	if envelope.Generation != nil && *envelope.Generation < 0 {
		return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set generation")
	}
	if status {
		if envelope.Generation == nil || *envelope.Generation <= 0 || !validDeviceChangeSetState(envelope.State) {
			return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set status")
		}
		return deviceChangeSetIdentity{
			ID:         envelope.ChangeSetID,
			DeviceID:   envelope.DeviceID,
			PlanHash:   envelope.PlanHash,
			Generation: *envelope.Generation,
			HasGen:     true,
		}, nil
	}
	if envelope.Generation != nil && *envelope.Generation == 0 {
		return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set generation")
	}
	if envelope.ConfirmationPolicy != "local_auto" || len(envelope.Operations) != 1 {
		return deviceChangeSetIdentity{}, fmt.Errorf("unsupported device change set")
	}
	operation := envelope.Operations[0]
	if !validDeviceOperationID(operation.OperationID) || operation.Config != "system" ||
		!validDeviceOperationPlanHash(operation.ObservedStateHash) || len(operation.Commands) == 0 {
		return deviceChangeSetIdentity{}, fmt.Errorf("unsupported device change set operation")
	}
	if !validDeviceChangeSetHealthChecks(envelope.HealthChecks) {
		return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set health checks")
	}
	for _, command := range operation.Commands {
		if !validDeviceChangeSetCommand(command) {
			return deviceChangeSetIdentity{}, fmt.Errorf("invalid device change set command")
		}
	}
	if deviceChangeSetPlanHash(envelope) != envelope.PlanHash {
		return deviceChangeSetIdentity{}, fmt.Errorf("device change set plan hash does not match content")
	}
	return deviceChangeSetIdentity{
		ID:         envelope.ChangeSetID,
		DeviceID:   envelope.DeviceID,
		PlanHash:   envelope.PlanHash,
		Generation: valueOrZero(envelope.Generation),
		HasGen:     envelope.Generation != nil,
	}, nil
}

func validDeviceChangeSetHealthChecks(targets []string) bool {
	for _, target := range targets {
		if net.ParseIP(target) == nil && !deviceChangeSetHealthPattern.MatchString(target) {
			return false
		}
	}
	return true
}

func validDeviceChangeSetCommand(command deviceChangeSetCommand) bool {
	if command.Config != "system" {
		return false
	}
	switch command.Action {
	case "set", "delete", "rename":
		if !deviceChangeSetSectionPattern.MatchString(command.Section) {
			return false
		}
		if command.Option != "" && !deviceChangeSetNamePattern.MatchString(command.Option) {
			return false
		}
		return command.Action != "rename" || deviceChangeSetNamePattern.MatchString(command.Value)
	case "add_list", "del_list":
		return deviceChangeSetSectionPattern.MatchString(command.Section) && deviceChangeSetNamePattern.MatchString(command.Option)
	case "add":
		return deviceChangeSetNamePattern.MatchString(command.Value)
	default:
		return false
	}
}

func deviceChangeSetPlanHash(envelope deviceChangeSetEnvelope) string {
	operations := make([]deviceChangeSetOperationContent, 0, len(envelope.Operations))
	for _, operation := range envelope.Operations {
		operations = append(operations, deviceChangeSetOperationContent{
			Config:            operation.Config,
			Commands:          operation.Commands,
			ObservedStateHash: operation.ObservedStateHash,
		})
	}
	canonical := deviceChangeSetContent{
		Operations:         operations,
		HealthChecks:       envelope.HealthChecks,
		ConfirmationPolicy: envelope.ConfirmationPolicy,
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(canonical); err != nil {
		return ""
	}
	raw := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func validDeviceChangeSetID(id string) bool {
	return validDeviceOperationID(id)
}

func validDeviceChangeSetDeviceID(id string) bool {
	if id == "" || len(id) > 50 {
		return false
	}
	for _, char := range id {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') &&
			(char < '0' || char > '9') && char != ':' && char != '.' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validDeviceChangeSetState(state string) bool {
	switch state {
	case "PREPARED", "APPLYING", "PENDING_CONFIRM", "ROLLING_BACK", "COMMITTED", "RESTORED", "RECOVERY_REQUIRED", "REJECTED":
		return true
	default:
		return false
	}
}

// QueueDeviceChangeSet persists a validated changeset and atomically reserves
// its device-local generation while fencing other pending work.
func QueueDeviceChangeSet(ctx context.Context, schema, deviceID string, changeSet json.RawMessage) (int64, error) {
	return queueDeviceChangeSet(ctx, schema, "", "", deviceID, changeSet, "")
}

// QueueDeviceChangeSetForRollout allows the rollout that owns the device lease
// to enqueue its changeset while continuing to fence every other active run.
func QueueDeviceChangeSetForRollout(ctx context.Context, schema, siteID, deviceRole, deviceID string, changeSet json.RawMessage, rolloutID string) (int64, error) {
	return queueDeviceChangeSet(ctx, schema, siteID, deviceRole, deviceID, changeSet, rolloutID)
}

func queueDeviceChangeSet(ctx context.Context, schema, ownerSiteID, ownerRole, deviceID string, changeSet json.RawMessage, ownerRolloutID string) (int64, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return 0, err
	}
	identity, err := parseDeviceChangeSet(changeSet, false)
	if err != nil {
		return 0, err
	}
	if !strings.EqualFold(identity.DeviceID, deviceID) {
		return 0, fmt.Errorf("device change set targets a different device")
	}
	requestedGeneration := identity.Generation
	var queuedGeneration int64
	var queuedChangeSet []byte
	ownerConstraints := ""
	activeRolloutExclusion := ""
	args := []interface{}{changeSet, deviceID, identity.ID, requestedGeneration}
	if ownerRolloutID != "" {
		ownerConstraints = fmt.Sprintf(`
		   AND LOWER(device.site_id::text) = LOWER($5)
		   AND EXISTS (
		       SELECT 1
		         FROM %s.rollout_runs AS owner_rollout
			        WHERE LOWER(owner_rollout.id::text) = LOWER($6::text)
		          AND owner_rollout.site_id = device.site_id
		          AND owner_rollout.status = 'RUNNING'
		   )
		   AND COALESCE(device.device_role, 'AP') = $7
		`, safeSchema)
		activeRolloutExclusion = `
		   AND LOWER(active_rollout.id::text) <> LOWER($6::text)`
		args = append(args, ownerSiteID, ownerRolloutID, ownerRole)
	}
	err = Tx(ctx).QueryRowContext(ctx, fmt.Sprintf(`
		UPDATE %s.devices AS device
		   SET pending_change_set = CASE
		           WHEN $4::bigint > 0 THEN $1::jsonb
		           ELSE jsonb_set($1::jsonb, '{generation}', to_jsonb(device.desired_generation + 1), true)
		       END,
		       desired_generation = CASE
		           WHEN $4::bigint > 0 THEN device.desired_generation
		           ELSE device.desired_generation + 1
		       END,
		       last_rollout_status = 'QUEUED',
		       last_rollout_at = CURRENT_TIMESTAMP,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE device.id = $2
		   %s
		   AND COALESCE(device.capabilities->'device_change_set' = '%s'::jsonb, false)
		   AND COALESCE(device.capabilities->'device_change_set' @> '%s'::jsonb, false)
		   AND device.pending_operation IS NULL
		   AND device.pending_change_set IS NULL
		   AND COALESCE(device.last_operation->>'state', '') <> 'RECOVERY_REQUIRED'
		   AND COALESCE(device.last_change_set->>'state', '') <> 'RECOVERY_REQUIRED'
		   AND COALESCE(device.last_change_set->>'change_set_id', '') <> $3
		   AND COALESCE(device.last_rollout_status, '') <> 'RUNNING'
		   AND NOT EXISTS (
		       SELECT 1
		         FROM %s.rollout_runs AS active_rollout
		        WHERE active_rollout.site_id = device.site_id
		          AND active_rollout.status = 'RUNNING'
		          AND active_rollout.updated_at >= CURRENT_TIMESTAMP - INTERVAL '20 minutes'
		          %s
		   )
		   AND ($4::bigint = 0 OR device.desired_generation = $4::bigint)
		 RETURNING device.desired_generation, device.pending_change_set
	`, safeSchema, ownerConstraints, deviceChangeSetCapabilityContract, deviceChangeSetCapabilityContract, safeSchema, activeRolloutExclusion), args...).Scan(&queuedGeneration, &queuedChangeSet)
	if err == nil {
		return queuedGeneration, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	var deviceSite, actualRole string
	var changeSetCapability bool
	var pending, last []byte
	var desiredGeneration int64
	lookupErr := Tx(ctx).QueryRowContext(ctx, fmt.Sprintf(
		`SELECT site_id::text, COALESCE(device_role, 'AP'), COALESCE(capabilities->'device_change_set' = '%s'::jsonb, false), desired_generation, pending_change_set, last_change_set
		   FROM %s.devices WHERE id = $1`, deviceChangeSetCapabilityContract, safeSchema,
	), deviceID).Scan(&deviceSite, &actualRole, &changeSetCapability, &desiredGeneration, &pending, &last)
	if lookupErr == sql.ErrNoRows {
		return 0, ErrDeviceNotFound
	}
	if lookupErr != nil {
		return 0, lookupErr
	}
	if ownerRolloutID != "" && !strings.EqualFold(deviceSite, ownerSiteID) {
		return 0, fmt.Errorf("device is not in the owning rollout site")
	}
	if ownerRolloutID != "" && actualRole != ownerRole {
		return 0, fmt.Errorf("device role is no longer current")
	}
	if !changeSetCapability {
		return 0, ErrDeviceChangeSetCapability
	}
	if len(last) > 0 && string(last) != "null" {
		var lastStatus struct {
			State string `json:"state"`
		}
		if json.Unmarshal(last, &lastStatus) == nil && lastStatus.State == "RECOVERY_REQUIRED" {
			return 0, fmt.Errorf("device requires recovery before accepting changesets")
		}
	}
	if len(pending) > 0 && string(pending) != "null" {
		pendingIdentity, pendingErr := parseDeviceChangeSet(pending, false)
		if pendingErr == nil && pendingIdentity.ID == identity.ID {
			if pendingIdentity.PlanHash != identity.PlanHash {
				return 0, fmt.Errorf("change set id already used for a different plan")
			}
			if identity.HasGen && pendingIdentity.Generation != identity.Generation {
				return 0, fmt.Errorf("%w: pending change set is generation %d, requested %d", ErrDeviceOperationGenerationConflict, pendingIdentity.Generation, identity.Generation)
			}
			return pendingIdentity.Generation, nil
		}
		return 0, fmt.Errorf("device already has a different pending change set")
	}
	if len(last) > 0 && string(last) != "null" {
		lastIdentity, lastErr := parseDeviceChangeSet(last, true)
		if lastErr == nil && lastIdentity.ID == identity.ID {
			if lastIdentity.PlanHash != identity.PlanHash {
				return 0, fmt.Errorf("change set id already used for a different plan")
			}
			if identity.HasGen && lastIdentity.Generation != identity.Generation {
				return 0, fmt.Errorf("%w: last change set is generation %d, requested %d", ErrDeviceOperationGenerationConflict, lastIdentity.Generation, identity.Generation)
			}
			return lastIdentity.Generation, nil
		}
	}
	if identity.HasGen && desiredGeneration != identity.Generation {
		return 0, fmt.Errorf("%w: device is at generation %d, requested %d", ErrDeviceOperationGenerationConflict, desiredGeneration, identity.Generation)
	}
	return 0, fmt.Errorf("device change set could not be queued")
}

// GetPendingDeviceChangeSet returns the immutable changeset awaiting delivery,
// or nil when the device has no pending changeset.
func GetPendingDeviceChangeSet(ctx context.Context, schema, deviceID string) (json.RawMessage, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = Tx(ctx).QueryRowContext(ctx, fmt.Sprintf(
		"SELECT pending_change_set FROM %s.devices WHERE id = $1", safeSchema,
	), deviceID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// GetLastDeviceChangeSet returns the most recently recorded changeset status,
// or nil when the device has no changeset history.
func GetLastDeviceChangeSet(ctx context.Context, schema, deviceID string) (json.RawMessage, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = Tx(ctx).QueryRowContext(ctx, fmt.Sprintf(
		"SELECT last_change_set FROM %s.devices WHERE id = $1", safeSchema,
	), deviceID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

// RecordDeviceChangeSetStatus applies a matching changeset status and ignores
// stale or cross-device reports without mutating device state.
func RecordDeviceChangeSetStatus(ctx context.Context, schema, deviceID string, status json.RawMessage) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	identity, err := parseDeviceChangeSet(status, true)
	if err != nil {
		return ErrDeviceChangeSetStatusNotApplied
	}
	if !strings.EqualFold(identity.DeviceID, deviceID) {
		return ErrDeviceChangeSetStatusNotApplied
	}
	var envelope deviceChangeSetEnvelope
	if err := json.Unmarshal(status, &envelope); err != nil {
		return ErrDeviceChangeSetStatusNotApplied
	}
	terminal := envelope.State == "COMMITTED" || envelope.State == "RESTORED" || envelope.State == "RECOVERY_REQUIRED" || envelope.State == "REJECTED"
	generationText := strconv.FormatInt(identity.Generation, 10)
	pendingMatch := `(pending_operation IS NULL
	    AND pending_change_set->>'change_set_id' = $3::text
	    AND pending_change_set->>'device_id' = $5::text
	    AND pending_change_set->>'plan_hash' = $6::text
	    AND pending_change_set->>'generation' = $7::text)`
	lastMatch := `(pending_operation IS NULL
	    AND pending_change_set IS NULL
	    AND last_change_set->>'change_set_id' = $3::text
	    AND last_change_set->>'device_id' = $5::text
	    AND last_change_set->>'plan_hash' = $6::text
	    AND last_change_set->>'generation' = $7::text
	    AND (COALESCE(last_change_set->>'state', '') NOT IN ('COMMITTED', 'RESTORED', 'RECOVERY_REQUIRED', 'REJECTED')
	         OR last_change_set->>'state' = $4::text))`
	statusTransition := `(COALESCE(last_change_set->>'change_set_id', '') <> $3::text
	    OR CASE $4::text
	         WHEN 'PREPARED' THEN COALESCE(last_change_set->>'state', '') IN ('', 'PREPARED')
	         WHEN 'APPLYING' THEN COALESCE(last_change_set->>'state', '') IN ('PREPARED', 'APPLYING')
	         WHEN 'PENDING_CONFIRM' THEN COALESCE(last_change_set->>'state', '') IN ('APPLYING', 'PENDING_CONFIRM')
	         WHEN 'ROLLING_BACK' THEN COALESCE(last_change_set->>'state', '') IN ('PREPARED', 'APPLYING', 'PENDING_CONFIRM', 'ROLLING_BACK')
	         WHEN 'COMMITTED' THEN COALESCE(last_change_set->>'state', '') IN ('APPLYING', 'PENDING_CONFIRM', 'COMMITTED')
	         WHEN 'RESTORED' THEN COALESCE(last_change_set->>'state', '') IN ('ROLLING_BACK', 'RESTORED')
	         WHEN 'RECOVERY_REQUIRED' THEN COALESCE(last_change_set->>'state', '') IN ('PREPARED', 'APPLYING', 'PENDING_CONFIRM', 'ROLLING_BACK', 'RECOVERY_REQUIRED')
	         WHEN 'REJECTED' THEN COALESCE(last_change_set->>'state', '') IN ('', 'PREPARED', 'APPLYING', 'REJECTED')
	         ELSE false
	       END)`
	pendingMatch += " AND " + statusTransition
	lastMatch += " AND " + statusTransition
	result, err := Tx(ctx).ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.devices
		   SET last_change_set = $1,
		       pending_change_set = CASE WHEN $2::boolean = true AND %s THEN NULL ELSE pending_change_set END,
		       last_rollout_status = CASE WHEN COALESCE(last_rollout_status, '') = 'RUNNING' THEN last_rollout_status ELSE $4::text END,
		       last_rollout_at = CURRENT_TIMESTAMP,
	       last_health_check_at = CASE WHEN $2::boolean = true THEN CURRENT_TIMESTAMP ELSE last_health_check_at END,
		       observed_generation = CASE
		           WHEN $2::boolean = true AND $4::text = 'COMMITTED'
		           THEN GREATEST(observed_generation, $8::bigint)
		           ELSE observed_generation
		       END,
		       last_successful_generation = CASE
		           WHEN $2::boolean = true AND $4::text = 'COMMITTED'
		           THEN GREATEST(last_successful_generation, $8::bigint)
		           ELSE last_successful_generation
		       END,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE id = $5 AND (%s OR %s)
	`, safeSchema, pendingMatch, pendingMatch, lastMatch), status, terminal, identity.ID, envelope.State, deviceID, identity.PlanHash, generationText, identity.Generation)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: device %s changeset %s", ErrDeviceChangeSetStatusNotApplied, deviceID, identity.ID)
	}
	if terminal {
		rolloutStatus := "failed"
		resultStatus := "FAILED"
		if envelope.State == "COMMITTED" {
			rolloutStatus = "completed"
			resultStatus = "SUCCESS"
		}
		if _, err := Tx(ctx).ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.rollout_runs
		   SET status = CASE
		           WHEN EXISTS (
		               SELECT 1 FROM jsonb_array_elements(COALESCE(results, '[]'::jsonb)) AS pending(result)
		               WHERE NOT (pending.result->>'device_id' = $2::text AND pending.result->>'change_set_id' = $3::text)
		                 AND COALESCE(pending.result->>'change_set_state', '') NOT IN ('COMMITTED', 'RESTORED', 'RECOVERY_REQUIRED', 'REJECTED')
		           ) THEN 'QUEUED'
		           WHEN EXISTS (
		               SELECT 1 FROM jsonb_array_elements(COALESCE(results, '[]'::jsonb)) AS failed(result)
		               WHERE NOT (failed.result->>'device_id' = $2::text AND failed.result->>'change_set_id' = $3::text)
		                 AND COALESCE(failed.result->>'change_set_state', '') <> 'COMMITTED'
		           ) THEN 'failed'
		           ELSE $1::text
		       END,
			       results = (
			           SELECT COALESCE(jsonb_agg(
			               CASE WHEN result->>'device_id' = $2 AND result->>'change_set_id' = $3
			                    THEN result || jsonb_build_object(
			                             'status', $4::text,
			                             'change_set_state', $5::text,
			                             'plan_hash', $6::text,
			                             'device_generation', $7::bigint
			                         ) || CASE WHEN $8::text <> '' THEN jsonb_build_object('error', $8::text) ELSE '{}'::jsonb END
			                    ELSE result
			               END ORDER BY ordinal
			           ), '[]'::jsonb)
			             FROM jsonb_array_elements(COALESCE(results, '[]'::jsonb)) WITH ORDINALITY AS entries(result, ordinal)
			       ),
			       updated_at = CURRENT_TIMESTAMP
			 WHERE status = 'QUEUED'
			   AND site_id = (SELECT site_id FROM %s.devices WHERE id = $2)
			   AND results @> jsonb_build_array(jsonb_build_object('device_id', $2::text, 'change_set_id', $3::text))
		`, safeSchema, safeSchema), rolloutStatus, deviceID, identity.ID, resultStatus, envelope.State, identity.PlanHash, identity.Generation, envelope.Failure); err != nil {
			return err
		}
	}
	return nil
}
