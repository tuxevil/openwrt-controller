package handlers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func ensureDeviceToken(ctx context.Context, schema, deviceID string, current sql.NullString) (string, error) {
	if current.Valid && current.String != "" {
		return current.String, nil
	}
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := fmt.Sprintf("%x", tokenBytes)
	if _, err := database.Tx(ctx).Exec("UPDATE "+schema+".devices SET device_token = $1 WHERE id = $2", token, deviceID); err != nil {
		return "", err
	}
	return token, nil
}

func resolveDeviceIdentity(ctx context.Context, schema, requestedID string) (string, sql.NullString, error) {
	var storedID string
	var storedToken sql.NullString
	err := database.Tx(ctx).QueryRow(
		"SELECT id, device_token FROM "+schema+".devices WHERE LOWER(id) = LOWER($1)",
		requestedID,
	).Scan(&storedID, &storedToken)
	return storedID, storedToken, err
}

func resolveDeviceIdentityByIP(ctx context.Context, schema, remoteIP string) (string, sql.NullString, error) {
	var storedID string
	var storedToken sql.NullString
	err := database.Tx(ctx).QueryRow(
		"SELECT id, device_token FROM "+schema+".devices WHERE site_id IS NOT NULL AND last_ip = $1 ORDER BY last_seen_at DESC NULLS LAST LIMIT 1",
		remoteIP,
	).Scan(&storedID, &storedToken)
	return storedID, storedToken, err
}

func resolveDeviceIdentityByToken(ctx context.Context, schema, token string) (string, sql.NullString, error) {
	var storedID string
	var storedToken sql.NullString
	err := database.Tx(ctx).QueryRow(
		"SELECT id, device_token FROM "+schema+".devices WHERE device_token = $1",
		token,
	).Scan(&storedID, &storedToken)
	return storedID, storedToken, err
}

func requestRemoteIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// allowLegacyProvision enables the historical behaviour where a device
// could pull config with only an X-Site-Key (no per-device token). It is
// kept behind a feature flag so that greenfield deployments can refuse to
// start, while existing single-tenant installations can opt in via
// ALLOW_LEGACY_PROVISION=true.
func allowLegacyProvision() bool {
	return os.Getenv("ALLOW_LEGACY_PROVISION") == "true"
}

func decodePendingDeviceOperation(raw json.RawMessage) (services.DeviceOperationPlan, error) {
	var plan services.DeviceOperationPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return services.DeviceOperationPlan{}, fmt.Errorf("invalid pending operation: %w", err)
	}
	if err := services.ValidateDeviceOperationPlan(plan); err != nil {
		return services.DeviceOperationPlan{}, fmt.Errorf("invalid pending operation: %w", err)
	}
	return plan, nil
}

func decodePendingDeviceChangeSet(raw json.RawMessage) (services.DeviceChangeSet, error) {
	var changeSet services.DeviceChangeSet
	if err := json.Unmarshal(raw, &changeSet); err != nil {
		return services.DeviceChangeSet{}, fmt.Errorf("invalid pending changeset: %w", err)
	}
	if err := services.ValidateDeviceChangeSet(changeSet); err != nil {
		return services.DeviceChangeSet{}, fmt.Errorf("invalid pending changeset: %w", err)
	}
	return changeSet, nil
}

func validatePendingDeviceChangeSetForDevice(changeSet services.DeviceChangeSet, deviceID string) error {
	if !strings.EqualFold(changeSet.DeviceID, deviceID) {
		return fmt.Errorf("pending changeset targets a different device")
	}
	if changeSet.Generation <= 0 {
		return fmt.Errorf("pending changeset has no reserved generation")
	}
	return nil
}

