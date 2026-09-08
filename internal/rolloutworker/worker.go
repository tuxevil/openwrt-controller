package rolloutworker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"openwrt-controller/internal/database"
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
