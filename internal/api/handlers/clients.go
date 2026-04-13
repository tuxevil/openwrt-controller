package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"openwrt-controller/internal/database"
)

// AggregatedClient is the unified client record after joining all data sources
type AggregatedClient struct {
	MAC            string  `json:"mac"`
	Hostname       string  `json:"hostname"`
	IPAddress      string  `json:"ip_address"`
	UplinkDevice   string  `json:"uplink"`
	UplinkName     string  `json:"uplink_name"`
	SSID           string  `json:"ssid"`
	Signal         float64 `json:"signal"`
	Noise          float64 `json:"noise"`
	TXRate             float64 `json:"tx_rate"`
	RXRate             float64 `json:"rx_rate"`
	TxMCS              float64 `json:"tx_mcs"`
	RxMCS              float64 `json:"rx_mcs"`
	TxMHz              string  `json:"tx_mhz"`
	RxMHz              string  `json:"rx_mhz"`
	TxPkts             int     `json:"tx_pkts"`
	RxPkts             int     `json:"rx_pkts"`
	InactiveTime       int     `json:"inactive"`
	ExpectedThroughput string  `json:"expected_throughput"`
	ConnectionType     string  `json:"conn_type"`
}

func GetClientsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	rows, err := database.DB.Query(
		"SELECT id, name, state_json FROM devices WHERE site_id = $1 AND state_json IS NOT NULL",
		siteID,
	)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Fetch custom hostnames
	customHostnames := make(map[string]string)
	hRows, err := database.DB.Query("SELECT mac, hostname FROM client_hostnames WHERE site_id = $1", siteID)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var m, h string
			if err := hRows.Scan(&m, &h); err == nil {
				customHostnames[strings.ToUpper(m)] = h
			}
		}
	}

	clientMap := make(map[string]*AggregatedClient)

	for rows.Next() {
		var devID string
		var devName sql.NullString
		var stateJSON []byte
		if err := rows.Scan(&devID, &devName, &stateJSON); err != nil {
			continue
		}
		nodeName := devName.String

		var d map[string]interface{}
		if err := json.Unmarshal(stateJSON, &d); err != nil {
			continue
		}

		// Extract board hostname as node name fallback
		if nodeName == "" {
			if board, ok := d["board"].(map[string]interface{}); ok {
				if bh, ok := board["hostname"].(string); ok && bh != "" {
					nodeName = bh
				}
			}
		}
		if nodeName == "" {
			nodeName = devID
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
						mac := strings.ToUpper(strVal(st, "mac"))
						if mac == "" {
							continue
						}
						// rx_rate and tx_rate come as strings e.g. "65.0"
						clientMap[mac] = &AggregatedClient{
							MAC:            mac,
							UplinkDevice:   devID,
							UplinkName:     nodeName,
							SSID:           ifaceName,
							Signal:         floatVal(st, "signal"),
							Noise:              floatVal(st, "noise"),
							TXRate:             anyToFloat(st["tx_rate"]),
							RXRate:             anyToFloat(st["rx_rate"]),
							TxMCS:              anyToFloat(st["tx_mcs"]),
							RxMCS:              anyToFloat(st["rx_mcs"]),
							TxMHz:              strVal(st, "tx_mhz"),
							RxMHz:              strVal(st, "rx_mhz"),
							TxPkts:             int(floatVal(st, "tx_pkts")),
							RxPkts:             int(floatVal(st, "rx_pkts")),
							InactiveTime:       int(floatVal(st, "inactive")),
							ExpectedThroughput: strVal(st, "expected_throughput"),
							ConnectionType:     "wireless",
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
						mac := strings.ToUpper(strVal(st, "mac"))
						if mac == "" {
							continue
						}
						if _, found := clientMap[mac]; !found {
							clientMap[mac] = &AggregatedClient{
								MAC:                mac,
								UplinkDevice:       devID,
								UplinkName:         nodeName,
								SSID:               ssid,
								Signal:             floatVal(st, "signal"),
								Noise:              floatVal(st, "noise"),
								TXRate:             anyToFloat(st["tx_rate"]),
								RXRate:             anyToFloat(st["rx_rate"]),
								TxMCS:              anyToFloat(st["tx_mcs"]),
								RxMCS:              anyToFloat(st["rx_mcs"]),
								TxMHz:              strVal(st, "tx_mhz"),
								RxMHz:              strVal(st, "rx_mhz"),
								TxPkts:             int(floatVal(st, "tx_pkts")),
								RxPkts:             int(floatVal(st, "rx_pkts")),
								InactiveTime:       int(floatVal(st, "inactive")),
								ExpectedThroughput: strVal(st, "expected_throughput"),
								ConnectionType:     "wireless",
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
				mac := strings.ToUpper(strVal(entry, "mac"))
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
						UplinkName:     nodeName,
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
					mac := strings.ToUpper(strVal(entry, "mac"))
					if mac == "" || mac == "00:00:00:00:00:00" {
						continue
					}
					if _, found := clientMap[mac]; !found {
						clientMap[mac] = &AggregatedClient{
							MAC:            mac,
							UplinkDevice:   devID,
							UplinkName:     nodeName,
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
					mac := strings.ToUpper(strVal(lease, "mac"))
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
							UplinkName:     nodeName,
							ConnectionType: "wired",
						}
					}
				}
			}
		}
	}

	clients := make([]AggregatedClient, 0, len(clientMap))
	for mac, c := range clientMap {
		if customHostname, ok := customHostnames[mac]; ok {
			c.Hostname = customHostname
		}
		clients = append(clients, *c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": clients})
}

// UpdateClientHostnamePayload encapsulates hostname updates
type UpdateClientHostnamePayload struct {
	Hostname string `json:"hostname"`
}

func UpdateClientHostnameHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	mac := strings.ToUpper(r.PathValue("mac"))

	if siteID == "" || mac == "" {
		http.Error(w, `{"error": "site_id and mac are required"}`, http.StatusBadRequest)
		return
	}

	var payload UpdateClientHostnamePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error": "invalid json body"}`, http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO client_hostnames (mac, site_id, hostname, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (mac) DO UPDATE SET 
			hostname = EXCLUDED.hostname,
			site_id = EXCLUDED.site_id,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := database.DB.Exec(query, mac, siteID, payload.Hostname)
	if err != nil {
		http.Error(w, `{"error": "failed to update hostname"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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
