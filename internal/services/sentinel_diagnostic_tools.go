package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
)

var (
	sentinelHostnamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)
	sentinelPingStatsRe     = regexp.MustCompile(`(\d+)\s+packets transmitted,\s+(\d+)\s+(?:packets\s+)?received.*?([0-9.]+)%\s+packet loss`)
	sentinelPingRTTRe       = regexp.MustCompile(`(?:rtt|round-trip)\s+min/avg/max(?:/[a-z]+)?\s*=\s*([0-9.]+)/([0-9.]+)/([0-9.]+)`)
	sentinelTraceHopRe      = regexp.MustCompile(`^\s*(\d+)\s+(\S+)(?:\s+([0-9.]+)\s*ms)?`)

	sentinelIperfCooldownMu sync.Mutex
	sentinelIperfLastRun    = map[string]time.Time{}
)

const sentinelIperfCooldown = 5 * time.Minute

func isValidSentinelProbeHost(raw string) bool {
	target := strings.TrimSpace(raw)
	if target == "" || len(target) > 253 || strings.HasPrefix(target, "-") {
		return false
	}
	if net.ParseIP(target) != nil {
		return true
	}
	return sentinelHostnamePattern.MatchString(target)
}

func isPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// Carrier-grade NAT 100.64.0.0/10 (Tailscale/Starlink/internal overlays)
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

type sentinelSiteDeviceRow struct {
	ID         string
	Name       string
	Model      string
	Status     string
	LastIP     string
	DeviceRole string
	State      map[string]interface{}
}

func loadSentinelSiteDevices(ctx context.Context, schema, siteID string) ([]sentinelSiteDeviceRow, database.SiteClientHostnameLookup, error) {
	lookup := database.NewSiteClientHostnameLookup()
	if database.DB == nil {
		return nil, lookup, fmt.Errorf("database is unavailable")
	}

	// Load custom client_hostnames for this schema/site
	hQuery := fmt.Sprintf(`SELECT mac, hostname FROM %s.client_hostnames`, schema)
	var hParams []interface{}
	if siteID != "" {
		hQuery += " WHERE site_id = $1"
		hParams = append(hParams, siteID)
	}
	if hRows, err := database.DB.QueryContext(ctx, hQuery, hParams...); err == nil {
		for hRows.Next() {
			var mac, host string
			if hRows.Scan(&mac, &host) == nil {
				host = strings.TrimSpace(host)
				if host == "" || host == "*" {
					continue
				}
				trimmed := strings.TrimSpace(mac)
				if net.ParseIP(trimmed) != nil {
					lookup.ByIP[trimmed] = host
				}
				if key := database.NormalizeMAC(trimmed); key != "" {
					lookup.ByMAC[key] = host
				}
			}
		}
		hRows.Close()
	}

	dQuery := fmt.Sprintf(`SELECT id, COALESCE(name, ''), COALESCE(model, ''), COALESCE(status, ''), COALESCE(last_ip, ''), COALESCE(device_role, ''), state_json FROM %s.devices`, schema)
	var dParams []interface{}
	if siteID != "" {
		dQuery += " WHERE site_id = $1"
		dParams = append(dParams, siteID)
	}
	dQuery += " ORDER BY last_ip ASC, id ASC"

	rows, err := database.DB.QueryContext(ctx, dQuery, dParams...)
	if err != nil {
		return nil, lookup, err
	}
	defer rows.Close()

	var devices []sentinelSiteDeviceRow
	for rows.Next() {
		var row sentinelSiteDeviceRow
		var stateBytes []byte
		if err := rows.Scan(&row.ID, &row.Name, &row.Model, &row.Status, &row.LastIP, &row.DeviceRole, &stateBytes); err != nil {
			continue
		}
		row.State = map[string]interface{}{}
		if len(stateBytes) > 0 {
			_ = json.Unmarshal(stateBytes, &row.State)
		}
		lookup.MergeDeviceState(row.ID, row.Name, row.LastIP, row.State)
		devices = append(devices, row)
	}
	return devices, lookup, rows.Err()
}

func resolveSentinelManagedDevice(ctx context.Context, schema, siteID, ref string) (sentinelSiteDeviceRow, error) {
	devices, lookup, err := loadSentinelSiteDevices(ctx, schema, siteID)
	if err != nil {
		return sentinelSiteDeviceRow{}, err
	}
	target := strings.TrimSpace(ref)
	targetLower := strings.ToLower(target)
	targetMAC := database.NormalizeMAC(target)
	for _, dev := range devices {
		name := dev.Name
		display := database.DeviceDisplayName(dev.ID, &name, dev.State, lookup)
		if strings.EqualFold(dev.ID, target) ||
			database.NormalizeMAC(dev.ID) == targetMAC ||
			strings.EqualFold(display, target) ||
			strings.EqualFold(dev.Name, target) ||
			dev.LastIP == target ||
			strings.ToLower(display) == targetLower {
			return dev, nil
		}
	}
	return sentinelSiteDeviceRow{}, fmt.Errorf("managed device %q not found in scope", ref)
}

func resolveSentinelGatewayIP(ctx context.Context, schema, siteID string) string {
	if database.DB == nil {
		return ""
	}
	devices, _, err := loadSentinelSiteDevices(ctx, schema, siteID)
	if err != nil {
		return ""
	}
	for _, dev := range devices {
		if strings.EqualFold(dev.DeviceRole, "Gateway") && dev.LastIP != "" {
			return dev.LastIP
		}
	}
	for _, dev := range devices {
		if _, hasWAN := dev.State["wan"]; hasWAN && dev.LastIP != "" {
			return dev.LastIP
		}
	}
	for _, dev := range devices {
		if dev.LastIP != "" {
			return dev.LastIP
		}
	}
	return ""
}

// ── 1. get_client_diagnostics ────────────────────────────────────────────────

func sentinelGetClientDiagnosticsTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	clientRef, _ := invocation.Args["client"].(string)
	clientRef = strings.TrimSpace(clientRef)
	if clientRef == "" {
		return nil, fmt.Errorf("client (MAC, IP, or hostname) is required")
	}
	siteID, _ := invocation.Args["site_id"].(string)
	schema := invocation.Schema

	devices, lookup, err := loadSentinelSiteDevices(ctx, schema, siteID)
	if err != nil {
		return nil, err
	}
	identityDirectory, _ := database.LoadNetworkIdentityDirectory(schema, siteID)

	// Resolve clientRef (could be MAC, IP, or hostname) to canonical MAC and/or IP
	targetMAC := ""
	targetIP := ""
	targetHostname := ""
	refLower := strings.ToLower(clientRef)

	if parsedMAC, err := net.ParseMAC(clientRef); err == nil && len(parsedMAC) == 6 {
		targetMAC = strings.ToUpper(parsedMAC.String())
	} else if net.ParseIP(clientRef) != nil {
		targetIP = clientRef
	} else {
		// Match by hostname (case-insensitive) across lookup.ByMAC and identityDirectory
		for mac, host := range lookup.ByMAC {
			if strings.EqualFold(host, clientRef) {
				if parsed, err := net.ParseMAC(mac); err == nil && len(parsed) == 6 {
					targetMAC = strings.ToUpper(parsed.String())
					targetHostname = host
					break
				}
			}
		}
		if targetMAC == "" {
			for mac, host := range lookup.ByMAC {
				if strings.Contains(strings.ToLower(host), refLower) {
					if parsed, err := net.ParseMAC(mac); err == nil && len(parsed) == 6 {
						targetMAC = strings.ToUpper(parsed.String())
						targetHostname = host
						break
					}
				}
			}
		}
		if targetMAC == "" {
			for ip, host := range lookup.ByIP {
				if strings.EqualFold(host, clientRef) {
					targetIP = ip
					targetHostname = host
					break
				}
			}
		}
	}

	// Cross-resolve IP <-> MAC from DHCP leases and ARP tables across all devices
	sources := map[string]bool{}
	var wirelessDetails map[string]interface{}
	var dhcpDetails map[string]interface{}
	arpObservedOn := []string{}
	bridgeObservedOn := []string{}
	uplinkNode := ""

	for _, dev := range devices {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)

		// DHCP leases
		if dhcpBlock, ok := dev.State["dhcp"].(map[string]interface{}); ok {
			if leases, ok := dhcpBlock["leases"].([]interface{}); ok {
				for _, raw := range leases {
					lease, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					lMAC := database.NormalizeMAC(stringField(lease, "mac"))
					lIP := stringField(lease, "ip")
					lHost := stringField(lease, "hostname")
					matched := (targetMAC != "" && lMAC == targetMAC) ||
						(targetIP != "" && lIP == targetIP) ||
						(targetMAC == "" && targetIP == "" && lHost != "" && strings.EqualFold(lHost, clientRef))
					if matched {
						if targetMAC == "" && lMAC != "" {
							targetMAC = lMAC
						}
						if targetIP == "" && lIP != "" {
							targetIP = lIP
						}
						sources["dhcp_lease"] = true
						dhcpDetails = map[string]interface{}{
							"ip":               lIP,
							"hostname":         lHost,
							"expires":          lease["expires"],
							"reported_by_node": devName,
						}
					}
				}
			}
		}

		// ARP table
		var arpRaw interface{}
		if ns, ok := dev.State["neighbor_stats"].(map[string]interface{}); ok {
			arpRaw = ns["arp_table"]
		}
		if arpRaw == nil {
			arpRaw = dev.State["arp_table"]
		}
		if entries, ok := arpRaw.([]interface{}); ok {
			for _, raw := range entries {
				entry, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				aMAC := database.NormalizeMAC(stringField(entry, "mac"))
				aIP := stringField(entry, "ip")
				if (targetMAC != "" && aMAC == targetMAC) || (targetIP != "" && aIP == targetIP) {
					if targetMAC == "" && aMAC != "" {
						targetMAC = aMAC
					}
					if targetIP == "" && aIP != "" {
						targetIP = aIP
					}
					sources["arp"] = true
					arpObservedOn = append(arpObservedOn, devName)
					if uplinkNode == "" {
						uplinkNode = devName
					}
				}
			}
		}
	}

	// Second pass with resolved targetMAC for wireless_stations & bridge_table
	for _, dev := range devices {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		if targetMAC != "" {
			if ws, ok := dev.State["wireless_stations"].(map[string]interface{}); ok {
				for iface, rawList := range ws {
					stations, ok := rawList.([]interface{})
					if !ok {
						continue
					}
					for _, raw := range stations {
						st, ok := raw.(map[string]interface{})
						if !ok {
							continue
						}
						if database.NormalizeMAC(stringField(st, "mac")) == targetMAC {
							sources["wireless_station"] = true
							uplinkNode = devName
							wirelessDetails = map[string]interface{}{
								"node":                devName,
								"node_id":             dev.ID,
								"interface":           iface,
								"signal_dbm":          st["signal"],
								"noise_dbm":           st["noise"],
								"tx_rate":             st["tx_rate"],
								"rx_rate":             st["rx_rate"],
								"tx_packets":          st["tx_packets"],
								"rx_packets":          st["rx_packets"],
								"tx_retries":          st["tx_retries"],
								"tx_failed":           st["tx_failed"],
								"inactive_ms":         st["inactive"],
								"expected_throughput": st["expected_throughput"],
							}
						}
					}
				}
			}

			var bridgeRaw interface{}
			if ns, ok := dev.State["neighbor_stats"].(map[string]interface{}); ok {
				bridgeRaw = ns["bridge_table"]
			}
			if bridgeRaw == nil {
				bridgeRaw = dev.State["bridge_table"]
			}
			if entries, ok := bridgeRaw.([]interface{}); ok {
				for _, raw := range entries {
					entry, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if strings.EqualFold(stringField(entry, "is_local"), "yes") {
						continue
					}
					if database.NormalizeMAC(stringField(entry, "mac")) == targetMAC {
						sources["bridge"] = true
						bridgeObservedOn = append(bridgeObservedOn, devName)
						if uplinkNode == "" {
							uplinkNode = devName
						}
					}
				}
			}
		}
	}

	if targetHostname == "" {
		if targetMAC != "" {
			targetHostname = lookup.ResolveMAC(targetMAC)
		}
		if targetHostname == "" && targetIP != "" {
			targetHostname = lookup.ResolveIP(targetIP)
		}
	}

	var vendorInfo *database.MACVendorInfo
	if targetMAC != "" {
		vi := database.LookupMACVendor(targetMAC)
		vendorInfo = &vi
	}

	// Active bandwidth shaping rule (if any)
	var shapingRule map[string]interface{}
	if targetMAC != "" {
		var devID string
		var dl, ul int
		err := database.DB.QueryRowContext(ctx, fmt.Sprintf(`SELECT device_id, download_kbps, upload_kbps FROM %s.shaping_rules WHERE UPPER(mac) = $1 LIMIT 1`, schema), targetMAC).Scan(&devID, &dl, &ul)
		if err == nil {
			shapingRule = map[string]interface{}{
				"device_id":     devID,
				"download_kbps": dl,
				"upload_kbps":   ul,
			}
		}
	}

	// Recent logs matching MAC or IP (max 15)
	recentLogs := []map[string]interface{}{}
	var logConds []string
	var logParams []interface{}
	if targetMAC != "" {
		logParams = append(logParams, "%"+targetMAC+"%")
		logConds = append(logConds, fmt.Sprintf("l.message ILIKE $%d", len(logParams)))
	}
	if targetIP != "" {
		logParams = append(logParams, "%"+targetIP+"%")
		logConds = append(logConds, fmt.Sprintf("l.message ILIKE $%d", len(logParams)))
	}
	if len(logConds) == 0 {
		logParams = append(logParams, "%"+clientRef+"%")
		logConds = append(logConds, fmt.Sprintf("l.message ILIKE $%d", len(logParams)))
	}
	logQuery := fmt.Sprintf(`SELECT l.log_timestamp, l.severity, l.message, l.device_id,
		COALESCE(NULLIF(d.name, ''), NULLIF(d.state_json->'board'->>'hostname', ''), NULLIF(d.model, ''), l.device_id)
		FROM %s.system_logs l LEFT JOIN %s.devices d ON d.id = l.device_id
		WHERE (%s)`, schema, schema, strings.Join(logConds, " OR "))
	if siteID != "" {
		logParams = append(logParams, siteID)
		logQuery += fmt.Sprintf(" AND d.site_id = $%d", len(logParams))
	}
	logQuery += " ORDER BY l.log_timestamp DESC LIMIT 15"
	if lRows, err := database.DB.QueryContext(ctx, logQuery, logParams...); err == nil {
		for lRows.Next() {
			var ts time.Time
			var sev, msg, devID, devName string
			if lRows.Scan(&ts, &sev, &msg, &devID, &devName) != nil {
				continue
			}
			if idInfo, ok := database.ResolveNetworkIdentity(identityDirectory, devID); ok {
				devName = idInfo.DisplayLabel()
			}
			human := database.AnnotateNetworkText(msg, identityDirectory)
			if strings.TrimSpace(devName) != "" {
				human = fmt.Sprintf("[node=%s] %s", strings.TrimSpace(devName), human)
			}
			recentLogs = append(recentLogs, map[string]interface{}{
				"timestamp":     ts.UTC().Format(time.RFC3339),
				"severity":      sev,
				"device_name":   devName,
				"message_human": human,
			})
		}
		lRows.Close()
	}

	connType := classifySentinelClientConnection(sources)
	found := len(sources) > 0 || targetMAC != "" || targetHostname != "" || len(recentLogs) > 0
	if len(sources) == 0 {
		connType = "not_observed"
	}

	result := map[string]interface{}{
		"query":           clientRef,
		"found":           found,
		"mac":             targetMAC,
		"hostname":        targetHostname,
		"ip":              targetIP,
		"connection_type": connType,
		"active_wireless": connType == "wireless",
		"observed_in":     sortedSentinelClientSources(sources),
		"uplink_node":     uplinkNode,
		"arp_nodes":       arpObservedOn,
		"bridge_nodes":    bridgeObservedOn,
		"recent_logs":     recentLogs,
	}
	if !found {
		result["note"] = "This client/IP/MAC was NOT found in any active wireless radio, DHCP lease, ARP table, bridge table, or recent system logs. Do NOT report it as wired or connected."
	}
	if vendorInfo != nil {
		result["vendor"] = vendorInfo.Vendor
		result["oui"] = vendorInfo.OUI
		result["locally_administered"] = vendorInfo.LocallyAdministered
		if vendorInfo.Note != "" {
			result["vendor_note"] = vendorInfo.Note
		}
	}
	if wirelessDetails != nil {
		result["wireless_radio"] = wirelessDetails
	}
	if dhcpDetails != nil {
		result["dhcp_lease"] = dhcpDetails
	}
	if shapingRule != nil {
		result["shaping_rule"] = shapingRule
	}
	return result, nil
}