// deepMerge merges src into dst. dst values have priority.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, sv := range src {
		result[k] = sv
	}
	for k, dv := range dst {
		if sv, ok := src[k]; ok {
			// Both have the key — recurse if both maps
			dstMap, dstIsMap := dv.(map[string]interface{})
			srcMap, srcIsMap := sv.(map[string]interface{})
			if dstIsMap && srcIsMap {
				result[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		result[k] = dv
	}
	return result
}

func GetDeviceConfigHandler(w http.ResponseWriter, r *http.Request) {
	deviceID, err := normalizeEnrollmentDeviceID(r.PathValue("device_id"))
	if err != nil {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	providedKey := r.Header.Get("X-Site-Key")
	providedToken := r.Header.Get("X-Device-Token")
	if providedKey == "" && providedToken == "" {
		http.Error(w, `{"error": "Forbidden: missing device credentials"}`, http.StatusForbidden)
		return
	}
	if providedToken == "" && !allowLegacyProvision() {
		http.Error(w, `{"error": "X-Device-Token header is required"}`, http.StatusUnauthorized)
		return
	}

	tenantSchema, token, err := resolveDeviceTenant(providedToken, providedKey)
	if err != nil {
		http.Error(w, `{"error": "Forbidden: invalid device credentials"}`, http.StatusForbidden)
		return
	}

	// Validate the schema name we are about to interpolate. GetTenantSchemaForSiteKey
	// already filters via tenants.schema_alias, but defence-in-depth.
	if _, sErr := database.SafeSchemaIdent(tenantSchema); sErr != nil {
		log.Printf("[provision] rejected suspicious schema %q for device %s", tenantSchema, deviceID)
		http.Error(w, `{"error": "Forbidden: invalid site key"}`, http.StatusForbidden)
		return
	}

	// --- Hardening: X-Device-Token is mandatory unless legacy mode is enabled ---
	resolvedDeviceID, storedToken, tokenErr := resolveDeviceIdentity(r.Context(), tenantSchema, deviceID)
	if tokenErr == sql.ErrNoRows && token != "" {
		resolvedDeviceID, storedToken, tokenErr = resolveDeviceIdentityByToken(r.Context(), tenantSchema, token)
	}
	if tokenErr == sql.ErrNoRows && token == "" && allowLegacyProvision() {
		resolvedDeviceID, storedToken, tokenErr = resolveDeviceIdentityByIP(r.Context(), tenantSchema, requestRemoteIP(r.RemoteAddr))
	}
	if tokenErr == nil {
		deviceID = resolvedDeviceID
	}
	if tokenErr != nil && tokenErr != sql.ErrNoRows {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	if token == "" {
		if !allowLegacyProvision() {
			http.Error(w, `{"error": "X-Device-Token header is required"}`, http.StatusUnauthorized)
			return
		}
		if tokenErr == sql.ErrNoRows {
			http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
			return
		}
	} else {
		if tokenErr != nil || !storedToken.Valid || storedToken.String == "" || storedToken.String != token {
			http.Error(w, `{"error": "invalid device token"}`, http.StatusUnauthorized)
			return
		}
	}
	if token == "" && (!storedToken.Valid || storedToken.String == "") {
		generated, err := ensureDeviceToken(r.Context(), tenantSchema, deviceID, storedToken)
		if err != nil {
			http.Error(w, `{"error":"could not initialize device token"}`, http.StatusInternalServerError)
			return
		}
		storedToken = sql.NullString{String: generated, Valid: true}
	}

	var siteID sql.NullString
	var siteKey *string
	err = database.Tx(r.Context()).QueryRow(`
		SELECT d.site_id, s.api_key 
		FROM `+tenantSchema+`.devices d 
		LEFT JOIN `+tenantSchema+`.sites s ON d.site_id = s.id 
		WHERE d.id = $1`, deviceID).Scan(&siteID, &siteKey)

	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	if token == "" && siteKey != nil && *siteKey != "" {
		if providedKey != "" && subtle.ConstantTimeCompare([]byte(providedKey), []byte(*siteKey)) != 1 {
			http.Error(w, `{"error": "Forbidden: invalid site key"}`, http.StatusForbidden)
			return
		}
	}

	var pendingOperation interface{}
	var pendingChangeSet interface{}
	operationRaw, operationErr := database.GetPendingDeviceOperation(r.Context(), tenantSchema, deviceID)
	if operationErr != nil {
		log.Printf("[provision] pending operation lookup failed for %s: %v", deviceID, operationErr)
		http.Error(w, `{"error":"could not read pending device operation"}`, http.StatusServiceUnavailable)
		return
	}
	if len(operationRaw) > 0 {
		plan, decodeErr := decodePendingDeviceOperation(operationRaw)
		if decodeErr != nil {
			log.Printf("[provision] refusing invalid pending operation for %s: %v", deviceID, decodeErr)
			http.Error(w, `{"error":"pending device operation is invalid"}`, http.StatusServiceUnavailable)
			return
		}
		pendingOperation = plan
	}
	changeSetRaw, changeSetErr := database.GetPendingDeviceChangeSet(r.Context(), tenantSchema, deviceID)
	if changeSetErr != nil {
		log.Printf("[provision] pending changeset lookup failed for %s: %v", deviceID, changeSetErr)
		http.Error(w, `{"error":"could not read pending device changeset"}`, http.StatusServiceUnavailable)
		return
	}
	if len(changeSetRaw) > 0 {
		changeSet, decodeErr := decodePendingDeviceChangeSet(changeSetRaw)
		if decodeErr != nil {
			log.Printf("[provision] refusing invalid pending changeset for %s: %v", deviceID, decodeErr)
			http.Error(w, `{"error":"pending device changeset is invalid"}`, http.StatusServiceUnavailable)
			return
		}
		if identityErr := validatePendingDeviceChangeSetForDevice(changeSet, deviceID); identityErr != nil {
			log.Printf("[provision] refusing mismatched pending changeset for %s: %v", deviceID, identityErr)
			http.Error(w, `{"error":"pending device changeset identity is invalid"}`, http.StatusServiceUnavailable)
			return
		}
		pendingChangeSet = changeSet
	}
	if pendingOperation != nil && pendingChangeSet != nil {
		log.Printf("[provision] refusing conflicting pending operation and changeset for %s", deviceID)
		http.Error(w, `{"error":"conflicting pending device work"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !siteID.Valid {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"action":  "wait",
			"message": "Pending adoption",
		})
		return
	}

	// Return the per-device token so the agent can persist it after adoption.
	deviceToken := storedToken

	// --- Módulo 2: Actualizar last_config_pulled_at ---
	_, _ = database.Tx(r.Context()).Exec(
		"UPDATE "+tenantSchema+".devices SET last_config_pulled_at = CURRENT_TIMESTAMP WHERE id = $1",
		deviceID,
	)

	var enableGlobalSSID bool = true
	var globalSSID, globalWPAKey, globalEncryption string
	_ = database.Tx(r.Context()).QueryRow(`
		SELECT COALESCE(enable_global_ssid, true), COALESCE(global_ssid, ''),
		       COALESCE(global_wpa_key, ''), COALESCE(global_encryption, '')
		FROM `+tenantSchema+`.site_configs WHERE site_id = $1
	`, siteID.String).Scan(&enableGlobalSSID, &globalSSID, &globalWPAKey, &globalEncryption)

	rows, qErr := database.Tx(r.Context()).Query(`
		SELECT w.ssid, w.security, COALESCE(w.password, ''), COALESCE(w.band, 'both'),
		       COALESCE(w.roaming_enabled, false), COALESCE(w.ieee80211k, false),
		       COALESCE(w.ieee80211v, false), COALESCE(w.ieee80211w, '0'),
		       COALESCE(w.auth_server, ''), COALESCE(w.auth_secret, ''),
		       COALESCE(w.dynamic_vlan, '0'), COALESCE(w.target_mode, 'all'),
		       EXISTS (SELECT 1 FROM `+tenantSchema+`.device_wlans dw
		               WHERE dw.wlan_id = w.id AND dw.device_id = $2)
		FROM `+tenantSchema+`.wlans w
		WHERE w.site_id = $1 AND w.enabled = true
		ORDER BY w.id
	`, siteID.String, deviceID)
	if qErr != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var wlansList []map[string]interface{}
	var policyConflicts []string
	for rows.Next() {
		var ssid, security, password, band, ieee80211w, authServer, authSecret, dynamicVLAN, targetMode string
		var roaming, k, v, assigned bool
		if err := rows.Scan(&ssid, &security, &password, &band, &roaming, &k, &v, &ieee80211w, &authServer, &authSecret, &dynamicVLAN, &targetMode, &assigned); err == nil {
			if !services.WLANAppliesToDevice(enableGlobalSSID, targetMode, assigned) {
				continue
			}
			resolution := services.ResolveCanonicalWLAN(globalSSID, globalEncryption, globalWPAKey, ssid, security, password)
			security = resolution.Security
			password = resolution.Password
			if len(resolution.Conflicts) > 0 {
				policyConflicts = append(policyConflicts, ssid+": "+strings.Join(resolution.Conflicts, ","))
			}
			wlan := map[string]interface{}{
				"ssid":     ssid,
				"security": security,
				"band":     band,
			}
			if password != "" {
				wlan["key"] = password
			}
			if roaming {
				wlan["ieee80211r"] = "1"
			}
			if k {
				wlan["ieee80211k"] = "1"
			}
			if v {
				wlan["ieee80211v"] = "1"
			}
			if ieee80211w != "0" && ieee80211w != "" {
				wlan["ieee80211w"] = ieee80211w
			}
			if authServer != "" {
				wlan["auth_server"] = authServer
			}
			if authSecret != "" {
				wlan["auth_secret"] = authSecret
			}
			if dynamicVLAN != "0" && dynamicVLAN != "" {
				wlan["dynamic_vlan"] = dynamicVLAN
			}
			wlansList = append(wlansList, wlan)
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	if wlansList == nil {
		wlansList = make([]map[string]interface{}, 0)
	}

	sshConfig := make(map[string]interface{})
	if PublicKey != "" {
		sshConfig["authorized_keys"] = []string{strings.TrimSpace(PublicKey)}
	}

	// --- Módulo SECURE_TUNNEL: Wireguard Config ---
	var wgPrivKey, wgPubKey, wgIP, wgEndpoint, siteWgPubKey, deviceRole sql.NullString
	var secureTunnelEnabled bool = true
	_ = database.Tx(r.Context()).QueryRow("SELECT COALESCE(secure_tunnel_enabled, true) FROM "+tenantSchema+".site_configs WHERE site_id = $1", siteID.String).Scan(&secureTunnelEnabled)

	_ = database.Tx(r.Context()).QueryRow(`
		SELECT d.wg_privkey, d.wg_pubkey, d.wg_ip, s.wg_endpoint, s.wg_pubkey, d.device_role 
		FROM `+tenantSchema+`.devices d 
		LEFT JOIN `+tenantSchema+`.sites s ON d.site_id = s.id 
		WHERE d.id = $1`, deviceID).Scan(&wgPrivKey, &wgPubKey, &wgIP, &wgEndpoint, &siteWgPubKey, &deviceRole)

	if !wgPrivKey.Valid || wgPrivKey.String == "" {
		priv, pub, err := services.GenerateWireGuardKeys()
		if err == nil {
			database.Tx(r.Context()).Exec("UPDATE "+tenantSchema+".devices SET wg_privkey = $1, wg_pubkey = $2 WHERE id = $3", priv, pub, deviceID)
			wgPrivKey = sql.NullString{String: priv, Valid: true}
			wgPubKey = sql.NullString{String: pub, Valid: true}
		}
	}

	if !wgIP.Valid || wgIP.String == "" {
		ip, err := services.AssignInternalIP(tenantSchema, deviceID)
		if err == nil {
			wgIP = sql.NullString{String: ip, Valid: true}
		}
	}

	// Make sure the site has a controller wg pubkey
	if !siteWgPubKey.Valid || siteWgPubKey.String == "" {
		// Generate site controller key if missing
		sitePriv, sitePub, err := services.GenerateWireGuardKeys()
		if err == nil {
			database.Tx(r.Context()).Exec("UPDATE "+tenantSchema+".sites SET wg_privkey = $1, wg_pubkey = $2 WHERE id = $3", sitePriv, sitePub, siteID.String)
			siteWgPubKey = sql.NullString{String: sitePub, Valid: true}
		}
	}

	wgConfig := make(map[string]interface{})
	isGateway := deviceRole.Valid && strings.EqualFold(deviceRole.String, "gateway")
	if wgEndpoint.Valid && wgEndpoint.String != "" && isGateway && secureTunnelEnabled {
		wgConfig["enabled"] = true
		wgConfig["private_key"] = wgPrivKey.String
		wgConfig["controller_pubkey"] = siteWgPubKey.String
		wgConfig["endpoint_ip"] = wgEndpoint.String
		wgConfig["internal_ip"] = wgIP.String
		wgConfig["allowed_ips"] = "10.8.0.0/24"
	} else {
		wgConfig["enabled"] = false
	}

	// Fetch threat shield setting for this site
	var threatShieldEnabled bool
	var tailscaleEnabled bool
	var tailscaleAuthKey string

	_ = database.Tx(r.Context()).QueryRow(
		"SELECT COALESCE(threat_shield_enabled, false) FROM "+tenantSchema+".sites WHERE id = $1", siteID.String,
	).Scan(&threatShieldEnabled)

	configPayload := map[string]interface{}{
		"device_token": deviceToken.String,
		"wireless": map[string]interface{}{
			"wlans": wlansList,
		},
		"wireless_policy_conflicts": policyConflicts,
		"ssh":                       sshConfig,
		"wireguard":                 wgConfig,
		"threat_shield":             threatShieldEnabled,
		"tailscale": map[string]interface{}{
			"enabled":  tailscaleEnabled,
			"auth_key": tailscaleAuthKey,
		},
		"survey_mode":                       surveyModeFor(tenantSchema, siteID.String),
		"survey_id":                         surveyIDFor(tenantSchema, siteID.String),
		"survey_telemetry_interval_seconds": surveyIntervalFor(tenantSchema, siteID.String),
	}
	if pendingOperation != nil {
		configPayload["apply_operation"] = pendingOperation
	}
	if pendingChangeSet != nil {
		configPayload["apply_change_set"] = pendingChangeSet
	}

	response := deviceConfigResponse(configPayload, deviceToken.String, allowLegacyProvision())
	json.NewEncoder(w).Encode(response)
}

func deviceConfigResponse(configPayload map[string]interface{}, deviceToken string, legacy bool) map[string]interface{} {
	response := map[string]interface{}{
		"action": "apply",
		"config": configPayload,
	}
	if legacy {
		// Older agents read the token from the response root instead of config.
		response["device_token"] = deviceToken
	}
	return response
}

func resolveDeviceTenant(providedToken, providedKey string) (string, string, error) {
	if providedToken != "" {
		tenantSchema, err := database.GetTenantSchemaForDeviceToken(providedToken)
		if err == nil {
			return tenantSchema, providedToken, nil
		}
		if !allowLegacyProvision() || providedKey == "" {
			return "", providedToken, err
		}
	}

	tenantSchema, err := database.GetTenantSchemaForSiteKey(providedKey)
	if err != nil {
		return "", "", err
	}
	return tenantSchema, "", nil
}

// surveyModeFor returns true if the site has an active wifi_survey.
// Used by the agent to switch its telemetry loop to the high-cadence branch.
func surveyModeFor(schema, siteID string) bool {
	if siteID == "" {
		return false
	}
	s, err := getActiveSurveyForSite(schema, siteID)
	return err == nil && s != nil
}

// surveyIDFor returns the active survey ID for the site, or "" if none.
func surveyIDFor(schema, siteID string) string {
	if siteID == "" {
		return ""
	}
	s, err := getActiveSurveyForSite(schema, siteID)
	if err != nil || s == nil {
		return ""
	}
	return s.ID
}

// surveyIntervalFor is the seconds the agent should sleep between telemetry
// cycles when in survey mode. 2s gives ~30 samples per minute per AP.
func surveyIntervalFor(schema, siteID string) int {
	if surveyModeFor(schema, siteID) {
		return 2
	}
	return 0
}

func getActiveSurveyForSite(schema, siteID string) (*database.Survey, error) {
	return database.GetActiveSurveyForSite(context.Background(), schema, siteID)
}
