package services

import (
	"context"
	"log"
	"time"

	"openwrt-controller/internal/database"
)

// LogRetentionDays is the system_logs retention window. Anything older
// is purged by the background sweeper. Keep this small enough that the
// table stays under ~1 GB for a typical fleet; large windows just
// bloat the trigram index and slow down /api/sites/{id}/logs.
const LogRetentionDays = 7

// StartLogRetentionCron launches a background goroutine that purges old
// system_logs rows. Idempotent; safe to call once from main. Stops on
// the supplied stopCh.
func StartLogRetentionCron(stopCh <-chan struct{}) {
	go func() {
		// Run once on boot to clean any backlog from previous crashes.
		sweepOnce()

		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				sweepOnce()
			}
		}
	}()
}

func sweepOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	out, err := database.SweepAllOldLogs(ctx, LogRetentionDays)
	if err != nil {
		log.Printf("[LOG_RETENTION] sweep failed: %v", err)
		return
	}
	total := int64(0)
	for _, n := range out {
		total += n
	}
	if total > 0 {
		log.Printf("[LOG_RETENTION] purged %d system_logs rows older than %d days across %d tenant(s)", total, LogRetentionDays, len(out))
	}
}