// ── 2. get_bandwidth_stats ───────────────────────────────────────────────────

func sentinelGetBandwidthStatsTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	schema := invocation.Schema

	devices, lookup, err := loadSentinelSiteDevices(ctx, schema, siteID)
	if err != nil {
		return nil, err
	}

	// Active shaping rules
	shaping := []map[string]interface{}{}
	sQuery := fmt.Sprintf(`SELECT sr.device_id, sr.mac, sr.download_kbps, sr.upload_kbps
		FROM %s.shaping_rules sr JOIN %s.devices d ON d.id = sr.device_id`, schema, schema)
	var sParams []interface{}
	if siteID != "" {
		sQuery += " WHERE d.site_id = $1"
		sParams = append(sParams, siteID)
	}
	if sRows, err := database.DB.QueryContext(ctx, sQuery, sParams...); err == nil {
		for sRows.Next() {
			var devID, mac string
			var dl, ul int
			if sRows.Scan(&devID, &mac, &dl, &ul) == nil {
				shaping = append(shaping, map[string]interface{}{
					"device_id":     devID,
					"mac":           database.NormalizeMAC(mac),
					"hostname":      lookup.ResolveMAC(mac),
					"download_kbps": dl,
					"upload_kbps":   ul,
				})
			}
		}
		sRows.Close()
	}

	nodesOut := make([]map[string]interface{}, 0, len(devices))
	for _, dev := range devices {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		talkers := []map[string]interface{}{}
		if rawList, ok := dev.State["top_talkers"].([]interface{}); ok {
			for _, raw := range rawList {
				tm, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				mac := stringField(tm, "mac")
				rxBytes, _ := tm["rate_rx"].(float64)
				txBytes, _ := tm["rate_tx"].(float64)
				talkers = append(talkers, map[string]interface{}{
					"mac":      database.NormalizeMAC(mac),
					"hostname": lookup.Resolve(mac),
					"rx_mbps":  mathRound2(rxBytes * 8 / 1024 / 1024),
					"tx_mbps":  mathRound2(txBytes * 8 / 1024 / 1024),
				})
			}
		}
		nodesOut = append(nodesOut, map[string]interface{}{
			"device_id":   dev.ID,
			"device_name": devName,
			"iface_stats": dev.State["iface_stats"],
			"top_talkers": talkers,
		})
	}

	return map[string]interface{}{
		"site_id":       siteID,
		"nodes":         nodesOut,
		"shaping_rules": shaping,
	}, nil
}

// ── 3. get_flow_summary ──────────────────────────────────────────────────────

func sentinelGetFlowSummaryTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	clientFilter, _ := invocation.Args["client"].(string)
	clientFilter = strings.ToLower(strings.TrimSpace(clientFilter))
	flaggedOnly, _ := invocation.Args["flagged_only"].(bool)
	limit := invocation.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}

	devices, lookup, err := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	if err != nil {
		return nil, err
	}

	flowsOut := []map[string]interface{}{}
	flaggedCount := 0
	for _, dev := range devices {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		rawFlows, ok := dev.State["flow_sense"].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range rawFlows {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			src := stringField(m, "sample_src")
			dst := stringField(m, "dst")
			srcHost := lookup.Resolve(src)
			dstHost := lookup.Resolve(dst)
			flagged, _ := m["flagged"].(bool)
			if flagged {
				flaggedCount++
			}
			if flaggedOnly && !flagged {
				continue
			}
			if clientFilter != "" {
				hay := strings.ToLower(strings.Join([]string{src, srcHost, dst, dstHost}, " "))
				if !strings.Contains(hay, clientFilter) {
					continue
				}
			}
			entry := map[string]interface{}{
				"node":       devName,
				"proto":      m["proto"],
				"src_ip":     src,
				"src_host":   srcHost,
				"dst_ip":     dst,
				"dst_host":   dstHost,
				"dport":      m["dport"],
				"conns":      m["conns"],
				"flagged":    flagged,
				"flag_notes": m["reason"],
			}
			flowsOut = append(flowsOut, entry)
			if len(flowsOut) >= limit {
				break
			}
		}
		if len(flowsOut) >= limit {
			break
		}
	}

	return map[string]interface{}{
		"site_id":       siteID,
		"total_shown":   len(flowsOut),
		"flagged_total": flaggedCount,
		"flows":         flowsOut,
	}, nil
}

// ── 4. get_rf_spectrum ───────────────────────────────────────────────────────

func sentinelGetRFSpectrumTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	devices, lookup, err := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	if err != nil {
		return nil, err
	}

	clients := []map[string]interface{}{}
	radios := []map[string]interface{}{}
	for _, dev := range devices {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		if ws, ok := dev.State["wireless_stations"].(map[string]interface{}); ok {
			for iface, rawList := range ws {
				stations, ok := rawList.([]interface{})
				if !ok {
					continue
				}
				radios = append(radios, map[string]interface{}{
					"node":         devName,
					"interface":    iface,
					"client_count": len(stations),
				})
				for _, raw := range stations {
					st, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					mac := database.NormalizeMAC(stringField(st, "mac"))
					sig, _ := st["signal"].(float64)
					noise, _ := st["noise"].(float64)
					if noise == 0 {
						noise = -95
					}
					clients = append(clients, map[string]interface{}{
						"node":        devName,
						"interface":   iface,
						"mac":         mac,
						"hostname":    lookup.ResolveMAC(mac),
						"signal_dbm":  sig,
						"noise_dbm":   noise,
						"snr_db":      sig - noise,
						"tx_rate":     st["tx_rate"],
						"rx_rate":     st["rx_rate"],
						"tx_retries":  st["tx_retries"],
						"tx_failed":   st["tx_failed"],
						"inactive_ms": st["inactive"],
					})
				}
			}
		}
	}

	sort.Slice(clients, func(i, j int) bool {
		si, _ := clients[i]["snr_db"].(float64)
		sj, _ := clients[j]["snr_db"].(float64)
		return si < sj
	})

	return map[string]interface{}{
		"site_id":      siteID,
		"radio_count":  len(radios),
		"client_count": len(clients),
		"radios":       radios,
		"clients":      clients,
	}, nil
}

// ── 5. get_dhcp_leases ───────────────────────────────────────────────────────

func sentinelGetDHCPLeasesTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	filter, _ := invocation.Args["query"].(string)
	filter = strings.ToLower(strings.TrimSpace(filter))
	limit := invocation.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	devices, lookup, err := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	if err != nil {
		return nil, err
	}

	leasesOut := []map[string]interface{}{}
	seen := map[string]bool{}
	for _, dev := range devices {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		dhcpBlock, ok := dev.State["dhcp"].(map[string]interface{})
		if !ok {
			continue
		}
		leases, ok := dhcpBlock["leases"].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range leases {
			lease, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			mac := database.NormalizeMAC(stringField(lease, "mac"))
			ip := stringField(lease, "ip")
			dhcpHost := stringField(lease, "hostname")
			customHost := lookup.ResolveMAC(mac)
			vendorInfo := database.LookupMACVendor(mac)

			dedupKey := mac + "|" + ip
			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true

			if filter != "" {
				hay := strings.ToLower(strings.Join([]string{mac, ip, dhcpHost, customHost, vendorInfo.Vendor}, " "))
				if !strings.Contains(hay, filter) {
					continue
				}
			}

			leasesOut = append(leasesOut, map[string]interface{}{
				"mac":                  mac,
				"ip":                   ip,
				"hostname":             customHost,
				"dhcp_hostname":        dhcpHost,
				"vendor":               vendorInfo.Vendor,
				"locally_administered": vendorInfo.LocallyAdministered,
				"expires":              lease["expires"],
				"reported_by_node":     devName,
			})
			if len(leasesOut) >= limit {
				break
			}
		}
	}

	return map[string]interface{}{
		"site_id": siteID,
		"count":   len(leasesOut),
		"leases":  leasesOut,
	}, nil
}

// ── 6. get_wan_health ────────────────────────────────────────────────────────

var sentinelSiteIDRegex = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

func queryStarlinkHealthLive(ctx context.Context, siteID string) map[string]interface{} {
	if database.InfluxClient == nil || !sentinelSiteIDRegex.MatchString(siteID) {
		return map[string]interface{}{"available": false}
	}
	query := `from(bucket: "` + database.GetInfluxBucket() + `")
		|> range(start: -10m)
		|> filter(fn: (r) => r["_measurement"] == "starlink_health")
		|> filter(fn: (r) => r["site_id"] == "` + siteID + `")
		|> last()`

	qctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	result, err := database.InfluxClient.QueryAPI(database.GetInfluxOrg()).Query(qctx, query)
	if err != nil {
		return map[string]interface{}{"available": false, "error": err.Error()}
	}
	defer result.Close()

	status := ""
	var dishRTT, popRTT, pktLoss float64
	microOutage := false
	var sampledAt time.Time
	found := false

	for result.Next() {
		found = true
		rec := result.Record()
		sampledAt = rec.Time()
		if tag, ok := rec.ValueByKey("status").(string); ok && tag != "" {
			status = tag
		}
		switch rec.Field() {
		case "dish_rtt_ms":
			if v, ok := rec.Value().(float64); ok {
				dishRTT = v
			}
		case "pop_rtt_ms":
			if v, ok := rec.Value().(float64); ok {
				popRTT = v
			}
		case "packet_loss":
			if v, ok := rec.Value().(float64); ok {
				pktLoss = v
			}
		case "micro_outage":
			if v, ok := rec.Value().(int64); ok {
				microOutage = v == 1
			}
		}
	}
	if !found {
		return map[string]interface{}{"available": false, "reason": "no starlink_health samples in last 10m"}
	}

	rating := status
	if status == "ONLINE" {
		if dishRTT < 5.0 && popRTT < 55.0 && pktLoss == 0 {
			rating = "OPTIMAL"
		} else {
			rating = "SATISFACTORY"
		}
	}

	return map[string]interface{}{
		"available":       true,
		"status":          status,
		"health_rating":   rating,
		"dish_rtt_ms":     mathRound2(dishRTT),
		"pop_rtt_ms":      mathRound2(popRTT),
		"packet_loss_pct": mathRound2(pktLoss),
		"micro_outage":    microOutage,
		"last_sampled_at": sampledAt.UTC().Format(time.RFC3339),
	}
}

