package main

import (
	"fmt"
	"log"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
)

func main() {
	if err := database.InitPostgres(); err != nil {
		log.Fatalf("Failed to init postgres: %v", err)
	}

	rows, err := database.DB.Query("SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema', 'pg_catalog') AND schema_name NOT LIKE 'pg_toast%'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			continue
		}

		// For each schema, get sites
		siteRows, err := database.DB.Query(fmt.Sprintf("SELECT id FROM %s.sites", schema))
		if err != nil {
			continue
		}
		var siteIDs []string
		for siteRows.Next() {
			var sid string
			siteRows.Scan(&sid)
			siteIDs = append(siteIDs, sid)
		}
		siteRows.Close()

		for _, siteID := range siteIDs {
			// Get wlans
			wlanRows, err := database.DB.Query(fmt.Sprintf("SELECT id, ssid FROM %s.wlans WHERE site_id = $1", schema), siteID)
			if err != nil {
				continue
			}
			wlans := make(map[string]string)
			for wlanRows.Next() {
				var wid, ssid string
				wlanRows.Scan(&wid, &ssid)
				wlans[ssid] = wid
			}
			wlanRows.Close()

			if len(wlans) == 0 {
				continue
			}

			// Get one device for the site
			var devID string
			err = database.DB.QueryRow(fmt.Sprintf("SELECT id FROM %s.devices WHERE site_id = $1 LIMIT 1", schema), siteID).Scan(&devID)
			if err != nil {
				continue
			}

			fmt.Printf("Syncing roaming config from device %s for site %s\n", devID, siteID)

			cmd := "uci show wireless"
			out, err := orchestrator.ExecuteCommandWithOutput(schema, devID, cmd)
			if err != nil {
				fmt.Printf("Failed to execute command on device %s: %v. Output: %s\n", devID, err, out)
				continue
			}

			lines := strings.Split(out, "\n")

			// We need to map interface name to ssid first
			cmdSsid := "uci show wireless | grep ssid"
			outSsid, _ := orchestrator.ExecuteCommandWithOutput(schema, devID, cmdSsid)
			ifSsidMap := make(map[string]string)
			for _, line := range strings.Split(outSsid, "\n") {
				if strings.Contains(line, ".ssid=") {
					parts := strings.Split(line, "=")
					if len(parts) == 2 {
						iface := strings.TrimSuffix(parts[0], ".ssid")
						ssid := strings.Trim(parts[1], "'")
						ifSsidMap[iface] = ssid
					}
				}
			}

			kEnabled := make(map[string]bool)
			vEnabled := make(map[string]bool)

			for _, line := range lines {
				if strings.Contains(line, ".ieee80211k='1'") {
					parts := strings.Split(line, "=")
					iface := strings.TrimSuffix(parts[0], ".ieee80211k")
					if ssid, ok := ifSsidMap[iface]; ok {
						kEnabled[ssid] = true
					}
				}
				if strings.Contains(line, ".bss_transition='1'") {
					parts := strings.Split(line, "=")
					iface := strings.TrimSuffix(parts[0], ".bss_transition")
					if ssid, ok := ifSsidMap[iface]; ok {
						vEnabled[ssid] = true
					}
				}
			}

			for ssid, wid := range wlans {
				k := false
				v := false
				for devSSID, isK := range kEnabled {
					if strings.Contains(devSSID, ssid) && isK {
						k = true
					}
				}
				for devSSID, isV := range vEnabled {
					if strings.Contains(devSSID, ssid) && isV {
						v = true
					}
				}
				_, err = database.DB.Exec(fmt.Sprintf("UPDATE %s.wlans SET ieee80211k = $1, ieee80211v = $2 WHERE id = $3", schema), k, v, wid)
				if err != nil {
					fmt.Printf("Failed to update wlan %s (%s): %v\n", wid, ssid, err)
				} else {
					fmt.Printf("Updated wlan %s (%s) with 802.11k=%v, 802.11v=%v\n", wid, ssid, k, v)
				}
			}
		}
	}
	fmt.Println("Sync completed")
}
