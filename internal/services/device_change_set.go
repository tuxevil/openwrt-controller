package services

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ConfirmationLocalAuto lets the enrolled agent commit after local validation
// and health checks pass.
const ConfirmationLocalAuto = "local_auto"

// DeviceChangeSet is the typed, device-delivered unit for a coordinated
// configuration change. Operations are ordered and share one device generation.
type DeviceChangeSet struct {
	ChangeSetID        string                  `json:"change_set_id"`
	DeviceID           string                  `json:"device_id"`
	PlanHash           string                  `json:"plan_hash"`
	Generation         int64                   `json:"generation,omitempty"`
	Operations         []DeviceChangeOperation `json:"operations"`
	HealthChecks       []string                `json:"health_checks,omitempty"`
	ConfirmationPolicy string                  `json:"confirmation_policy"`
}

// DeviceChangeOperation is one ordered, typed namespace operation in a
// DeviceChangeSet.
type DeviceChangeOperation struct {
	OperationID       string       `json:"operation_id"`
	Config            string       `json:"config"`
	Commands          []UciCommand `json:"commands"`
	ObservedStateHash string       `json:"observed_state_hash"`
}

type deviceChangeSetContent struct {
	Operations         []deviceChangeOperationContent `json:"operations"`
	HealthChecks       []string                       `json:"health_checks"`
	ConfirmationPolicy string                         `json:"confirmation_policy"`
}

type deviceChangeOperationContent struct {
	Config            string       `json:"config"`
	Commands          []UciCommand `json:"commands"`
	ObservedStateHash string       `json:"observed_state_hash"`
}

// NewDeviceChangeSet creates one safe changeset attempt. Generation is assigned
// atomically by the database queue and is therefore zero in the returned value.
func NewDeviceChangeSet(deviceID, config string, commands []UciCommand, observedStateHash string, healthChecks []string, confirmationPolicy string) (DeviceChangeSet, error) {
	operationID, err := newDeviceOperationID()
	if err != nil {
		return DeviceChangeSet{}, fmt.Errorf("generate device change operation id: %w", err)
	}
	changeSetID, err := newDeviceChangeSetID()
	if err != nil {
		return DeviceChangeSet{}, fmt.Errorf("generate device change set id: %w", err)
	}
	return newDeviceChangeSetWithIDs(operationID, changeSetID, deviceID, config, commands, observedStateHash, healthChecks, confirmationPolicy)
}

// NewDeviceChangeSetForRollout derives stable identities from the immutable
// rollout identity so a retried apply reuses the same logical work item.
func NewDeviceChangeSetForRollout(rolloutID, deviceID, config string, commands []UciCommand, observedStateHash string, healthChecks []string, confirmationPolicy string) (DeviceChangeSet, error) {
	identitySeed := strings.ToLower(rolloutID) + ":" + strings.ToLower(deviceID)
	operationSeed := sha256.Sum256([]byte("device-operation:" + identitySeed))
	changeSetSeed := sha256.Sum256([]byte("device-change-set:" + identitySeed))
	return newDeviceChangeSetWithIDs(
		"op-"+hex.EncodeToString(operationSeed[:16]),
		"cs-"+hex.EncodeToString(changeSetSeed[:16]),
		deviceID, config, commands, observedStateHash, healthChecks, confirmationPolicy,
	)
}

// NewDeviceChangeSetForRolloutOperations creates stable operation identities
// for an ordered multi-namespace rollout on one device.
func NewDeviceChangeSetForRolloutOperations(rolloutID, deviceID string, operations []DeviceChangeOperation, healthChecks []string, confirmationPolicy string) (DeviceChangeSet, error) {
	identitySeed := strings.ToLower(rolloutID) + ":" + strings.ToLower(deviceID)
	changeSetSeed := sha256.Sum256([]byte("device-change-set:" + identitySeed))
	changeSetID := "cs-" + hex.EncodeToString(changeSetSeed[:16])
	operations = append([]DeviceChangeOperation(nil), operations...)
	for index := range operations {
		operationSeed := sha256.Sum256([]byte(fmt.Sprintf("device-operation:%s:%d:%s", identitySeed, index, operations[index].Config)))
		operations[index].OperationID = "op-" + hex.EncodeToString(operationSeed[:16])
	}
	changeSet := DeviceChangeSet{
		ChangeSetID:        changeSetID,
		DeviceID:           deviceID,
		Operations:         append([]DeviceChangeOperation(nil), operations...),
		HealthChecks:       append([]string(nil), healthChecks...),
		ConfirmationPolicy: confirmationPolicy,
	}
	changeSet.PlanHash = deviceChangeSetPlanHash(changeSet)
	if err := ValidateDeviceChangeSet(changeSet); err != nil {
		return DeviceChangeSet{}, err
	}
	return changeSet, nil
}