func parseWANLiveStatus(raw string, now time.Time, lookup database.SiteClientHostnameLookup) (map[string]interface{}, map[string]interface{}, map[string]interface{}) {
	// Split optional Starlink / WAN live ping sections from the end
	starlinkProbe := map[string]interface{}{"available": false}
	mainPart := raw
	if idx := strings.Index(mainPart, "---STARLINK_DISH---"); idx >= 0 {
		probeTail := mainPart[idx+len("---STARLINK_DISH---"):]
		mainPart = mainPart[:idx]
		dishPart := probeTail
		popPart := ""
		if pIdx := strings.Index(probeTail, "---WAN_POP---"); pIdx >= 0 {
			dishPart = probeTail[:pIdx]
			popPart = probeTail[pIdx+len("---WAN_POP---"):]
		}
		dishStats := parsePingSummary(dishPart)
		popStats := parsePingSummary(popPart)
		dishOK, _ := dishStats["reachable"].(bool)
		popOK, _ := popStats["reachable"].(bool)
		if dishOK || popOK {
			dishRTT, _ := dishStats["avg_ms"].(float64)
			popRTT, _ := popStats["avg_ms"].(float64)
			loss, _ := popStats["packet_loss_pct"].(float64)
			status := "ONLINE"
			rating := "SATISFACTORY"
			if !popOK {
				status = "DEGRADED"
				rating = "DEGRADED"
			} else if dishOK && dishRTT < 5.0 && popRTT < 55.0 && loss == 0 {
				rating = "OPTIMAL"
			}
			starlinkProbe = map[string]interface{}{
				"available":       true,
				"source":          "live_gateway_probe",
				"dish_ip":         "192.168.100.1",
				"dish_reachable":  dishOK,
				"dish_rtt_ms":     mathRound2(dishRTT),
				"pop_target":      "1.1.1.1",
				"pop_reachable":   popOK,
				"pop_rtt_ms":      mathRound2(popRTT),
				"packet_loss_pct": mathRound2(loss),
				"status":          status,
				"health_rating":   rating,
				"last_sampled_at": now.UTC().Format(time.RFC3339),
			}
		}
	}

	parts := strings.SplitN(mainPart, "---MWAN3---", 2)
	wgPart := parts[0]
	mwanPart := ""
	if len(parts) == 2 {
		mwanPart = parts[1]
	}

	wgInterfaces := []map[string]interface{}{}
	wgPeers := []map[string]interface{}{}
	for _, line := range strings.Split(wgPart, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		switch fields[0] {
		case "WG_IFACE":
			if len(fields) >= 4 {
				wgInterfaces = append(wgInterfaces, map[string]interface{}{
					"interface":   fields[1],
					"listen_port": fields[2],
					"fwmark":      fields[3],
				})
			}
		case "WG_PEER":
			if len(fields) >= 9 {
				hsUnix, _ := strconv.ParseInt(fields[5], 10, 64)
				rxBytes, _ := strconv.ParseInt(fields[6], 10, 64)
				txBytes, _ := strconv.ParseInt(fields[7], 10, 64)
				var hsAgeSec int64 = -1
				alive := false
				hsTime := ""
				if hsUnix > 0 {
					hsAgeSec = now.Unix() - hsUnix
					if hsAgeSec < 0 {
						hsAgeSec = 0
					}
					alive = hsAgeSec <= 180
					hsTime = time.Unix(hsUnix, 0).UTC().Format(time.RFC3339)
				}

				allowedRaw := strings.Split(fields[4], ",")
				allowedHosts := []string{}
				for _, cidr := range allowedRaw {
					cidr = strings.TrimSpace(cidr)
					if cidr == "" {
						continue
					}
					ipStr := strings.SplitN(cidr, "/", 2)[0]
					if h := lookup.ResolveIP(ipStr); h != "" {
						allowedHosts = append(allowedHosts, h)
					} else if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
						for ipKey, name := range lookup.ByIP {
							if ip := net.ParseIP(ipKey); ip != nil && ipNet.Contains(ip) {
								allowedHosts = append(allowedHosts, name+" ("+ipKey+")")
							}
						}
					}
				}
				sort.Strings(allowedHosts)

				peer := map[string]interface{}{
					"interface":             fields[1],
					"peer_pubkey_prefix":    fields[2],
					"endpoint":              fields[3],
					"allowed_ips":           fields[4],
					"latest_handshake_at":   hsTime,
					"handshake_age_seconds": hsAgeSec,
					"tunnel_alive":          alive,
					"rx_bytes":              rxBytes,
					"tx_bytes":              txBytes,
					"rx_mb":                 mathRound2(float64(rxBytes) / (1024 * 1024)),
					"tx_mb":                 mathRound2(float64(txBytes) / (1024 * 1024)),
					"persistent_keepalive":  fields[8],
				}
				if len(allowedHosts) > 0 {
					peer["known_remote_hosts"] = allowedHosts
				}
				wgPeers = append(wgPeers, peer)
			}
		}
	}

	wgSummary := map[string]interface{}{
		"interfaces": wgInterfaces,
		"peers":      wgPeers,
		"peer_count": len(wgPeers),
	}

	mwanIfaces := []string{}
	mwanPolicies := map[string]string{}
	section := ""
	currentPolicy := ""
	for _, rawLine := range strings.Split(mwanPart, "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "Interface status:"):
			section = "interfaces"
			continue
		case strings.HasPrefix(trimmed, "Current ipv4 policies:"):
			section = "ipv4_policies"
			currentPolicy = ""
			continue
		case strings.HasPrefix(trimmed, "Current ipv6 policies:"), strings.HasPrefix(trimmed, "Directly connected"):
			section = "other"
			continue
		}
		if section == "interfaces" && strings.HasPrefix(trimmed, "interface ") {
			mwanIfaces = append(mwanIfaces, trimmed)
		} else if section == "ipv4_policies" {
			if strings.HasSuffix(trimmed, ":") {
				currentPolicy = strings.TrimSuffix(trimmed, ":")
			} else if currentPolicy != "" {
				if prev := mwanPolicies[currentPolicy]; prev != "" {
					mwanPolicies[currentPolicy] = prev + ", " + trimmed
				} else {
					mwanPolicies[currentPolicy] = trimmed
				}
			}
		}
	}
	mwanSummary := map[string]interface{}{
		"available":     len(mwanIfaces) > 0 || len(mwanPolicies) > 0,
		"interfaces":    mwanIfaces,
		"ipv4_policies": mwanPolicies,
	}

	return wgSummary, mwanSummary, starlinkProbe
}

func sentinelGetWANHealthTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	devices, lookup, err := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	if err != nil {
		return nil, err
	}

	// Never transmit WireGuard private_key ($2 on NF==5) or preshared_key ($3 on NF>=9).
	const wanLiveCmd = `wg show all dump 2>/dev/null | awk -F'\t' 'NF==5{print "WG_IFACE\t"$1"\t"$4"\t"$5} NF>=9{print "WG_PEER\t"$1"\t"substr($2,1,12)"\t"$4"\t"$5"\t"$6"\t"$7"\t"$8"\t"$9}'; echo '---MWAN3---'; mwan3 status 2>/dev/null | head -n 45; echo '---STARLINK_DISH---'; ping -c 2 -W 1 192.168.100.1 2>/dev/null || true; echo '---WAN_POP---'; ping -c 2 -W 1 1.1.1.1 2>/dev/null || true`

	starlinkLive := queryStarlinkHealthLive(ctx, siteID)
	starlinkAvailable, _ := starlinkLive["available"].(bool)

	gateways := []map[string]interface{}{}
	for _, dev := range devices {
		_, hasWAN := dev.State["wan"]
		_, hasStarlink := dev.State["starlink"]
		_, hasWG := dev.State["wireguard"]
		if !strings.EqualFold(dev.DeviceRole, "Gateway") && !hasWAN && !hasStarlink && !hasWG {
			continue
		}
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		gw := map[string]interface{}{
			"device_id":   dev.ID,
			"device_name": devName,
			"status":      dev.Status,
			"last_ip":     dev.LastIP,
			"wan":         dev.State["wan"],
			"starlink":    dev.State["starlink"],
			"wireguard":   dev.State["wireguard"],
			"system":      dev.State["system"],
		}
		if out, sshErr := orchestrator.ExecuteCommandWithOutput(invocation.Schema, dev.ID, wanLiveCmd); sshErr == nil {
			wgLive, mwanLive, slProbe := parseWANLiveStatus(out, time.Now(), lookup)
			gw["wireguard_live"] = wgLive
			gw["mwan3_live"] = mwanLive
			if !starlinkAvailable {
				if ok, _ := slProbe["available"].(bool); ok {
					starlinkLive = slProbe
					starlinkAvailable = true
				}
			}
		} else {
			gw["live_probe_error"] = sshErr.Error()
		}
		gateways = append(gateways, gw)
	}

	return map[string]interface{}{
		"site_id":       siteID,
		"starlink_live": starlinkLive,
		"gateway_count": len(gateways),
		"gateways":      gateways,
	}, nil
}

