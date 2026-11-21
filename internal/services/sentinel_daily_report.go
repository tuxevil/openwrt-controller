package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"openwrt-controller/internal/database"
)

func StartSentinelDailyReports(stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			generateSentinelDailyReports()
			select {
			case <-ticker.C:
			case <-stop:
				return
			}
		}
	}()
}

func generateSentinelDailyReports() {
	tenants, err := ListTenants()
	if err != nil {
		slog.Warn("Sentinel daily report tenant list failed", "error", err)
		return
	}
	for _, tenant := range tenants {
		schema, err := database.SafeTenantSchema(tenant.SchemaAlias)
		if err != nil {
			continue
		}
		if err := generateSentinelDailyReport(schema); err != nil {
			slog.Warn("Sentinel daily report failed", "tenant", tenant.SchemaAlias, "error", err)
		}
	}
}

func generateSentinelDailyReport(schema string) error {
	quoted, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err = database.DB.ExecContext(ctx, fmt.Sprintf(`
		WITH day_window AS (
			SELECT date_trunc('day', CURRENT_TIMESTAMP AT TIME ZONE 'UTC') AS end_utc
		), bounds AS (
			SELECT (end_utc - INTERVAL '1 day')::date AS day,
			       (end_utc - INTERVAL '1 day') AT TIME ZONE 'UTC' AS since,
			       end_utc AT TIME ZONE 'UTC' AS until
			FROM day_window
		)
		INSERT INTO %[1]s.sentinel_daily_reports (report_day, generated_at, opened, resolved, devices, fresh_devices)
		SELECT day, CURRENT_TIMESTAMP,
		       (SELECT count(*) FROM %[1]s.incidents WHERE created_at >= since AND created_at < until),
		       (SELECT count(*) FROM %[1]s.incidents WHERE resolved_at >= since AND resolved_at < until),
		       (SELECT count(*) FROM %[1]s.devices),
		       (SELECT count(*) FROM %[1]s.devices WHERE last_seen_at > CURRENT_TIMESTAMP - INTERVAL '2 minutes')
		FROM bounds
		WHERE NOT EXISTS (SELECT 1 FROM %[1]s.sentinel_daily_reports WHERE report_day = day)
		ON CONFLICT (report_day) DO NOTHING
	`, quoted))
	return err
}
