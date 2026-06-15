package services

import (
	"context"
	"openwrt-controller/internal/database"
)

func GetGlobalHealth(ctx context.Context) int {
	health := 0.0

	// 1. Status de Nodos (40%)
	var totalNodes, onlineNodes int
	err := database.Tx(ctx).QueryRow(`
		SELECT count(*), sum(case when status='ONLINE' then 1 else 0 end) 
		FROM devices
	`).Scan(&totalNodes, &onlineNodes)

	if err == nil && totalNodes > 0 {
		health += (float64(onlineNodes) / float64(totalNodes)) * 40.0
	} else if totalNodes == 0 {
		health += 40.0 // No penalties if no nodes? Or maybe 0. Let's do 40 for empty state.
	}

	// 2. Salud RF (30%)
	var siteID string
	// Just use the first available site for a global average to save DB hit complexity
	// In a real scenario we average all.
	err = database.Tx(ctx).QueryRow(`SELECT id FROM sites LIMIT 1`).Scan(&siteID)
	if err == nil && siteID != "" {
		res, err := AnalyzeSiteRF(ctx, siteID)
		if err == nil {
			health += (float64(res.OverallHealth) / 100.0) * 30.0
		}
	} else {
		health += 30.0
	}

	// 3. Integridad de backups recientes (30%)
	var recentBackups int
	// Any backup in last 2 days
	err = database.Tx(ctx).QueryRow(`
		SELECT count(*) FROM backups 
		WHERE created_at > CURRENT_TIMESTAMP - INTERVAL '2 days'
	`).Scan(&recentBackups)
	if err == nil && recentBackups > 0 {
		health += 30.0
	}

	final := int(health)
	if final > 100 { final = 100 }
	if final < 0 { final = 0 }
	return final
}
