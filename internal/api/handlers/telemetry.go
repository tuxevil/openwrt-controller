package handlers

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
	"openwrt-controller/internal/services"
)

const telemetryPersistenceTimeout = 10 * time.Second

func telemetryPersistenceContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), telemetryPersistenceTimeout)
}

func validateDeviceTelemetryToken(storedToken, providedToken string) error {
	if storedToken == "" {
		if providedToken == "" {
			return nil
		}
		return fmt.Errorf("device token is not initialized")
	}
	if providedToken == "" || subtle.ConstantTimeCompare([]byte(providedToken), []byte(storedToken)) != 1 {
		return fmt.Errorf("invalid device token")
	}
	return nil
}

func validateDeviceTelemetryTokenForMode(storedToken, providedToken string, legacy bool) error {
	if storedToken != "" && providedToken == "" && legacy {
		return nil
	}
	return validateDeviceTelemetryToken(storedToken, providedToken)
}

func TelemetryHandler(w http.ResponseWriter, r *http.Request) {
	// Method check is redundant — routes.go registers POST only.
	// Kept for explicitness; remove if a future refactor relies solely on the mux.

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
	canonicalDeviceID, err := normalizeEnrollmentDeviceID(deviceID)
	if err != nil {
		http.Error(w, "Bad request: invalid device_id", http.StatusBadRequest)
		return
	}
	deviceID = canonicalDeviceID

	providedKey := r.Header.Get("X-Site-Key")
	providedToken := r.Header.Get("X-Device-Token")

	// Never write the site key to logs: it authenticates every device in the
	// site and is sufficient to pull the agent/config endpoints.
	log.Printf("[DEBUG] Telemetry received from device_id=%s, IP=%s, site_key_present=%t, device_token_present=%t", deviceID, r.RemoteAddr, providedKey != "", providedToken != "")

	if providedKey == "" && providedToken == "" {
		http.Error(w, "Forbidden: missing device credentials", http.StatusForbidden)
		return
	}
	if providedToken == "" && !allowLegacyProvision() {
		http.Error(w, "Forbidden: X-Device-Token header is required", http.StatusForbidden)
		return
	}

	tenantSchema, authToken, err := resolveDeviceTenant(providedToken, providedKey)
	if err != nil {
		http.Error(w, "Forbidden: invalid device credentials", http.StatusForbidden)
		return
	}

	var siteKey *string
	var storedDeviceToken *string
	var deviceSiteID *string
	var storedDeviceID string
	err = database.Tx(r.Context()).QueryRow(`
		SELECT d.id, d.site_id, s.api_key, d.device_token FROM `+tenantSchema+`.devices d
		LEFT JOIN `+tenantSchema+`.sites s ON d.site_id = s.id
		WHERE LOWER(d.id) = LOWER($1)`, deviceID).Scan(&storedDeviceID, &deviceSiteID, &siteKey, &storedDeviceToken)
	if err == sql.ErrNoRows {
		if authToken != "" {
			err = database.Tx(r.Context()).QueryRow(`
				SELECT d.id, d.site_id, s.api_key, d.device_token FROM `+tenantSchema+`.devices d
				LEFT JOIN `+tenantSchema+`.sites s ON d.site_id = s.id
				WHERE d.device_token = $1`, authToken).Scan(&storedDeviceID, &deviceSiteID, &siteKey, &storedDeviceToken)
		}
		if err == sql.ErrNoRows && (authToken != "" || !allowLegacyProvision()) {
			http.Error(w, "Forbidden: unknown device", http.StatusForbidden)
			return
		}
		if err == sql.ErrNoRows {
			remoteIP := requestRemoteIP(r.RemoteAddr)
			var legacyID string
			legacyErr := database.Tx(r.Context()).QueryRow(`
				SELECT id FROM `+tenantSchema+`.devices
				WHERE site_id IS NOT NULL AND last_ip = $1
				ORDER BY last_seen_at DESC NULLS LAST LIMIT 1`, remoteIP).Scan(&legacyID)
			if legacyErr != nil {
				http.Error(w, "Forbidden: unknown device", http.StatusForbidden)
				return
			}
			canonicalDeviceID = legacyID
			err = database.Tx(r.Context()).QueryRow(`
				SELECT d.id, d.site_id, s.api_key, d.device_token FROM `+tenantSchema+`.devices d
				LEFT JOIN `+tenantSchema+`.sites s ON d.site_id = s.id
				WHERE LOWER(d.id) = LOWER($1)`, canonicalDeviceID).Scan(&storedDeviceID, &deviceSiteID, &siteKey, &storedDeviceToken)
		}
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if storedDeviceID != "" {
		canonicalDeviceID = storedDeviceID
	}
	// From this point on every stateful processor must use the canonical
	// controller identity, not a transient bridge MAC reported by the agent.
	deviceID = canonicalDeviceID
	assigned := deviceSiteID != nil && *deviceSiteID != ""
	storedToken := ""
	if storedDeviceToken != nil {
		storedToken = *storedDeviceToken
	}
	if assigned {
		if siteKey == nil || *siteKey == "" {
			http.Error(w, "Forbidden: device site is not configured", http.StatusForbidden)
			return
		}
		if authToken == "" && providedKey != "" && subtle.ConstantTimeCompare([]byte(providedKey), []byte(*siteKey)) != 1 {
			http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
			return
		}
		if storedToken == "" {
			if authToken != "" || (!allowLegacyProvision() && providedKey == "") {
				http.Error(w, "Forbidden: device token is not initialized", http.StatusForbidden)
				return
			}
		}
		if storedToken != "" && validateDeviceTelemetryTokenForMode(storedToken, authToken, allowLegacyProvision()) != nil {
			http.Error(w, "Forbidden: invalid device token", http.StatusForbidden)
			return
		}
	} else if storedToken != "" {
		if err := validateDeviceTelemetryTokenForMode(storedToken, authToken, allowLegacyProvision()); err != nil {
			http.Error(w, "Forbidden: invalid device token", http.StatusForbidden)
			return
		}
	} else if authToken != "" {
		http.Error(w, "Forbidden: invalid device token", http.StatusForbidden)
		return
	}

	// ── ZERO_TOUCH: Auto-Adoption ─────────────────────────────────────────────
	// If the device has no site_id yet, check if the X-Site-Key matches a site
	// with auto_adopt=true. If so, adopt the device automatically.
	if !assigned && storedToken == "" && providedKey != "" {
		var autoSiteID string
		var autoAdopt bool
		zeroTouchErr := database.Tx(r.Context()).QueryRow(`
			SELECT id, auto_adopt FROM `+tenantSchema+`.sites WHERE api_key = $1
		`, providedKey).Scan(&autoSiteID, &autoAdopt)

		if zeroTouchErr == nil && autoAdopt {
			result, updateErr := database.Tx(r.Context()).Exec(
				"UPDATE "+tenantSchema+".devices SET site_id = $1, status = 'Adopted' WHERE id = $2 AND site_id IS NULL",
				autoSiteID, deviceID,
			)
			if updateErr == nil {
				if affected, rowsErr := result.RowsAffected(); rowsErr == nil && affected == 1 {
					log.Printf("[ZERO_TOUCH] Device %s auto-adopted to site %s", deviceID, autoSiteID)
					go database.InsertAuditLog("system", "ZERO_TOUCH_ADOPTION", "DEVICE", deviceID, "auto-adopted to site: "+autoSiteID, r.RemoteAddr)
				}
			}
		}
	}
	// ─────────────────────────────────────────────────────────────────────────

	modelStr := "UNKNOWN"
	if boardInfo, ok := raw["board"].(map[string]interface{}); ok {
		if model, ok := boardInfo["model"].(string); ok {
			modelStr = model
		}
	}
	capabilities, hasCapabilities := raw["capabilities"]
	capabilityPayload := []byte(nil)
	if hasCapabilities {
		if _, ok := capabilities.(map[string]interface{}); !ok {
			http.Error(w, "Bad request: capabilities must be an object", http.StatusBadRequest)
			return
		}
		capabilityPayload, err = json.Marshal(capabilities)
		if err != nil {
			http.Error(w, "Bad request: invalid capabilities", http.StatusBadRequest)
			return
		}
	}
	if operationStatus, ok := raw["transaction"].(map[string]interface{}); ok {
		if operationStatus["id"] != nil && operationStatus["state"] != nil {
			if statusPayload, marshalErr := json.Marshal(operationStatus); marshalErr == nil {
				statusContext, cancelStatusPersistence := telemetryPersistenceContext(r)
				go func(ctx context.Context, cancel context.CancelFunc, devID, schema string, status []byte) {
					defer cancel()
					if err := database.RecordDeviceOperationStatus(ctx, schema, devID, status); err != nil {
						log.Printf("Error persisting device operation status: %v", err)
					}
				}(statusContext, cancelStatusPersistence, canonicalDeviceID, tenantSchema, statusPayload)
			}
		}
	}

	agentVersion := ""
	if v, ok := raw["agent_version"].(string); ok {
		agentVersion = v
	}

	remoteIP := requestRemoteIP(r.RemoteAddr)

	// 1. Goroutine for PostgreSQL (upsert state and explicit model)
	go func(devID string, state, capabilities []byte, mod string, ip string, av string, schema string) {
		if err := database.UpsertDeviceState(schema, devID, state, mod, ip, av); err != nil {
			log.Printf("Error upserting device state to postgres: %v\n", err)
		}
		if len(capabilities) > 0 {
			if _, err := database.DB.Exec("UPDATE "+schema+".devices SET capabilities = $1, capabilities_updated_at = CURRENT_TIMESTAMP WHERE id = $2", capabilities, devID); err != nil {
				log.Printf("Error persisting device capabilities: %v", err)
			}
		}
	}(canonicalDeviceID, body, capabilityPayload, modelStr, remoteIP, agentVersion, tenantSchema)

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

	// 2b. WIFI_SURVEY: when a survey_id is attached to the payload, write
	//     per-station signal samples to the client_signal measurement so
	//     the correlation worker can pair them with phone GPS samples.
	//     This is the only path that gets the signal into InfluxDB at the
	//     2s survey cadence without doubling the telemetry cost.
	if surveyID, _ := raw["survey_id"].(string); surveyID != "" {
		go func(devID string, payload map[string]interface{}, sID string) {
			stations, _ := payload["wireless_stations"].(map[string]interface{})
			if len(stations) == 0 {
				return
			}
			var samples []database.ClientSignalSample
			now := time.Now()
			for iface, ifaceClients := range stations {
				list, _ := ifaceClients.([]interface{})
				for _, cIf := range list {
					cMap, _ := cIf.(map[string]interface{})
					mac, _ := cMap["mac"].(string)
					if mac == "" {
						continue
					}
					sig, _ := cMap["signal"].(float64)
					noise, _ := cMap["noise"].(float64)
					if sig == 0 {
						continue
					}
					rxStr, _ := cMap["rx_rate"].(string)
					txStr, _ := cMap["tx_rate"].(string)
					inactF, _ := cMap["inactive"].(float64)
					rx, _ := strconv.ParseFloat(rxStr, 64)
					tx, _ := strconv.ParseFloat(txStr, 64)
					_ = iface
					samples = append(samples, database.ClientSignalSample{
						DeviceID:   devID,
						MAC:        mac,
						SurveyID:   sID,
						SignalDBM:  sig,
						NoiseDBM:   noise,
						RxRate:     rx,
						TxRate:     tx,
						InactiveMs: int64(inactF),
						Time:       now,
					})
				}
			}
			if len(samples) == 0 {
				return
			}
			if err := database.WriteClientSignalBatch(samples); err != nil {
				log.Printf("[SURVEY] WriteClientSignalBatch failed for device %s: %v", devID, err)
			}
		}(deviceID, raw, surveyID)
	}

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
				"Mon Jan _2 15:04:05 2006",  // OpenWrt default with year
				"Mon Jan  2 15:04:05 2006",  // double-space variant
				"Jan _2 15:04:05",           // RFC3164 without year
				"Jan  2 15:04:05",           // RFC3164 double-space
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
			services.AnalyzeLogs(tenantSchema, devID, logs)
		}(deviceID, parsedLogs, tenantSchema)
	}

	// 4. The Signal (Alerts Evaluation)
	var sID string
	_ = database.Tx(r.Context()).QueryRow("SELECT site_id FROM "+tenantSchema+".devices WHERE id = $1", deviceID).Scan(&sID)
	go services.ProcessTelemetry(tenantSchema, deviceID, sID, metrics)

	// 5. FLOW_SENSE — process conntrack snapshot
	if rawFlows, ok := raw["flow_sense"].([]interface{}); ok && len(rawFlows) > 0 {
		controllerIP := r.Host
		if idx := strings.LastIndex(controllerIP, ":"); idx != -1 {
			controllerIP = controllerIP[:idx]
		}
		go func(devID, siteID string, flows []interface{}, ctrlIP string, rawPayload []byte, schema string) {
			enriched := services.ProcessFlowSenseForSchema(schema, siteID, devID, flows, ctrlIP)
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
			database.Tx(r.Context()).Exec(
				"UPDATE "+schema+".devices SET state_json = $1 WHERE id = $2",
				rawPayload, devID,
			)
		}(deviceID, sID, rawFlows, controllerIP, body, tenantSchema)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}
