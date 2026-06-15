package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

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
	deviceID := strings.ToLower(r.PathValue("device_id"))
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	providedKey := r.Header.Get("X-Site-Key")
	if providedKey == "" {
		http.Error(w, `{"error": "Forbidden: missing site key"}`, http.StatusForbidden)
		return
	}

	tenantSchema, err := database.GetTenantSchemaForSiteKey(providedKey)
	if err != nil {
		http.Error(w, `{"error": "Forbidden: invalid site key"}`, http.StatusForbidden)
		return
	}

	// --- Módulo 3: Hardening - Validar X-Device-Token ---
	token := r.Header.Get("X-Device-Token")
	if token != "" {
		// Solo valida si se envía un token; sin token = acceso sin auth (modo legado)
		var storedToken sql.NullString
		err := database.Tx(r.Context()).QueryRow("SELECT device_token FROM "+tenantSchema+".devices WHERE id = $1", deviceID).Scan(&storedToken)
		if err == nil && storedToken.Valid && storedToken.String != "" && storedToken.String != token {
			http.Error(w, `{"error": "invalid device token"}`, http.StatusUnauthorized)
			return
		}
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

	if siteKey != nil && *siteKey != "" {
		if providedKey != *siteKey {
			http.Error(w, `{"error": "Forbidden: invalid site key"}`, http.StatusForbidden)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if !siteID.Valid {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"action":  "wait",
			"message": "Pending adoption",
		})
		return
	}

	// --- Módulo 2: Actualizar last_config_pulled_at ---
	_, _ = database.Tx(r.Context()).Exec(
		"UPDATE "+tenantSchema+".devices SET last_config_pulled_at = CURRENT_TIMESTAMP WHERE id = $1",
		deviceID,
	)

	rows, err := database.Tx(r.Context()).Query(
		"SELECT ssid, security, password, band, COALESCE(roaming_enabled, false), COALESCE(ieee80211k, false), COALESCE(ieee80211v, false) FROM "+tenantSchema+".wlans WHERE site_id = $1 AND enabled = true",
		siteID.String,
	)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var wlansList []map[string]interface{}
	for rows.Next() {
		var ssid, security, password, band, ieee80211w, auth_server, auth_secret, dynamic_vlan string
		var roaming, k, v bool
		if err := rows.Scan(&ssid, &security, &password, &band, &roaming, &k, &v, &ieee80211w, &auth_server, &auth_secret, &dynamic_vlan); err == nil {
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
			wlansList = append(wlansList, wlan)
		}
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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"action": "apply",
		"config": map[string]interface{}{
			"wireless": map[string]interface{}{
				"wlans": wlansList,
			},
			"ssh":           sshConfig,
			"wireguard":     wgConfig,
			"threat_shield": threatShieldEnabled,
			"tailscale": map[string]interface{}{
				"enabled":  tailscaleEnabled,
				"auth_key": tailscaleAuthKey,
			},
		},
	})
}