// ── 6b. get_firewall_summary ─────────────────────────────────────────────────

func trimUCIValue(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
		return s[1 : len(s)-1]
	}
	return s
}

func parseUCIFirewallSummary(raw string, lookup database.SiteClientHostnameLookup) map[string]interface{} {
	type sectionData struct {
		kind  string
		order int
		attrs map[string]string
	}
	sections := map[string]*sectionData{}
	order := 0

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "firewall.") {
			continue
		}
		rest := strings.TrimPrefix(line, "firewall.")
		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx <= 0 {
			continue
		}
		lhs := rest[:eqIdx]
		rhs := trimUCIValue(rest[eqIdx+1:])

		dotIdx := strings.IndexByte(lhs, '.')
		if dotIdx < 0 {
			sec := sections[lhs]
			if sec == nil {
				order++
				sec = &sectionData{kind: rhs, order: order, attrs: map[string]string{}}
				sections[lhs] = sec
			} else {
				sec.kind = rhs
			}
			continue
		}
		secName := lhs[:dotIdx]
		optName := lhs[dotIdx+1:]
		lowerOpt := strings.ToLower(optName)
		if strings.Contains(lowerOpt, "secret") || strings.Contains(lowerOpt, "password") || strings.Contains(lowerOpt, "key") {
			continue
		}
		sec := sections[secName]
		if sec == nil {
			order++
			sec = &sectionData{order: order, attrs: map[string]string{}}
			sections[secName] = sec
		}
		sec.attrs[optName] = rhs
	}

	ordered := make([]*sectionData, 0, len(sections))
	for _, s := range sections {
		ordered = append(ordered, s)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].order < ordered[j].order })

	defaults := map[string]string{}
	zones := []map[string]string{}
	forwardings := []map[string]string{}
	portForwards := []map[string]interface{}{}
	rules := []map[string]interface{}{}

	for _, sec := range ordered {
		switch sec.kind {
		case "defaults":
			for _, k := range []string{"input", "output", "forward", "synflood_protect", "flow_offloading", "flow_offloading_hw", "drop_invalid"} {
				if v, ok := sec.attrs[k]; ok {
					defaults[k] = v
				}
			}
		case "zone":
			z := map[string]string{
				"name":    sec.attrs["name"],
				"input":   sec.attrs["input"],
				"output":  sec.attrs["output"],
				"forward": sec.attrs["forward"],
			}
			if v := sec.attrs["network"]; v != "" {
				z["network"] = v
			}
			if v := sec.attrs["masq"]; v != "" {
				z["masq"] = v
			}
			zones = append(zones, z)
		case "forwarding":
			forwardings = append(forwardings, map[string]string{
				"src":  sec.attrs["src"],
				"dest": sec.attrs["dest"],
			})
		case "redirect":
			enabled := sec.attrs["enabled"] != "0"
			destIP := sec.attrs["dest_ip"]
			entry := map[string]interface{}{
				"name":      sec.attrs["name"],
				"enabled":   enabled,
				"src":       sec.attrs["src"],
				"src_dport": sec.attrs["src_dport"],
				"dest":      sec.attrs["dest"],
				"dest_ip":   destIP,
				"dest_port": sec.attrs["dest_port"],
				"proto":     sec.attrs["proto"],
				"target":    sec.attrs["target"],
			}
			if host := lookup.ResolveIP(destIP); host != "" {
				entry["dest_hostname"] = host
			}
			portForwards = append(portForwards, entry)
		case "rule":
			enabled := sec.attrs["enabled"] != "0"
			entry := map[string]interface{}{
				"name":    sec.attrs["name"],
				"enabled": enabled,
				"src":     sec.attrs["src"],
				"dest":    sec.attrs["dest"],
				"proto":   sec.attrs["proto"],
				"target":  sec.attrs["target"],
			}
			if v := sec.attrs["dest_port"]; v != "" {
				entry["dest_port"] = v
			}
			if v := sec.attrs["family"]; v != "" {
				entry["family"] = v
			}
			if v := sec.attrs["dest_ip"]; v != "" {
				entry["dest_ip"] = v
				if host := lookup.ResolveIP(v); host != "" {
					entry["dest_hostname"] = host
				}
			}
			rules = append(rules, entry)
		}
	}

	pfSummary := fmt.Sprintf("%d WAN-to-LAN port forwards (DNAT redirects) configured", len(portForwards))
	if len(portForwards) == 0 {
		pfSummary = "NONE (0 exposed WAN-to-LAN port forwards / DNAT redirects configured)"
	}

	return map[string]interface{}{
		"defaults":                      defaults,
		"zones":                         zones,
		"forwardings":                   forwardings,
		"port_forward_count":            len(portForwards),
		"exposed_port_forwards_summary": pfSummary,
		"port_forwards":                 portForwards,
		"rule_count":                    len(rules),
		"builtin_and_filter_rules":      rules,
		"note":                          "port_forwards are actual WAN->LAN exposed DNAT port forwards. builtin_and_filter_rules (such as Allow-DHCP-Renew UDP 68, Allow-DHCPv6 UDP 546, Allow-Ping, Allow-IPSec) are standard OpenWrt router filter rules, NOT exposed LAN port forwards.",
	}
}

func sentinelGetFirewallSummaryTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	deviceRef, _ := invocation.Args["device_id"].(string)

	devices, lookup, err := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	if err != nil {
		return nil, err
	}

	// Threat Shield status + per-device drop counters from DB
	threatEnabled := false
	_ = database.DB.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COALESCE(threat_shield_enabled, false) FROM %s.sites WHERE id = $1", invocation.Schema), siteID,
	).Scan(&threatEnabled)

	dropStats := []map[string]interface{}{}
	totalDrops := int64(0)
	if rows, qErr := database.DB.QueryContext(ctx,
		fmt.Sprintf("SELECT id, COALESCE(name, ''), COALESCE(threat_shield_drops, 0) FROM %s.devices WHERE site_id = $1 ORDER BY threat_shield_drops DESC, id ASC", invocation.Schema),
		siteID,
	); qErr == nil {
		stateByID := map[string]map[string]interface{}{}
		for _, d := range devices {
			stateByID[d.ID] = d.State
		}
		for rows.Next() {
			var devID, name string
			var drops int64
			if rows.Scan(&devID, &name, &drops) == nil {
				totalDrops += drops
				dropStats = append(dropStats, map[string]interface{}{
					"device_id":   devID,
					"device_name": database.DeviceDisplayName(devID, &name, stateByID[devID], lookup),
					"drops":       drops,
				})
			}
		}
		rows.Close()
	}

	// Select gateway(s) or explicit device for live UCI firewall inspection
	targets := []sentinelSiteDeviceRow{}
	if strings.TrimSpace(deviceRef) != "" {
		dev, err := resolveSentinelManagedDevice(ctx, invocation.Schema, siteID, deviceRef)
		if err != nil {
			return nil, err
		}
		targets = []sentinelSiteDeviceRow{dev}
	} else {
		for _, dev := range devices {
			if strings.EqualFold(dev.DeviceRole, "Gateway") {
				targets = append(targets, dev)
			}
		}
		if len(targets) == 0 && len(devices) > 0 {
			targets = []sentinelSiteDeviceRow{devices[0]}
		}
	}

	nodeFirewalls := make([]map[string]interface{}, 0, len(targets))
	for _, dev := range targets {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		entry := map[string]interface{}{
			"device_id":   dev.ID,
			"device_name": devName,
			"last_ip":     dev.LastIP,
		}
		if out, sshErr := orchestrator.ExecuteCommandWithOutput(invocation.Schema, dev.ID, "uci show firewall 2>/dev/null"); sshErr == nil {
			entry["firewall"] = parseUCIFirewallSummary(out, lookup)
		} else {
			entry["error"] = sshErr.Error()
		}
		nodeFirewalls = append(nodeFirewalls, entry)
	}

	return map[string]interface{}{
		"site_id": siteID,
		"threat_shield": map[string]interface{}{
			"enabled":      threatEnabled,
			"total_drops":  totalDrops,
			"intel":        GetThreatIntelStatus(),
			"device_drops": dropStats,
		},
		"nodes": nodeFirewalls,
	}, nil
}

