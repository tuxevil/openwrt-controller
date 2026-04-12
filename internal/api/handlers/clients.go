package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"openwrt-controller/internal/database"
)

// AggregatedClient is the unified client record after joining all data sources
type AggregatedClient struct {
	MAC            string  `json:"mac"`
	Hostname       string  `json:"hostname"`
	IPAddress      string  `json:"ip_address"`
	UplinkDevice   string  `json:"uplink"`
	SSID           string  `json:"ssid"`
	Signal         float64 `json:"signal"`
	Noise          float64 `json:"noise"`
	TXRate         float64 `json:"tx_rate"`
	RXRate         float64 `json:"rx_rate"`
	ConnectionType string  `json:"conn_type"`
}

func GetClientsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	rows, err := database.DB.Query(
		"SELECT id, state_json FROM devices WHERE site_id = $1 AND state_json IS NOT NULL",
		siteID,
	)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	clientMap := make(map[string]*AggregatedClient)

	for rows.Next() {
		var devID string
		var stateJSON []byte
		if err := rows.Scan(&devID, &stateJSON); err != nil {
			continue
		}

		var d map[string]interface{}
		if err := json.Unmarshal(stateJSON, &d); err != nil {
			continue
		}

		// ── PRIORITY 1: wireless_stations (new agent format) ──────────────────
		// Structure: { "phy0-ap0": [{mac, signal, noise, rx_rate, tx_rate}], "phy1-ap0": [] }
		if wsRaw, ok := d["wireless_stations"]; ok {
			if wsMap, ok := wsRaw.(map[string]interface{}); ok {
				for ifaceName, stationsRaw := range wsMap {
					stations, ok := stationsRaw.([]interface{})
					if !ok || len(stations) == 0 {
						continue
					}
					for _, stRaw := range stations {
						st, ok := stRaw.(map[string]interface{})
						if !ok {
							continue
						}
						mac := strVal(st, "mac")
						if mac == "" {
							continue
						}
						// rx_rate and tx_rate come as strings e.g. "65.0"
						clientMap[mac] = &AggregatedClient{
							MAC:            mac,
							UplinkDevice:   devID,
							SSID:           ifaceName,
							Signal:         floatVal(st, "signal"),
							Noise:          floatVal(st, "noise"),
							TXRate:         anyToFloat(st["tx_rate"]),
							RXRate:         anyToFloat(st["rx_rate"]),
							ConnectionType: "wireless",
						}
					}
				}
			}
		}

		// ── FALLBACK: legacy wireless.radioX.interfaces[].stations[] ──────────
		if wirelessBlock, ok := d["wireless"].(map[string]interface{}); ok {
			for _, radioRaw := range wirelessBlock {
				radio, ok := radioRaw.(map[string]interface{})
				if !ok {
					continue
				}
				ifaces, ok := radio["interfaces"].([]interface{})
				if !ok {
					continue
				}
				for _, ifRaw := range ifaces {
					iface, ok := ifRaw.(map[string]interface{})
					if !ok {
						continue
					}
					ssid := ""
					if cfg, ok := iface["config"].(map[string]interface{}); ok {
						ssid = strVal(cfg, "ssid")
					}
					stations, ok := iface["stations"].([]interface{})
					if !ok || len(stations) == 0 {
						continue
					}
					for _, stRaw := range stations {
						st, ok := stRaw.(map[string]interface{})
						if !ok {
							continue
						}
						mac := strVal(st, "mac")
						if mac == "" {
							continue
						}
						if _, found := clientMap[mac]; !found {
							clientMap[mac] = &AggregatedClient{
								MAC:            mac,
								UplinkDevice:   devID,
								SSID:           ssid,
								Signal:         floatVal(st, "signal"),
								Noise:          floatVal(st, "noise"),
								TXRate:         anyToFloat(st["tx_rate"]),
								RXRate:         anyToFloat(st["rx_rate"]),
								ConnectionType: "wireless",
							}
						}
					}
				}
			}
		}

		// ── PRIORITY 2: ARP table (flat array: [{ip, mac}]) ───────────────────
		if arpRaw, ok := d["arp_table"].([]interface{}); ok {
			for _, entryRaw := range arpRaw {
				entry, ok := entryRaw.(map[string]interface{})
				if !ok {
					continue
				}
				mac := strVal(entry, "mac")
				ip := strVal(entry, "ip")
				if mac == "" || mac == "00:00:00:00:00:00" {
					continue
				}
				if existing, found := clientMap[mac]; found {
					if ip != "" && existing.IPAddress == "" {
						existing.IPAddress = ip
					}
				} else {
					clientMap[mac] = &AggregatedClient{
						MAC:            mac,
						IPAddress:      ip,
						UplinkDevice:   devID,
						ConnectionType: "wired",
					}
				}
			}
		}

		// ── PRIORITY 3: bridge_table ───────────────────────────────────────────
		if btRaw, ok := d["bridge_table"].(map[string]interface{}); ok {
			for _, entriesRaw := range btRaw {
				entries, ok := entriesRaw.([]interface{})
				if !ok {
					continue
				}
				for _, eRaw := range entries {
					entry, ok := eRaw.(map[string]interface{})
					if !ok {
						continue
					}
					mac := strVal(entry, "mac")
					if mac == "" || mac == "00:00:00:00:00:00" {
						continue
					}
					if _, found := clientMap[mac]; !found {
						clientMap[mac] = &AggregatedClient{
							MAC:            mac,
							UplinkDevice:   devID,
							ConnectionType: "wired",
						}
					}
				}
			}
		}

		// ── PRIORITY 4: DHCP leases (hostname enrichment) ────────────────────
		if dhcpBlock, ok := d["dhcp"].(map[string]interface{}); ok {
			if leases, ok := dhcpBlock["leases"].([]interface{}); ok {
				for _, leaseRaw := range leases {
					lease, ok := leaseRaw.(map[string]interface{})
					if !ok {
						continue
					}
					mac := strVal(lease, "mac")
					ip := strVal(lease, "ip")
					hostname := strVal(lease, "hostname")
					if mac == "" {
						continue
					}
					if existing, found := clientMap[mac]; found {
						if ip != "" {
							existing.IPAddress = ip
						}
						if hostname != "" {
							existing.Hostname = hostname
						}
					} else {
						clientMap[mac] = &AggregatedClient{
							MAC:            mac,
							Hostname:       hostname,
							IPAddress:      ip,
							UplinkDevice:   devID,
							ConnectionType: "wired",
						}
					}
				}
			}
		}
	}

	clients := make([]AggregatedClient, 0, len(clientMap))
	for _, c := range clientMap {
		clients = append(clients, *c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": clients})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func floatVal(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

// anyToFloat handles both float64 and string number values (e.g. "65.0")
func anyToFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err == nil {
			return f
		}
	}
	return 0
}
