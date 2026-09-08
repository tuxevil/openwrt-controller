package rolloutworker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

const (
	defaultInterval = 5 * time.Second
	defaultLease    = 30 * time.Second
)

// Worker reconciles durable changeset rollouts. Devices pull their queued
// changesets independently; this process owns only controller-side progress.
type Worker struct {
	Interval      time.Duration
	LeaseDuration time.Duration
	Logger        *slog.Logger
}

type rolloutPlan struct {
	HealthChecks []string        `json:"health_checks"`
	Namespace    string          `json:"namespace"`
	Devices      []rolloutDevice `json:"devices"`
}

type rolloutDevice struct {
	DeviceID      string                `json:"device_id"`
	Hostname      string                `json:"hostname"`
	Role          string                `json:"role"`
	Commands      []services.UciCommand `json:"commands"`
	ObservedState map[string]string     `json:"observed_state"`
}

func (w Worker) Start(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	leaseDuration := w.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = defaultLease
	}
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}

	go func() {
		w.reconcile(ctx, leaseDuration, logger)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.reconcile(ctx, leaseDuration, logger)
			}
		}
	}()
}

func (w Worker) reconcile(ctx context.Context, leaseDuration time.Duration, logger *slog.Logger) {
	schemas, err := database.ActiveTenantSchemas(ctx)
	if err != nil {
		logger.Warn("rollout worker could not list tenants", "err", err)
		return
	}
	for _, schema := range schemas {
		lease, err := database.ClaimQueuedRollout(ctx, schema, leaseDuration)
		if errors.Is(err, database.ErrRolloutLeaseUnavailable) {
			continue
		}
		if err != nil {
			logger.Warn("rollout worker could not claim rollout", "schema", schema, "err", err)
			continue
		}
		go w.reconcileLease(ctx, schema, lease, leaseDuration, logger)
	}
}

func (w Worker) reconcileLease(ctx context.Context, schema string, lease database.RolloutLease, leaseDuration time.Duration, logger *slog.Logger) {
	interval := leaseDuration / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		progress, err := database.GetRolloutProgress(ctx, schema, lease.RolloutID, lease.SiteID)
		if err != nil {
			logger.Warn("rollout worker could not read progress", "rollout_id", lease.RolloutID, "err", err)
			return
		}
		if progress.TerminalCount > lease.Cursor {
			if err := database.UpdateRolloutWorkerCursor(ctx, schema, lease, progress.TerminalCount); err != nil {
				logger.Warn("rollout worker lost lease while updating cursor", "rollout_id", lease.RolloutID, "err", err)
				return
			}
			lease.Cursor = progress.TerminalCount
		}
		if lease.Cursor > 0 && progress.Status != "FAILED" && progress.Status != "failed" {
			if err := w.queueNextPhase(ctx, schema, lease, progress); err != nil {
				logger.Warn("rollout worker could not queue next phase", "rollout_id", lease.RolloutID, "err", err)
				return
			}
		}
		if progress.Status != "RUNNING" || progress.ResultCount > 0 && progress.TerminalCount == progress.ResultCount {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := database.RenewRolloutLease(ctx, schema, lease, leaseDuration); err != nil {
				logger.Warn("rollout worker lease renewal failed", "rollout_id", lease.RolloutID, "err", err)
				return
			}
		}
	}
}

func (w Worker) queueNextPhase(ctx context.Context, schema string, lease database.RolloutLease, progress database.RolloutProgress) error {
	var plan rolloutPlan
	if err := json.Unmarshal(progress.Plan, &plan); err != nil {
		return err
	}
	var results []map[string]interface{}
	if err := json.Unmarshal(progress.Results, &results); err != nil {
		return err
	}
	order := append([]rolloutDevice(nil), plan.Devices...)
	sort.SliceStable(order, func(i, j int) bool {
		leftGateway, rightGateway := strings.EqualFold(order[i].Role, "Gateway"), strings.EqualFold(order[j].Role, "Gateway")
		if leftGateway != rightGateway {
			return !leftGateway
		}
		return order[i].DeviceID < order[j].DeviceID
	})
	if lease.Cursor >= len(order) {
		return nil
	}
	device := order[lease.Cursor]
	for _, result := range results {
		if result["device_id"] == device.DeviceID && result["status"] != "WAITING" {
			return nil
		}
	}
	namespaces := map[string]bool{}
	for _, namespace := range strings.Split(plan.Namespace, ",") {
		namespace = strings.TrimSpace(namespace)
		if namespace == "system" || namespace == "dhcp" || namespace == "firewall" || namespace == "dropbear" || namespace == "sqm" {
			namespaces[namespace] = true
		}
	}
	byNamespace := map[string][]services.UciCommand{}
	for _, command := range device.Commands {
		if !namespaces[command.Config] {
			return nil
		}
		byNamespace[command.Config] = append(byNamespace[command.Config], command)
	}
	operations := make([]services.DeviceChangeOperation, 0, len(byNamespace))
	for _, namespace := range []string{"system", "dhcp", "firewall", "dropbear", "sqm"} {
		commands := byNamespace[namespace]
		if len(commands) == 0 {
			continue
		}
		observed := device.ObservedState[namespace]
		if observed == "" {
			return nil
		}
		operations = append(operations, services.DeviceChangeOperation{Config: namespace, Commands: commands, ObservedStateHash: observed})
	}
	changeSet, err := services.NewDeviceChangeSetForRolloutOperations(lease.RolloutID, device.DeviceID, operations, plan.HealthChecks, services.ConfirmationLocalAuto)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(changeSet)
	if err != nil {
		return err
	}
	generation, err := database.QueueDeviceChangeSetForRollout(ctx, schema, lease.SiteID, device.Role, device.DeviceID, encoded, lease.RolloutID)
	if err != nil {
		return err
	}
	for _, result := range results {
		if result["device_id"] == device.DeviceID {
			result["status"] = "QUEUED"
			result["output"] = "device agent will apply and report the durable changeset result"
			result["device_generation"] = generation
		}
	}
	encodedResults, err := json.Marshal(results)
	if err != nil {
		return err
	}
	return database.UpdateRolloutWorkerResults(ctx, schema, lease, encodedResults)
}