// ── 7. get_wifi_config_summary ───────────────────────────────────────────────

func sanitizeWirelessUCIConfig(raw string) string {
	var clean []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, ".key=") ||
			strings.Contains(lower, ".auth_secret=") ||
			strings.Contains(lower, ".acct_secret=") ||
			strings.Contains(lower, ".dae_secret=") ||
			strings.Contains(lower, ".r0kh=") ||
			strings.Contains(lower, ".r1kh=") ||
			strings.Contains(lower, "passphrase") ||
			strings.Contains(lower, "password") {
			continue
		}
		clean = append(clean, trimmed)
	}
	return strings.Join(clean, "\n")
}

func sentinelGetWiFiConfigSummaryTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	deviceRef, _ := invocation.Args["device_id"].(string)

	devices, lookup, err := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	if err != nil {
		return nil, err
	}

	targets := devices
	if strings.TrimSpace(deviceRef) != "" {
		dev, err := resolveSentinelManagedDevice(ctx, invocation.Schema, siteID, deviceRef)
		if err != nil {
			return nil, err
		}
		targets = []sentinelSiteDeviceRow{dev}
	}

	const readCmd = `uci show wireless | grep -vE '\.(key|auth_secret|acct_secret|dae_secret|r0kh|r1kh)='; echo '---IW---'; iw dev | grep -E 'Interface|ssid|channel|txpower'`
	nodesOut := make([]map[string]interface{}, 0, len(targets))
	for _, dev := range targets {
		devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		out, err := orchestrator.ExecuteCommandWithOutput(invocation.Schema, dev.ID, readCmd)
		entry := map[string]interface{}{
			"device_id":   dev.ID,
			"device_name": devName,
			"last_ip":     dev.LastIP,
		}
		if err != nil {
			entry["error"] = err.Error()
		} else {
			entry["wireless_config"] = sanitizeWirelessUCIConfig(out)
		}
		nodesOut = append(nodesOut, entry)
	}

	return map[string]interface{}{
		"site_id": siteID,
		"nodes":   nodesOut,
	}, nil
}

// ── 8. ping_host ─────────────────────────────────────────────────────────────

func parsePingSummary(output string) map[string]interface{} {
	res := map[string]interface{}{
		"reachable": false,
	}
	if m := sentinelPingStatsRe.FindStringSubmatch(output); len(m) == 4 {
		tx, _ := strconv.Atoi(m[1])
		rx, _ := strconv.Atoi(m[2])
		loss, _ := strconv.ParseFloat(m[3], 64)
		res["transmitted"] = tx
		res["received"] = rx
		res["packet_loss_pct"] = loss
		res["reachable"] = rx > 0
	}
	if m := sentinelPingRTTRe.FindStringSubmatch(output); len(m) == 4 {
		minMs, _ := strconv.ParseFloat(m[1], 64)
		avgMs, _ := strconv.ParseFloat(m[2], 64)
		maxMs, _ := strconv.ParseFloat(m[3], 64)
		res["min_ms"] = minMs
		res["avg_ms"] = avgMs
		res["max_ms"] = maxMs
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	res["summary"] = strings.Join(lines, "\n")
	return res
}

func sentinelPingHostTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	target, _ := invocation.Args["target"].(string)
	target = strings.TrimSpace(target)
	if !isValidSentinelProbeHost(target) {
		return nil, fmt.Errorf("target must be a valid IP address or hostname")
	}
	count := 4
	if rawCount, ok := invocation.Args["count"].(float64); ok && rawCount >= 1 && rawCount <= 5 {
		count = int(rawCount)
	}
	fromDevice, _ := invocation.Args["from_device_id"].(string)
	siteID, _ := invocation.Args["site_id"].(string)

	var out string
	var fromLabel string

	if strings.TrimSpace(fromDevice) != "" {
		dev, err := resolveSentinelManagedDevice(ctx, invocation.Schema, siteID, fromDevice)
		if err != nil {
			return nil, err
		}
		_, lookup, _ := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
		fromLabel = database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		cmd := fmt.Sprintf("ping -c %d -W 2 %s", count, target)
		rawOut, runErr := orchestrator.ExecuteCommandWithOutput(invocation.Schema, dev.ID, cmd)
		out = rawOut
		if runErr != nil && out == "" {
			return map[string]interface{}{
				"target":    target,
				"from":      fromLabel,
				"reachable": false,
				"error":     runErr.Error(),
			}, nil
		}
	} else {
		fromLabel = "controller"
		cmd := exec.CommandContext(ctx, "ping", "-n", "-c", strconv.Itoa(count), "-W", "2", target)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		_ = cmd.Run()
		out = buf.String()
	}

	parsed := parsePingSummary(out)
	parsed["target"] = target
	parsed["from"] = fromLabel
	return parsed, nil
}

// ── 9. dns_lookup ────────────────────────────────────────────────────────────

func sentinelDNSLookupTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	domain, _ := invocation.Args["domain"].(string)
	domain = strings.TrimSpace(domain)
	if domain == "" || !sentinelHostnamePattern.MatchString(domain) || net.ParseIP(domain) != nil {
		return nil, fmt.Errorf("domain must be a valid DNS hostname")
	}
	server, _ := invocation.Args["server"].(string)
	server = strings.TrimSpace(server)
	siteID, _ := invocation.Args["site_id"].(string)

	if server != "" {
		if net.ParseIP(server) == nil {
			return nil, fmt.Errorf("server must be a valid IPv4 or IPv6 address")
		}
	} else {
		// Default to site gateway (AdGuardHome on .1:53) when available
		if gwIP := resolveSentinelGatewayIP(ctx, invocation.Schema, siteID); gwIP != "" {
			server = gwIP
		}
	}

	resolver := net.DefaultResolver
	serverLabel := "system_default"
	if server != "" {
		serverLabel = server + ":53"
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(dctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(dctx, network, net.JoinHostPort(server, "53"))
			},
		}
	}

	started := time.Now()
	lookupCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ips, err := resolver.LookupHost(lookupCtx, domain)
	latencyMs := time.Since(started).Milliseconds()

	blockedHint := false
	for _, ip := range ips {
		if ip == "0.0.0.0" || ip == "::" {
			blockedHint = true
		}
	}

	res := map[string]interface{}{
		"domain":          domain,
		"server":          serverLabel,
		"latency_ms":      latencyMs,
		"resolved":        err == nil && len(ips) > 0,
		"ips":             ips,
		"adguard_blocked": blockedHint,
	}
	if err != nil {
		res["error"] = err.Error()
	}
	return res, nil
}

