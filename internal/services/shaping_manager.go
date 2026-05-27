package services

import (
	"fmt"
	"log"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
)

// StartSniperReaper starts a background routine to revoke expired sniper rules
func StartSniperReaper() {
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			reapExpiredRules()
		}
	}()
}

func reapExpiredRules() {
	tenants, err := ListTenants()
	if err != nil {
		return
	}
	for _, t := range tenants {
		schema := "tenant_" + t.SchemaAlias
		rows, err := database.DB.Query(fmt.Sprintf(`
			SELECT device_id, mac FROM %s.shaping_rules 
			WHERE expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP
		`, schema))
		if err != nil {
			continue
		}

		var targets []struct{ dev, mac string }
		for rows.Next() {
			var tg struct{ dev, mac string }
			if err := rows.Scan(&tg.dev, &tg.mac); err == nil {
				targets = append(targets, tg)
			}
		}
		rows.Close()

		for _, tg := range targets {
			ClearShaping(schema, tg.dev, tg.mac)
		}
	}
}

// ApplySniperShaping creates or overrides a shaping rule for a specific MAC
func ApplySniperShaping(schema, deviceID, mac string, rateMbytes int, durationMinutes int) error {
	// First execute the SSH command
	// nft add table inet sentinel_shaping
	// nft add chain inet sentinel_shaping forward { type filter hook forward priority 0; }
	// The prompt specified limiting rx and tx identically via the forward chain.
	// Since we need to replace or add, we'll flush the specific rule if it exists? We can handle it via handles or just simple filter.
	// nftables allows naming sets or doing inline drops. We can do: 
	cmd := fmt.Sprintf(`
		# Ensure the table and chain exist
		nft list table inet sentinel_shaping >/dev/null 2>&1 || {
			nft add table inet sentinel_shaping
			nft add chain inet sentinel_shaping forward '{ type filter hook forward priority 0; }'
		}
		
		# Define MAC variable and remove old rules for this MAC to remain idempotent
		MAC="%s"
		# List table with handles, grep the mac, awk the handle, execute delete
		HANDLES=$(nft -a list table inet sentinel_shaping | awk -v m="$MAC" '$0 ~ m {print $NF}')
		for h in $HANDLES; do
			nft delete rule inet sentinel_shaping forward handle $h
		done
		
		# Now add the new rate limit
		nft add rule inet sentinel_shaping forward ether saddr $MAC limit rate over %d kbytes/second drop
		nft add rule inet sentinel_shaping forward ether daddr $MAC limit rate over %d kbytes/second drop
	`, mac, rateMbytes, rateMbytes)

	err := orchestrator.ExecuteCommand(schema, deviceID, cmd)
	if err != nil {
		return err
	}

	var expiresAt *time.Time
	if durationMinutes > 0 {
		t := time.Now().Add(time.Duration(durationMinutes) * time.Minute)
		expiresAt = &t
	}

	// Persist to database
	_, err = database.DB.Exec(fmt.Sprintf(`
		INSERT INTO %s.shaping_rules (device_id, mac, rate_mbytes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (device_id, mac) DO UPDATE SET 
			rate_mbytes = EXCLUDED.rate_mbytes,
			expires_at = EXCLUDED.expires_at,
			created_at = CURRENT_TIMESTAMP
	`, schema), deviceID, mac, rateMbytes, expiresAt)

	return err
}

// ClearShaping removes the shaping for a specific MAC
func ClearShaping(schema, deviceID, mac string) error {
	cmd := fmt.Sprintf(`
		MAC="%s"
		nft list table inet sentinel_shaping >/dev/null 2>&1 || exit 0
		HANDLES=$(nft -a list table inet sentinel_shaping | awk -v m="$MAC" '$0 ~ m {print $NF}')
		for h in $HANDLES; do
			nft delete rule inet sentinel_shaping forward handle $h
		done
	`, mac)

	err := orchestrator.ExecuteCommand(schema, deviceID, cmd)
	if err != nil {
		// Log the error but keep going to clean the DB
		log.Printf("[SNIPER] Clear shaping executor error: %v", err)
	}

	_, dbErr := database.DB.Exec(fmt.Sprintf("DELETE FROM %s.shaping_rules WHERE device_id = $1 AND mac = $2", schema), deviceID, mac)
	return dbErr
}
