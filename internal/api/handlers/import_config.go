package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"log"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"

	"golang.org/x/crypto/curve25519"
)

// derivePublicKey computes the Curve25519 public key from a base64-encoded private key.
func derivePublicKey(privBase64 string) (string, error) {
	privBytes, err := base64.StdEncoding.DecodeString(privBase64)
	if err != nil {
		return "", err
	}
	if len(privBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: %d", len(privBytes))
	}
	var privKey [32]byte
	copy(privKey[:], privBytes)

	var pubKey [32]byte
	curve25519.ScalarBaseMult(&pubKey, &privKey)
	return base64.StdEncoding.EncodeToString(pubKey[:]), nil
}

// ImportDeviceConfigHandler connects to the device via SSH, pulls all major UCI configurations
// (wireless, network, dhcp, firewall, system, dropbear), parses them, and populates both the
// site's SiteConfig template, active WLANs, and any active WireGuard/VPN configurations.
func ImportDeviceConfigHandler(w http.ResponseWriter, r *http.Request) {
	schema := getTenantSchema(r)
	username := GetUsernameFromReq(r)
	deviceID := r.PathValue("device_id")

	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	var siteID sql.NullString
	var lastIP sql.NullString
	var status sql.NullString

	err := database.Tx(r.Context()).QueryRow("SELECT site_id, last_ip, status FROM "+schema+".devices WHERE id = $1", deviceID).Scan(&siteID, &lastIP, &status)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error": "device not found"}`, http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	if !siteID.Valid || siteID.String == "" {
		http.Error(w, `{"error": "device is not adopted to any site"}`, http.StatusBadRequest)
		return
	}

	if !lastIP.Valid || lastIP.String == "" {
		http.Error(w, `{"error": "device IP not found"}`, http.StatusBadRequest)
		return
	}

	// Connect to device and run unified SSH query
	cmd := `uci show wireless; echo "===SECTION_BREAK==="; uci show network; echo "===SECTION_BREAK==="; uci show dhcp; echo "===SECTION_BREAK==="; uci show firewall; echo "===SECTION_BREAK==="; uci show system; echo "===SECTION_BREAK==="; uci show dropbear`
	out, err := runSSHCommand(deviceID, cmd)
	log.Printf("[IMPORT_DEBUG] runSSHCommand output: %q, error: %v", out, err)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  fmt.Sprintf("Failed to fetch device config via SSH: %s", err.Error()),
			"output": out,
		})
		return
	}

	parts := strings.Split(out, "===SECTION_BREAK===")
	var rawWireless, rawNetwork, rawDhcp, rawFirewall, rawSystem, rawDropbear string
	if len(parts) > 0 {
		rawWireless = parts[0]
	}
	if len(parts) > 1 {
		rawNetwork = parts[1]
	}
	if len(parts) > 2 {
		rawDhcp = parts[2]
	}
	if len(parts) > 3 {
		rawFirewall = parts[3]
	}
	if len(parts) > 4 {
		rawSystem = parts[4]
	}
	if len(parts) > 5 {
		rawDropbear = parts[5]
	}

	log.Printf("[IMPORT_DEBUG] rawWireless: %q, rawNetwork: %q", rawWireless, rawNetwork)
	wirelessSecs := parseUciShow(rawWireless, "wireless")
	log.Printf("[IMPORT_DEBUG] wirelessSecs parsed: %d", len(wirelessSecs))
	networkSecs := parseUciShow(rawNetwork, "network")
	dhcpSecs := parseUciShow(rawDhcp, "dhcp")
	firewallSecs := parseUciShow(rawFirewall, "firewall")
	systemSecs := parseUciShow(rawSystem, "system")
	dropbearSecs := parseUciShow(rawDropbear, "dropbear")

	// 1. Parse Wireless (supporting multiple SSIDs)
	type ImportedWLAN struct {
		SSID          string `json:"ssid"`
		Encryption    string `json:"encryption"`
		WpaKeyPresent bool   `json:"wpa_key_present"`
		WpaKey        string `json:"-"`
	}
	var importedWLANs []ImportedWLAN

	for _, sec := range wirelessSecs {
		if sec.Type == "wifi-iface" {
			ssidVal, hasSSID := sec.Options["ssid"]
			if hasSSID {
				if s, ok := ssidVal.(string); ok && s != "" {
					key := ""
					if keyVal, hasKey := sec.Options["key"]; hasKey {
						if k, ok := keyVal.(string); ok {
							key = k
						}
					}
					enc := "psk2"
					if encVal, hasEnc := sec.Options["encryption"]; hasEnc {
						if e, ok := encVal.(string); ok {
							enc = e
						}
					}
					importedWLANs = append(importedWLANs, ImportedWLAN{
						SSID:          s,
						Encryption:    enc,
						WpaKeyPresent: key != "",
						WpaKey:        key,
					})
				}
			}
		}
	}

	var globalSSID, globalWPAKey, globalEncryption string
	globalEncryption = "psk2"
	if len(importedWLANs) > 0 {
		globalSSID = importedWLANs[0].SSID
		globalWPAKey = importedWLANs[0].WpaKey
		globalEncryption = importedWLANs[0].Encryption
	}

	// 2. Parse Network & VPN (supporting multiple VPNs)
	lanIPAddr := lastIP.String
	lanNetmask := "255.255.255.0"

	type ImportedVPN struct {
		Interface  string `json:"interface"`
		IP         string `json:"ip"`
		PubKey     string `json:"pubkey"`
		PrivKey    string `json:"-"`
		Endpoint   string `json:"endpoint"`
		PeerPubKey string `json:"peer_pubkey"`
	}
	var importedVPNs []ImportedVPN

	for _, sec := range networkSecs {
		if sec.ID == "lan" && sec.Type == "interface" {
			if ip, ok := sec.Options["ipaddr"].(string); ok && ip != "" {
				lanIPAddr = ip
			}
			if mask, ok := sec.Options["netmask"].(string); ok && mask != "" {
				lanNetmask = mask
			}
		}
		if sec.Type == "interface" {
			if proto, ok := sec.Options["proto"].(string); ok && proto == "wireguard" {
				ifaceName := sec.ID
				privKey := ""
				if pk, ok := sec.Options["private_key"].(string); ok {
					privKey = pk
				}
				pubKey := ""
				if privKey != "" {
					if derived, err := derivePublicKey(privKey); err == nil {
						pubKey = derived
					}
				}
				ip := ""
				if ipaddr, ok := sec.Options["ipaddr"].(string); ok && ipaddr != "" {
					ip = ipaddr
				} else if ips, ok := sec.Options["ipaddr"].([]string); ok && len(ips) > 0 {
					ip = ips[0]
				} else if addr, ok := sec.Options["addresses"].(string); ok && addr != "" {
					ip = addr
				} else if addrs, ok := sec.Options["addresses"].([]string); ok && len(addrs) > 0 {
					ip = addrs[0]
				}
				if idx := strings.Index(ip, "/"); idx != -1 {
					ip = ip[:idx]
				}

				// Find associated peer section
				peerPubKey := ""
				endpoint := ""
				peerSectionType := "wireguard_" + ifaceName

				for _, psec := range networkSecs {
					isPeer := psec.Type == peerSectionType || 
						(psec.Type == "wireguard_peer" && psec.Options["config"] == ifaceName)
					if isPeer {
						if pub, ok := psec.Options["public_key"].(string); ok {
							peerPubKey = pub
						}
						host := ""
						if h, ok := psec.Options["endpoint_host"].(string); ok {
							host = h
						}
						port := ""
						if p, ok := psec.Options["endpoint_port"].(string); ok {
							port = p
						}
						if host != "" && port != "" {
							endpoint = host + ":" + port
						}
						break
					}
				}

				importedVPNs = append(importedVPNs, ImportedVPN{
					Interface:  ifaceName,
					IP:         ip,
					PubKey:     pubKey,
					PrivKey:    privKey,
					Endpoint:   endpoint,
					PeerPubKey: peerPubKey,
				})
			}
		}
	}

	// 3. Parse DHCP
	dhcpStart := 100
	dhcpLimit := 150
	dhcpLeasetime := "12h"
	var dhcpReservations []services.StaticLease

	for _, sec := range dhcpSecs {
		if sec.ID == "lan" && sec.Type == "dhcp" {
			if sVal, ok := sec.Options["start"].(string); ok {
				var s int
				if _, err := fmt.Sscanf(sVal, "%d", &s); err == nil {
					dhcpStart = s
				}
			}
			if lVal, ok := sec.Options["limit"].(string); ok {
				var l int
				if _, err := fmt.Sscanf(lVal, "%d", &l); err == nil {
					dhcpLimit = l
				}
			}
			if lt, ok := sec.Options["leasetime"].(string); ok && lt != "" {
				dhcpLeasetime = lt
			}
		}
		if sec.Type == "host" {
			var name, mac, ip string
			if n, ok := sec.Options["name"].(string); ok {
				name = n
			}
			if m, ok := sec.Options["mac"].(string); ok {
				mac = m
			}
			if macs, ok := sec.Options["mac"].([]string); ok && len(macs) > 0 {
				mac = macs[0]
			}
			if ipaddr, ok := sec.Options["ip"].(string); ok {
				ip = ipaddr
			}
			if mac != "" && ip != "" {
				if name == "" {
					name = "Imported-" + ip
				}
				dhcpReservations = append(dhcpReservations, services.StaticLease{
					Name: name,
					MAC:  strings.ToUpper(mac),
					IP:   ip,
				})
			}
		}
	}

	// 4. Parse Firewall
	firewallSynFlood := true
	firewallDropInvalid := true
	var portForwardingRules []services.PortForwardRule

	for _, sec := range firewallSecs {
		if sec.Type == "defaults" {
			if sf, ok := sec.Options["syn_flood"].(string); ok {
				firewallSynFlood = (sf == "1" || sf == "true")
			}
			if di, ok := sec.Options["drop_invalid"].(string); ok {
				firewallDropInvalid = (di == "1" || di == "true")
			}
		}
		if sec.Type == "redirect" {
			src, _ := sec.Options["src"].(string)
			dest, _ := sec.Options["dest"].(string)
			target, _ := sec.Options["target"].(string)
			if src == "wan" && (dest == "lan" || dest == "") && (target == "DNAT" || target == "") {
				var name, proto, destIP string
				var srcPort, destPort int
				enabled := true

				if n, ok := sec.Options["name"].(string); ok {
					name = n
				}
				if pr, ok := sec.Options["proto"].(string); ok {
					proto = pr
				}
				if dip, ok := sec.Options["dest_ip"].(string); ok {
					destIP = dip
				}
				if spVal, ok := sec.Options["src_dport"].(string); ok {
					fmt.Sscanf(spVal, "%d", &srcPort)
				}
				if dpVal, ok := sec.Options["dest_port"].(string); ok {
					fmt.Sscanf(dpVal, "%d", &destPort)
				}
				if enVal, ok := sec.Options["enabled"].(string); ok {
					enabled = (enVal == "1" || enVal == "true")
				}

				if destIP != "" {
					if name == "" {
						name = fmt.Sprintf("Imported-%d", srcPort)
					}
					if proto == "" {
						proto = "tcp"
					}
					portForwardingRules = append(portForwardingRules, services.PortForwardRule{
						Name:     name,
						Proto:    proto,
						SrcPort:  srcPort,
						DestIP:   destIP,
						DestPort: destPort,
						Enabled:  enabled,
					})
				}
			}
		}
	}

	// 5. Parse System
	timezone := "UTC"
	hostnamePrefix := "nerve"

	for _, sec := range systemSecs {
		if sec.Type == "system" {
			if tz, ok := sec.Options["timezone"].(string); ok && tz != "" {
				timezone = tz
			}
			if hn, ok := sec.Options["hostname"].(string); ok && hn != "" {
				if idx := strings.Index(hn, "-"); idx != -1 {
					hostnamePrefix = hn[:idx]
				} else {
					hostnamePrefix = hn
				}
			}
		}
	}

	// 6. Parse Dropbear
	dropbearPort := 22
	dropbearPasswordAuth := true

	for _, sec := range dropbearSecs {
		if sec.Type == "dropbear" {
			if pVal, ok := sec.Options["Port"].(string); ok {
				fmt.Sscanf(pVal, "%d", &dropbearPort)
			}
			if pa, ok := sec.Options["PasswordAuth"].(string); ok {
				dropbearPasswordAuth = (pa == "1" || pa == "true")
			}
		}
	}

	// Marshal arrays to JSON
	dhcpResJSON, err := json.Marshal(dhcpReservations)
	if err != nil {
		dhcpResJSON = []byte("[]")
	}

	pfRulesJSON, err := json.Marshal(portForwardingRules)
	if err != nil {
		pfRulesJSON = []byte("[]")
	}

	// --- Database updates (transacted for consistency) ---
	tx, err := database.DB.Begin()
	if err != nil {
		http.Error(w, `{"error": "Failed to start database transaction"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Upsert site configs
	_, err = tx.Exec(`
		INSERT INTO `+schema+`.site_configs (
			site_id, enable_global_ssid, global_ssid, global_wpa_key, global_encryption,
			lan_ipaddr, lan_netmask, dhcp_start, dhcp_limit, dhcp_leasetime,
			dns_primary, dns_secondary, timezone, hostname_prefix,
			firewall_syn_flood, firewall_drop_invalid,
			dropbear_port, dropbear_password_auth,
			dhcp_reservations, port_forwarding_rules, updated_at
		) VALUES ($1, true, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, CURRENT_TIMESTAMP)
		ON CONFLICT (site_id) DO UPDATE SET
			global_ssid = EXCLUDED.global_ssid,
			global_wpa_key = EXCLUDED.global_wpa_key,
			global_encryption = EXCLUDED.global_encryption,
			lan_ipaddr = EXCLUDED.lan_ipaddr,
			lan_netmask = EXCLUDED.lan_netmask,
			dhcp_start = EXCLUDED.dhcp_start,
			dhcp_limit = EXCLUDED.dhcp_limit,
			dhcp_leasetime = EXCLUDED.dhcp_leasetime,
			dns_primary = EXCLUDED.dns_primary,
			dns_secondary = EXCLUDED.dns_secondary,
			timezone = EXCLUDED.timezone,
			hostname_prefix = EXCLUDED.hostname_prefix,
			firewall_syn_flood = EXCLUDED.firewall_syn_flood,
			firewall_drop_invalid = EXCLUDED.firewall_drop_invalid,
			dropbear_port = EXCLUDED.dropbear_port,
			dropbear_password_auth = EXCLUDED.dropbear_password_auth,
			dhcp_reservations = EXCLUDED.dhcp_reservations,
			port_forwarding_rules = EXCLUDED.port_forwarding_rules,
			updated_at = CURRENT_TIMESTAMP
	`, siteID.String, globalSSID, globalWPAKey, globalEncryption,
		lanIPAddr, lanNetmask, dhcpStart, dhcpLimit, dhcpLeasetime,
		"9.9.9.9", "1.1.1.1", timezone, hostnamePrefix,
		firewallSynFlood, firewallDropInvalid,
		dropbearPort, dropbearPasswordAuth,
		dhcpResJSON, pfRulesJSON)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to update site config: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// 2. Insert ALL WLAN configs that do not already exist
	for _, wlan := range importedWLANs {
		var wlanExists bool
		err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM "+schema+".wlans WHERE site_id = $1 AND ssid = $2)", siteID.String, wlan.SSID).Scan(&wlanExists)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to check existing WLANs: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if !wlanExists {
			_, err = tx.Exec(`
				INSERT INTO `+schema+`.wlans (site_id, ssid, security, password, enabled, roaming_enabled)
				VALUES ($1, true, $2, $3, $4, true, false)
			`, siteID.String, wlan.SSID, wlan.Encryption, wlan.WpaKey)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "Failed to insert WLAN: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
		}
	}

	// 3. Update device and site WireGuard config using the first imported VPN if any
	if len(importedVPNs) > 0 {
		mainVpn := importedVPNs[0]
		if mainVpn.PrivKey != "" {
			_, err = tx.Exec(`
				UPDATE `+schema+`.devices
				SET wg_privkey = $1, wg_pubkey = $2, wg_ip = $3
				WHERE id = $4
			`, mainVpn.PrivKey, mainVpn.PubKey, mainVpn.IP, deviceID)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "Failed to update device VPN: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
		}
		if mainVpn.PeerPubKey != "" {
			_, err = tx.Exec(`
				UPDATE `+schema+`.sites
				SET wg_endpoint = $1, wg_pubkey = $2
				WHERE id = $3
			`, mainVpn.Endpoint, mainVpn.PeerPubKey, siteID.String)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "Failed to update site VPN: %s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error": "Failed to commit database updates"}`, http.StatusInternalServerError)
		return
	}

	// Register Audit Log
	database.InsertAuditLog(username, "DEVICE_CONFIG_IMPORTED", "DEVICE", deviceID,
		fmt.Sprintf("Imported %d WLANs and %d VPNs from device %s into site %s", len(importedWLANs), len(importedVPNs), deviceID, siteID.String), r.RemoteAddr)

	// Prepare complete report
	report := map[string]interface{}{
		"wlans":                importedWLANs,
		"vpns":                 importedVPNs,
		"lan_ip":               lanIPAddr,
		"lan_netmask":          lanNetmask,
		"dhcp_start":           dhcpStart,
		"dhcp_limit":           dhcpLimit,
		"dhcp_leasetime":       dhcpLeasetime,
		"dhcp_leases_count":    len(dhcpReservations),
		"port_forwards_count":  len(portForwardingRules),
		"timezone":             timezone,
		"hostname_prefix":      hostnamePrefix,
		"dropbear_port":        dropbearPort,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Configuration and VPN settings imported successfully into site template",
		"report":  report,
	})
}