// ── 10. probe_tcp_port ───────────────────────────────────────────────────────

func sentinelProbeTCPPortTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	host, _ := invocation.Args["host"].(string)
	host = strings.TrimSpace(host)
	if !isValidSentinelProbeHost(host) {
		return nil, fmt.Errorf("host must be a valid private LAN IP or local hostname")
	}
	portFloat, ok := invocation.Args["port"].(float64)
	if !ok || portFloat < 1 || portFloat > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	port := int(portFloat)

	timeoutMs := 1500
	if rawT, ok := invocation.Args["timeout_ms"].(float64); ok && rawT >= 200 && rawT <= 3000 {
		timeoutMs = int(rawT)
	}

	// Safety boundary: restrict TCP probing to private/LAN/local addresses only
	var targetIP net.IP
	if ip := net.ParseIP(host); ip != nil {
		targetIP = ip
	} else {
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(resolved) == 0 {
			return map[string]interface{}{
				"host":      host,
				"port":      port,
				"reachable": false,
				"error":     fmt.Sprintf("could not resolve host %q", host),
			}, nil
		}
		targetIP = resolved[0].IP
	}
	if !isPrivateOrLocalIP(targetIP) {
		return nil, fmt.Errorf("probe_tcp_port is restricted to private LAN / local addresses; %s is public", targetIP.String())
	}

	started := time.Now()
	dialer := &net.Dialer{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(targetIP.String(), strconv.Itoa(port)))
	latencyMs := time.Since(started).Milliseconds()
	if err != nil {
		return map[string]interface{}{
			"host":       host,
			"ip":         targetIP.String(),
			"port":       port,
			"reachable":  false,
			"latency_ms": latencyMs,
			"error":      err.Error(),
		}, nil
	}
	_ = conn.Close()
	return map[string]interface{}{
		"host":       host,
		"ip":         targetIP.String(),
		"port":       port,
		"reachable":  true,
		"latency_ms": latencyMs,
	}, nil
}

// ── 11. traceroute_host ──────────────────────────────────────────────────────

func parseTracerouteHops(output string) []map[string]interface{} {
	hops := []map[string]interface{}{}
	for _, line := range strings.Split(output, "\n") {
		m := sentinelTraceHopRe.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		hopNum, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		entry := map[string]interface{}{
			"hop": hopNum,
			"ip":  m[2],
		}
		if len(m) >= 4 && m[3] != "" {
			if rtt, err := strconv.ParseFloat(m[3], 64); err == nil {
				entry["rtt_ms"] = rtt
			}
		}
		hops = append(hops, entry)
	}
	return hops
}

func sentinelTracerouteHostTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	target, _ := invocation.Args["target"].(string)
	target = strings.TrimSpace(target)
	if !isValidSentinelProbeHost(target) {
		return nil, fmt.Errorf("target must be a valid IP address or hostname")
	}
	maxHops := 12
	if rawH, ok := invocation.Args["max_hops"].(float64); ok && rawH >= 1 && rawH <= 15 {
		maxHops = int(rawH)
	}
	fromDevice, _ := invocation.Args["from_device_id"].(string)
	siteID, _ := invocation.Args["site_id"].(string)

	var out string
	fromLabel := "controller"
	if strings.TrimSpace(fromDevice) != "" {
		dev, err := resolveSentinelManagedDevice(ctx, invocation.Schema, siteID, fromDevice)
		if err != nil {
			return nil, err
		}
		_, lookup, _ := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
		fromLabel = database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)
		cmd := fmt.Sprintf("traceroute -n -m %d -w 1 -q 1 %s", maxHops, target)
		rawOut, runErr := orchestrator.ExecuteCommandWithOutput(invocation.Schema, dev.ID, cmd)
		out = rawOut
		if runErr != nil && out == "" {
			return map[string]interface{}{
				"target": target,
				"from":   fromLabel,
				"error":  runErr.Error(),
			}, nil
		}
	} else {
		cmd := exec.CommandContext(ctx, "traceroute", "-n", "-m", strconv.Itoa(maxHops), "-w", "1", "-q", "1", target)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		_ = cmd.Run()
		out = buf.String()
	}

	return map[string]interface{}{
		"target": target,
		"from":   fromLabel,
		"hops":   parseTracerouteHops(out),
	}, nil
}

// ── 12. run_iperf3_test ──────────────────────────────────────────────────────

func sentinelRunIperf3TestTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	deviceRef, _ := invocation.Args["device_id"].(string)
	if strings.TrimSpace(deviceRef) == "" {
		return nil, fmt.Errorf("device_id (node name, MAC, or IP) is required")
	}
	siteID, _ := invocation.Args["site_id"].(string)
	dev, err := resolveSentinelManagedDevice(ctx, invocation.Schema, siteID, deviceRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(dev.LastIP) == "" {
		return nil, fmt.Errorf("managed device %s has no known management IP", dev.ID)
	}
	_, lookup, _ := loadSentinelSiteDevices(ctx, invocation.Schema, siteID)
	devName := database.DeviceDisplayName(dev.ID, &dev.Name, dev.State, lookup)

	// Guard against link saturation with a per-device cooldown
	cooldownKey := invocation.Schema + "|" + dev.ID
	now := time.Now()
	sentinelIperfCooldownMu.Lock()
	if last, ok := sentinelIperfLastRun[cooldownKey]; ok && now.Sub(last) < sentinelIperfCooldown {
		remaining := (sentinelIperfCooldown - now.Sub(last)).Round(time.Second)
		sentinelIperfCooldownMu.Unlock()
		return map[string]interface{}{
			"device_id":   dev.ID,
			"device_name": devName,
			"skipped":     true,
			"reason":      fmt.Sprintf("iperf3 cooldown active for %s; retry in %s", devName, remaining),
		}, nil
	}
	sentinelIperfLastRun[cooldownKey] = now
	sentinelIperfCooldownMu.Unlock()

	bench := RunNodeBenchmark(ctx, dev.ID, devName, dev.LastIP, GetBaselineForDevice(dev.ID, dev.LastIP))
	return map[string]interface{}{
		"device_id":       bench.DeviceID,
		"device_name":     bench.DeviceName,
		"target_ip":       bench.IP,
		"throughput_mbps": mathRound2(bench.ThroughputMbps),
		"retransmits":     bench.Retransmits,
		"latency_ms":      mathRound2(bench.LatencyMs),
		"status":          bench.Status,
		"assessment":      bench.Assessment,
		"error":           bench.Error,
	}, nil
}

func stringField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func mathRound2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
