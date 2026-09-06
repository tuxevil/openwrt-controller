package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

// AggregatedClient is the unified client record after joining all data sources
type AggregatedClient struct {
	MAC                string  `json:"mac"`
	Hostname           string  `json:"hostname"`
	IPAddress          string  `json:"ip_address"`
	UplinkDevice       string  `json:"uplink"`
	UplinkName         string  `json:"uplink_name"`
	SSID               string  `json:"ssid"`
	Signal             float64 `json:"signal"`
	Noise              float64 `json:"noise"`
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
	Trusted            bool    `json:"trusted"`
	TrustLabel         string  `json:"trust_label,omitempty"`
	TrustReason        string  `json:"trust_reason,omitempty"`
	TrustExpiresAt     string  `json:"trust_expires_at,omitempty"`
}

type clientIdentityMetadata struct {
	hostname   string
	trusted    bool
	trustLabel string
	reason     string
	expiresAt  sql.NullTime
}

func GetClientsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	// Fetch custom hostnames first so we don't interleave active queries on the same Tx connection
	customHostnames := make(map[string]clientIdentityMetadata)
	hRows, err := database.Tx(r.Context()).Query(`SELECT mac, hostname, COALESCE(trusted, false),
		COALESCE(trusted_label, ''), COALESCE(trusted_reason, ''), trust_expires_at
		FROM client_hostnames WHERE site_id = $1`, siteID)
	if err == nil {
		for hRows.Next() {
			var m, h, trustLabel, reason string
			var trusted bool
			var expiresAt sql.NullTime
			if err := hRows.Scan(&m, &h, &trusted, &trustLabel, &reason, &expiresAt); err == nil {
				customHostnames[database.NormalizeMAC(m)] = clientIdentityMetadata{hostname: h, trusted: trusted, trustLabel: trustLabel, reason: reason, expiresAt: expiresAt}
			}
		}
		hRows.Close()
	}

	rows, err := database.Tx(r.Context()).Query(
		"SELECT id, name, model, state_json FROM devices WHERE site_id = $1 AND state_json IS NOT NULL",
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
		var devName, devModel sql.NullString
		var stateJSON []byte
		if err := rows.Scan(&devID, &devName, &devModel, &stateJSON); err != nil {
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
			nodeName = devModel.String
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
							MAC:                mac,
							UplinkDevice:       devID,
							UplinkName:         nodeName,
							SSID:               ifaceName,
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

		// Extract L2 tables from neighbor_stats (as sent by the agent) or fallback to root d
		var arpRaw interface{}
		var btRaw interface{}
		if ns, ok := d["neighbor_stats"].(map[string]interface{}); ok {
			arpRaw = ns["arp_table"]
			btRaw = ns["bridge_table"]
		}
		if arpRaw == nil {
			arpRaw = d["arp_table"]
		}
		if btRaw == nil {
			btRaw = d["bridge_table"]
		}

		// ── PRIORITY 2: ARP table (flat array: [{ip, mac}]) ───────────────────
		if arpList, ok := arpRaw.([]interface{}); ok {
			for _, entryRaw := range arpList {
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
		if btList, ok := btRaw.([]interface{}); ok {
			for _, eRaw := range btList {
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
		if custom, ok := customHostnames[mac]; ok {
			if custom.hostname != "" {
				c.Hostname = custom.hostname
			}
			c.Trusted = custom.trusted && (!custom.expiresAt.Valid || time.Now().Before(custom.expiresAt.Time))
			c.TrustLabel = custom.trustLabel
			c.TrustReason = custom.reason
			if custom.expiresAt.Valid {
				c.TrustExpiresAt = custom.expiresAt.Time.UTC().Format(time.RFC3339)
			}
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
	mac, err := parseTrustedClientMAC(r.PathValue("mac"))

	if siteID == "" || err != nil {
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
	_, err = database.Tx(r.Context()).Exec(query, mac, siteID, payload.Hostname)
	if err != nil {
		http.Error(w, `{"error": "failed to update hostname"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

type TrustedClientPayload struct {
	Label     string `json:"label"`
	Reason    string `json:"reason"`
	ExpiresAt string `json:"expires_at"`
}

func parseTrustedClientMAC(raw string) (string, error) {
	canonical := database.NormalizeMAC(raw)
	parsed, err := net.ParseMAC(canonical)
	if err != nil || len(parsed) != 6 {
		return "", fmt.Errorf("mac must be a six-octet address")
	}
	return strings.ToUpper(parsed.String()), nil
}

func parseTrustedClientExpiry(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("expires_at must be RFC3339")
	}
	if !value.After(time.Now()) {
		return nil, fmt.Errorf("expires_at must be in the future")
	}
	return &value, nil
}

func TrustClientHandler(w http.ResponseWriter, r *http.Request) {
	siteID := strings.TrimSpace(r.PathValue("site_id"))
	mac, err := parseTrustedClientMAC(r.PathValue("mac"))
	if err != nil || siteID == "" {
		http.Error(w, `{"error":"site_id and a valid mac are required"}`, http.StatusBadRequest)
		return
	}
	var payload TrustedClientPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	payload.Label = strings.TrimSpace(payload.Label)
	payload.Reason = strings.TrimSpace(payload.Reason)
	if payload.Label == "" || len(payload.Label) > 255 || len(payload.Reason) > 1000 {
		http.Error(w, `{"error":"label is required and reason is limited to 1000 characters"}`, http.StatusBadRequest)
		return
	}
	expiresAt, err := parseTrustedClientExpiry(payload.ExpiresAt)
	if err != nil {
		http.Error(w, `{"error":"invalid expires_at"}`, http.StatusBadRequest)
		return
	}
	var expiresArg interface{}
	if expiresAt != nil {
		expiresArg = *expiresAt
	}
	_, err = database.Tx(r.Context()).Exec(`
		INSERT INTO client_hostnames (mac, site_id, hostname, trusted, trusted_label, trusted_reason, trusted_by, trusted_at, trust_expires_at, updated_at)
		VALUES ($1, $2, $3, true, $3, $4, $5, CURRENT_TIMESTAMP, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (mac) DO UPDATE SET
			site_id = EXCLUDED.site_id,
			hostname = CASE WHEN NULLIF(client_hostnames.hostname, '') IS NULL THEN EXCLUDED.hostname ELSE client_hostnames.hostname END,
			trusted = true,
			trusted_label = EXCLUDED.trusted_label,
			trusted_reason = EXCLUDED.trusted_reason,
			trusted_by = EXCLUDED.trusted_by,
			trusted_at = CURRENT_TIMESTAMP,
			trust_expires_at = EXCLUDED.trust_expires_at,
			updated_at = CURRENT_TIMESTAMP`, mac, siteID, payload.Label, payload.Reason, GetUsernameFromReq(r), expiresArg)
	if err != nil {
		http.Error(w, `{"error":"failed to trust client"}`, http.StatusInternalServerError)
		return
	}
	database.InsertAuditLog(GetUsernameFromReq(r), "CLIENT_TRUSTED", "CLIENT", siteID+"/"+mac,
		fmt.Sprintf("label=%s reason=%s expires_at=%s", payload.Label, payload.Reason, payload.ExpiresAt), r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "trusted", "mac": mac, "label": payload.Label, "expires_at": expiresAt})
}

func UntrustClientHandler(w http.ResponseWriter, r *http.Request) {
	siteID := strings.TrimSpace(r.PathValue("site_id"))
	mac, err := parseTrustedClientMAC(r.PathValue("mac"))
	if err != nil || siteID == "" {
		http.Error(w, `{"error":"site_id and a valid mac are required"}`, http.StatusBadRequest)
		return
	}
	_, err = database.Tx(r.Context()).Exec(`UPDATE client_hostnames SET trusted = false, trusted_label = '', trusted_reason = '', trusted_by = '', trusted_at = NULL, trust_expires_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE site_id = $1 AND mac = $2`, siteID, mac)
	if err != nil {
		http.Error(w, `{"error":"failed to remove client trust"}`, http.StatusInternalServerError)
		return
	}
	database.InsertAuditLog(GetUsernameFromReq(r), "CLIENT_UNTRUSTED", "CLIENT", siteID+"/"+mac, "", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "untrusted", "mac": mac})
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
