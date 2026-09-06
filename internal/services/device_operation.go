package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// DeviceOperationPlan is the typed, device-delivered unit of configuration
// change. operation_id identifies one attempt; plan_hash identifies the
// immutable command content and may be reused by a later reconciliation.
type DeviceOperationPlan struct {
	OperationID  string       `json:"operation_id"`
	PlanHash     string       `json:"plan_hash"`
	Config       string       `json:"config"`
	Commands     []UciCommand `json:"commands"`
	HealthChecks []string     `json:"health_checks,omitempty"`
	AutoConfirm  bool         `json:"auto_confirm"`
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
	if len(hash) != sha256.Size*2 {
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

func ValidateDeviceOperationPlan(plan DeviceOperationPlan) error {
	if !validDeviceOperationID(plan.OperationID) || !validDeviceOperationPlanHash(plan.PlanHash) {
		return fmt.Errorf("invalid operation identity")
	}
	return ValidateDeviceOperation(plan.Config, plan.Commands, plan.HealthChecks)
}

var deviceOperationConfigs = map[string]struct{}{
	"wireless": {},
	"network":  {},
	"dhcp":     {},
	"firewall": {},
	"dropbear": {},
	"system":   {},
	"sqm":      {},
}

// NewDeviceOperationPlan validates the small typed command grammar supported
// by the local executor and derives a stable content hash plus a unique
// idempotency key for this attempt.
func NewDeviceOperationPlan(config string, commands []UciCommand, healthChecks []string, autoConfirm bool) (DeviceOperationPlan, error) {
	if err := ValidateDeviceOperation(config, commands, healthChecks); err != nil {
		return DeviceOperationPlan{}, err
	}

	canonical := struct {
		Config       string       `json:"config"`
		Commands     []UciCommand `json:"commands"`
		HealthChecks []string     `json:"health_checks"`
		AutoConfirm  bool         `json:"auto_confirm"`
	}{config, commands, healthChecks, autoConfirm}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return DeviceOperationPlan{}, fmt.Errorf("marshal device operation: %w", err)
	}
	hash := sha256.Sum256(raw)
	hashString := hex.EncodeToString(hash[:])
	operationID, err := newDeviceOperationID()
	if err != nil {
		return DeviceOperationPlan{}, fmt.Errorf("generate device operation id: %w", err)
	}
	return DeviceOperationPlan{
		OperationID:  operationID,
		PlanHash:     hashString,
		Config:       config,
		Commands:     commands,
		HealthChecks: append([]string(nil), healthChecks...),
		AutoConfirm:  autoConfirm,
	}, nil
}

func newDeviceOperationID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "op-" + hex.EncodeToString(bytes), nil
}

// ValidateDeviceOperation accepts only commands that can be executed by the
// local typed executor. Arbitrary shell snippets remain on the legacy path.
func ValidateDeviceOperation(config string, commands []UciCommand, healthChecks []string) error {
	if _, ok := deviceOperationConfigs[config]; !ok {
		return fmt.Errorf("invalid UCI config namespace")
	}
	if len(commands) == 0 {
		return fmt.Errorf("empty device operation")
	}
	if config == "network" && len(healthChecks) == 0 {
		return fmt.Errorf("network operations require at least one health check")
	}
	if _, err := ValidateHealthTargets(healthChecks); err != nil {
		return err
	}
	for _, command := range commands {
		if command.Config != config {
			return fmt.Errorf("operation command crosses config namespace")
		}
		switch command.Action {
		case "set", "delete", "add_list", "del_list", "add", "rename":
		case "ensure_host":
			if config != "dhcp" {
				return fmt.Errorf("ensure_host is only valid for DHCP operations")
			}
		case "delete_all":
			if config != "firewall" || command.Section != "redirect" || command.Option != "" || command.Value != "" {
				return fmt.Errorf("delete_all is only valid for firewall redirects")
			}
		default:
			return fmt.Errorf("unsupported device operation action %q", command.Action)
		}
		if translateCommand(command) == "" {
			return fmt.Errorf("invalid UCI command")
		}
	}
	return nil
}
