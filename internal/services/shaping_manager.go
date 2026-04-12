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
	rows, err := database.DB.Query(`
		SELECT device_id, mac FROM shaping_rules 
		WHERE expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	var targets []struct{ dev, mac string }
	for rows.Next() {
		var t struct{ dev, mac string }
		if err := rows.Scan(&t.dev, &t.mac); err == nil {
			targets = append(targets, t)
		}
	}

	for _, t := range targets {
		ClearShaping(t.dev, t.mac)
	}
}

// ApplySniperShaping creates or overrides a shaping rule for a specific MAC
func ApplySniperShaping(deviceID, mac string, rateMbytes int, durationMinutes int) error {
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

	err := orchestrator.ExecuteCommand(deviceID, cmd)
	if err != nil {
		return err
	}

	var expiresAt *time.Time
	if durationMinutes > 0 {
		t := time.Now().Add(time.Duration(durationMinutes) * time.Minute)
		expiresAt = &t
	}

	// Persist to database
	_, err = database.DB.Exec(`
		INSERT INTO shaping_rules (device_id, mac, rate_mbytes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (device_id, mac) DO UPDATE SET 
			rate_mbytes = EXCLUDED.rate_mbytes,
			expires_at = EXCLUDED.expires_at,
			created_at = CURRENT_TIMESTAMP
	`, deviceID, mac, rateMbytes, expiresAt)

	return err
}

// ClearShaping removes the shaping for a specific MAC
func ClearShaping(deviceID, mac string) error {
	cmd := fmt.Sprintf(`
		MAC="%s"
		nft list table inet sentinel_shaping >/dev/null 2>&1 || exit 0
		HANDLES=$(nft -a list table inet sentinel_shaping | awk -v m="$MAC" '$0 ~ m {print $NF}')
		for h in $HANDLES; do
			nft delete rule inet sentinel_shaping forward handle $h
		done
	`, mac)

	err := orchestrator.ExecuteCommand(deviceID, cmd)
	if err != nil {
		// Log the error but keep going to clean the DB
		log.Printf("[SNIPER] Clear shaping executor error: %v", err)
	}

	_, dbErr := database.DB.Exec("DELETE FROM shaping_rules WHERE device_id = $1 AND mac = $2", deviceID, mac)
	return dbErr
}