func newDeviceChangeSetWithIDs(operationID, changeSetID, deviceID, config string, commands []UciCommand, observedStateHash string, healthChecks []string, confirmationPolicy string) (DeviceChangeSet, error) {
	changeSet := DeviceChangeSet{
		ChangeSetID: changeSetID,
		DeviceID:    deviceID,
		Operations: []DeviceChangeOperation{{
			OperationID:       operationID,
			Config:            config,
			Commands:          append([]UciCommand(nil), commands...),
			ObservedStateHash: observedStateHash,
		}},
		HealthChecks:       append([]string(nil), healthChecks...),
		ConfirmationPolicy: confirmationPolicy,
	}
	changeSet.PlanHash = deviceChangeSetPlanHash(changeSet)
	if err := ValidateDeviceChangeSet(changeSet); err != nil {
		return DeviceChangeSet{}, err
	}
	return changeSet, nil
}

// ValidateDeviceChangeSet accepts the safe multi-namespace contract. Network
// and wireless remain outside this mutation path until controller confirmation.
func ValidateDeviceChangeSet(changeSet DeviceChangeSet) error {
	if !validDeviceChangeSetID(changeSet.ChangeSetID) || !validDeviceOperationPlanHash(changeSet.PlanHash) {
		return fmt.Errorf("invalid device change set identity")
	}
	if !validDeviceChangeSetDeviceID(changeSet.DeviceID) {
		return fmt.Errorf("invalid device change set device")
	}
	if changeSet.Generation < 0 {
		return fmt.Errorf("invalid device change set generation")
	}
	if changeSet.ConfirmationPolicy != ConfirmationLocalAuto {
		return fmt.Errorf("unsupported device change set confirmation policy")
	}
	if len(changeSet.Operations) == 0 || len(changeSet.Operations) > 8 {
		return fmt.Errorf("device change set must contain between one and eight operations")
	}
	if _, err := ValidateHealthTargets(changeSet.HealthChecks); err != nil {
		return err
	}
	seenConfigs := make(map[string]bool, len(changeSet.Operations))
	for _, operation := range changeSet.Operations {
		if !validDeviceOperationID(operation.OperationID) || !deviceChangeSetConfigAllowed(operation.Config) {
			return fmt.Errorf("unsupported device change set namespace")
		}
		if seenConfigs[operation.Config] {
			return fmt.Errorf("device change set contains duplicate namespace %s", operation.Config)
		}
		seenConfigs[operation.Config] = true
		if !validDeviceOperationPlanHash(operation.ObservedStateHash) {
			return fmt.Errorf("invalid observed state hash")
		}
		if err := ValidateDeviceOperation(operation.Config, operation.Commands, changeSet.HealthChecks); err != nil {
			return err
		}
	}
	if deviceChangeSetPlanHash(changeSet) != changeSet.PlanHash {
		return fmt.Errorf("device change set plan hash does not match content")
	}
	return nil
}

func deviceChangeSetConfigAllowed(config string) bool {
	switch config {
	case "system", "dhcp", "firewall", "dropbear", "sqm":
		return true
	default:
		return false
	}
}

func deviceChangeSetPlanHash(changeSet DeviceChangeSet) string {
	operations := make([]deviceChangeOperationContent, 0, len(changeSet.Operations))
	for _, operation := range changeSet.Operations {
		operations = append(operations, deviceChangeOperationContent{
			Config:            operation.Config,
			Commands:          operation.Commands,
			ObservedStateHash: operation.ObservedStateHash,
		})
	}
	canonical := deviceChangeSetContent{
		Operations:         operations,
		HealthChecks:       changeSet.HealthChecks,
		ConfirmationPolicy: changeSet.ConfirmationPolicy,
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

func newDeviceChangeSetID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "cs-" + hex.EncodeToString(bytes), nil
}
