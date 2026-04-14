package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
	"openwrt-controller/internal/services"
)

func TelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "Bad request: invalid json", http.StatusBadRequest)
		return
	}

	deviceID, ok := raw["device_id"].(string)
	if !ok || deviceID == "" {
		http.Error(w, "Bad request: missing device_id", http.StatusBadRequest)
		return
	}

	providedKey := r.Header.Get("X-Site-Key")
	
	log.Printf("[DEBUG] Telemetry received from device_id=%s, IP=%s, X-Site-Key=%s", deviceID, r.RemoteAddr, providedKey)

	if providedKey == "" {
		http.Error(w, "Forbidden: missing site key", http.StatusForbidden)
		return
	}

	tenantSchema, err := database.GetTenantSchemaForSiteKey(providedKey)
	if err != nil {
		http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
		return
	}

	var siteKey *string
	err = database.DB.QueryRow(`
		SELECT s.api_key FROM `+tenantSchema+`.sites s 
		JOIN `+tenantSchema+`.devices d ON d.site_id = s.id 
		WHERE d.id = $1`, deviceID).Scan(&siteKey)
	if err == nil && siteKey != nil && *siteKey != "" {
		if providedKey != *siteKey {
			http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
			return
		}
	}

	// ── ZERO_TOUCH: Auto-Adoption ─────────────────────────────────────────────
	// If the device has no site_id yet, check if the X-Site-Key matches a site
	// with auto_adopt=true. If so, adopt the device automatically.
	if err != nil && providedKey != "" {
		var autoSiteID string
		var autoAdopt bool
		zeroTouchErr := database.DB.QueryRow(`
			SELECT id, auto_adopt FROM `+tenantSchema+`.sites WHERE api_key = $1
		`, providedKey).Scan(&autoSiteID, &autoAdopt)

		if zeroTouchErr == nil && autoAdopt {
			_, _ = database.DB.Exec(
				"UPDATE "+tenantSchema+".devices SET site_id = $1, status = 'Adopted' WHERE id = $2",
				autoSiteID, deviceID,
			)
			log.Printf("[ZERO_TOUCH] Device %s auto-adopted to site %s", deviceID, autoSiteID)
			go database.InsertAuditLog("system", "ZERO_TOUCH_ADOPTION", "DEVICE", deviceID, "auto-adopted to site: "+autoSiteID, r.RemoteAddr)
		}
	}
	// ─────────────────────────────────────────────────────────────────────────

	modelStr := "UNKNOWN"
	if boardInfo, ok := raw["board"].(map[string]interface{}); ok {
		if model, ok := boardInfo["model"].(string); ok {
			modelStr = model
		}
	}
	
	agentVersion := ""
	if v, ok := raw["agent_version"].(string); ok {
		agentVersion = v
	}

	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}

	// 1. Goroutine for PostgreSQL (upsert state and explicit model)
	go func(devID string, state []byte, mod string, ip string, av string, schema string) {
		if err := database.UpsertDeviceState(schema, devID, state, mod, ip, av); err != nil {
			log.Printf("Error upserting device state to postgres: %v\n", err)
		}
	}(deviceID, body, modelStr, remoteIP, agentVersion, tenantSchema)

	// Extract Metrics cleanly bypassing struct matching
	var metrics models.DeviceMetrics
	if sys, ok := raw["system"].(map[string]interface{}); ok {
		if loadArr, ok := sys["load"].([]interface{}); ok && len(loadArr) > 0 {
			if v, ok := loadArr[0].(float64); ok {
				metrics.CPULoad = v / 65536.0
			}
		}
		if mem, ok := sys["memory"].(map[string]interface{}); ok {
			if free, ok := mem["free"].(float64); ok {
				metrics.RAMFree = int64(free)
			}
		}
		if uptime, ok := sys["uptime"].(float64); ok {
			metrics.Uptime = int64(uptime)
		}
	}

	var totalSignal float64
	var totalRx float64
	var totalTx float64
	var clientCount int

	if wStations, ok := raw["wireless_stations"].(map[string]interface{}); ok {
		for _, clientsList := range wStations {
			if clients, ok := clientsList.([]interface{}); ok {
				clientCount += len(clients)
				for _, cIf := range clients {
					if cMap, ok := cIf.(map[string]interface{}); ok {
						if sig, ok := cMap["signal"].(float64); ok {
							totalSignal += sig
						}
						// rx_rate and tx_rate come as strings like "130.0" from awk
						if rxStr, ok := cMap["rx_rate"].(string); ok {
							if rx, err := strconv.ParseFloat(rxStr, 64); err == nil {
								totalRx += rx
							}
						}
						if txStr, ok := cMap["tx_rate"].(string); ok {
							if tx, err := strconv.ParseFloat(txStr, 64); err == nil {
								totalTx += tx
							}
						}
					}
				}
			}
		}
	}
	
	if clientCount > 0 {
		metrics.SignalDBM = totalSignal / float64(clientCount)
	} else {
		metrics.SignalDBM = 0
	}
	metrics.RxMbps = totalRx
	metrics.TxMbps = totalTx

	// 2. Goroutine for InfluxDB (metrics)
	go func(devID string, mets models.DeviceMetrics) {
		if err := database.WriteMetrics(devID, &mets); err != nil {
			log.Printf("Error writing metrics to influx: %v\n", err)
		}
	}(deviceID, metrics)

	// 3. Process logs
	if logsStr, ok := raw["logs"].(string); ok && logsStr != "" {
		lines := strings.Split(logsStr, "\n")
		var parsedLogs []database.LogEntry
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			severity := "INFO"
			if strings.Contains(lower, "warn") {
				severity = "WARN"
			}
			if strings.Contains(lower, "err") || strings.Contains(lower, "fail") || strings.Contains(lower, "panic") || strings.Contains(lower, "crit") || strings.Contains(lower, "auth.error") {
				severity = "ERROR"
			}

			// Parse the syslog timestamp from the log line.
			// OpenWrt logread emits lines like:
			//   "Mon Jan  2 15:04:05 2006 hostname daemon.info process: msg"
			// We try several common formats. The reference time fields MUST match
			// Go's magic reference (Mon=Mon Jan=Jan 2=2 15=15 04=04 05=05 2006=2006).
			syslogFormats := []string{
				"Mon Jan _2 15:04:05 2006", // OpenWrt default with year
				"Mon Jan  2 15:04:05 2006", // double-space variant
				"Jan _2 15:04:05",          // RFC3164 without year
				"Jan  2 15:04:05",          // RFC3164 double-space
				"2006-01-02T15:04:05Z07:00", // ISO-8601
				"2006-01-02 15:04:05",       // SQL-ish
			}
			timestamp := time.Now().UTC().Format(time.RFC3339)
			// Try to parse the leading portion of the line
			for _, fmt := range syslogFormats {
				prefixLen := len(fmt)
				if len(line) >= prefixLen {
					if t, err := time.Parse(fmt, line[:prefixLen]); err == nil {
						// For formats without a year, assume the current year
						if t.Year() == 0 {
							t = t.AddDate(time.Now().Year(), 0, 0)
						}
						timestamp = t.UTC().Format(time.RFC3339)
						break
					}
				}
			}

			parsedLogs = append(parsedLogs, database.LogEntry{
				Timestamp: timestamp,
				Level:     severity,
				Message:   line,
			})
		}

		go func(devID string, logs []database.LogEntry, schema string) {
			if err := database.InsertDeviceLogs(schema, devID, logs); err != nil {
				log.Printf("Error inserting logs: %v\n", err)
			}
			services.AnalyzeLogs(devID, logs)
		}(deviceID, parsedLogs, tenantSchema)
	}

	// 4. The Signal (Alerts Evaluation)
	var sID string
	_ = database.DB.QueryRow("SELECT site_id FROM "+tenantSchema+".devices WHERE id = $1", deviceID).Scan(&sID)
	go services.ProcessTelemetry(deviceID, sID, metrics)

	// 5. FLOW_SENSE — process conntrack snapshot
	if rawFlows, ok := raw["flow_sense"].([]interface{}); ok && len(rawFlows) > 0 {
		controllerIP := r.Host
		if idx := strings.LastIndex(controllerIP, ":"); idx != -1 {
			controllerIP = controllerIP[:idx]
		}
		go func(devID string, flows []interface{}, ctrlIP string, rawPayload []byte, schema string) {
			enriched := services.ProcessFlowSense(devID, flows, ctrlIP)
			if len(enriched) == 0 {
				return
			}
			
			// Extract flow analytics for dashboard matrix
			var analytics []database.FlowAnalytic
			for _, e := range enriched {
				// Only keep external traffic flows for analytics, or we take all
				// e.Dst is already filtered by non-private whitelist in ProcessFlowSense, so enriched contains mainly outbound.
				if e.SampleSrc != "" {
					analytics = append(analytics, database.FlowAnalytic{
						MAC:   e.SampleSrc,
						Port:  e.Dport,
						Conns: e.Conns,
					})
				}
			}
			if len(analytics) > 0 {
				if err := database.WriteFlowAnalyticsBatch(devID, analytics); err != nil {
					log.Printf("Error writing flow analytics to influx: %v\n", err)
				}
			}

			enrichedJSON, err := json.Marshal(enriched)
			if err != nil {
				return
			}
			// Merge enriched flow_sense back into state_json for querying
			var state map[string]interface{}
			if err := json.Unmarshal(rawPayload, &state); err == nil {
				var enrichedList []interface{}
				if err2 := json.Unmarshal(enrichedJSON, &enrichedList); err2 == nil {
					state["flow_sense"] = enrichedList
					if merged, err3 := json.Marshal(state); err3 == nil {
						rawPayload = merged
					}
				}
			}
			database.DB.Exec(
				"UPDATE "+schema+".devices SET state_json = $1 WHERE id = $2",
				rawPayload, devID,
			)
		}(deviceID, rawFlows, controllerIP, body, tenantSchema)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}
