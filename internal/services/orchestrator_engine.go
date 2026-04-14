package services

import (
	"fmt"

	"openwrt-controller/internal/database"
)

// ─── SITE_ORCHESTRATOR Engine ────────────────────────────────────────────────
// Renders UCI commands from a site_config template, differentiated by device role.
// Roles: "Gateway" (full L3 + DHCP + firewall), "AP" (wireless + system), "IoT_Node" (system only)

// SiteConfig represents the desired state of a site.
type SiteConfig struct {
	ID                  string `json:"id"`
	SiteID              string `json:"site_id"`
	GlobalSSID          string `json:"global_ssid"`
	GlobalWPAKey        string `json:"global_wpa_key"`
	GlobalEncryption    string `json:"global_encryption"`
	LanIPAddr           string `json:"lan_ipaddr"`
	LanNetmask          string `json:"lan_netmask"`
	DHCPStart           int    `json:"dhcp_start"`
	DHCPLimit           int    `json:"dhcp_limit"`
	DHCPLeasetime       string `json:"dhcp_leasetime"`
	DNSPrimary          string `json:"dns_primary"`
	DNSSecondary        string `json:"dns_secondary"`
	Timezone            string `json:"timezone"`
	HostnamePrefix      string `json:"hostname_prefix"`
	FirewallSynFlood    bool   `json:"firewall_syn_flood"`
	FirewallDropInvalid bool   `json:"firewall_drop_invalid"`
	DropbearPort        int    `json:"dropbear_port"`
	DropbearPasswordAuth bool  `json:"dropbear_password_auth"`
}

// DeviceRoleInfo holds the device identity and role for rendering.
type DeviceRoleInfo struct {
	DeviceID string `json:"device_id"`
	Hostname string `json:"hostname"`
	LastIP   string `json:"last_ip"`
	Role     string `json:"device_role"`
}

// RenderResult is the output of the rendering engine — UCI commands per device.
type RenderResult struct {
	DeviceID string       `json:"device_id"`
	Hostname string       `json:"hostname"`
	Role     string       `json:"role"`
	Commands []UciCommand `json:"commands"`
}

// RenderSiteConfig takes a SiteConfig and a list of devices with roles,
// and produces per-device UCI command sets based on each device's role.
//
// Role-based rendering logic:
//   - ALL roles:     wireless (SSID/key), system (timezone, hostname)
//   - Gateway only:  network (LAN IP), dhcp (range, leasetime, DNS), firewall (syn_flood, drop_invalid)
//   - AP:            wireless, system, dropbear
//   - IoT_Node:      system, dropbear
func RenderSiteConfig(cfg SiteConfig, devices []DeviceRoleInfo) []RenderResult {
	var results []RenderResult

	for i, dev := range devices {
		var cmds []UciCommand

		role := dev.Role
		if role == "" {
			role = "AP" // default
		}

		// ── SYSTEM (ALL roles) ───────────────────────────────────────
		hostname := fmt.Sprintf("%s-%s-%d", cfg.HostnamePrefix, role, i+1)
		if dev.Hostname != "" && dev.Hostname != "UNKNOWN" {
			hostname = dev.Hostname // keep existing hostname if already set
		}
		cmds = append(cmds,
			UciCommand{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: hostname},
			UciCommand{Action: "set", Config: "system", Section: "@system[0]", Option: "timezone", Value: cfg.Timezone},
		)

		// ── WIRELESS (Gateway + AP) ──────────────────────────────────
		if role == "Gateway" || role == "AP" {
			if cfg.GlobalSSID != "" {
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "wireless", Section: "default_radio0", Option: "ssid", Value: cfg.GlobalSSID},
					UciCommand{Action: "set", Config: "wireless", Section: "default_radio0", Option: "encryption", Value: cfg.GlobalEncryption},
				)
				if cfg.GlobalWPAKey != "" {
					cmds = append(cmds,
						UciCommand{Action: "set", Config: "wireless", Section: "default_radio0", Option: "key", Value: cfg.GlobalWPAKey},
					)
				}
				// Enable radio
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "wireless", Section: "radio0", Option: "disabled", Value: "0"},
				)
			}
		}

		// ── NETWORK (Gateway only) ───────────────────────────────────
		if role == "Gateway" {
			cmds = append(cmds,
				UciCommand{Action: "set", Config: "network", Section: "lan", Option: "ipaddr", Value: cfg.LanIPAddr},
				UciCommand{Action: "set", Config: "network", Section: "lan", Option: "netmask", Value: cfg.LanNetmask},
			)
		}

		// ── DHCP (Gateway only) ──────────────────────────────────────
		if role == "Gateway" {
			cmds = append(cmds,
				UciCommand{Action: "set", Config: "dhcp", Section: "lan", Option: "start", Value: fmt.Sprintf("%d", cfg.DHCPStart)},
				UciCommand{Action: "set", Config: "dhcp", Section: "lan", Option: "limit", Value: fmt.Sprintf("%d", cfg.DHCPLimit)},
				UciCommand{Action: "set", Config: "dhcp", Section: "lan", Option: "leasetime", Value: cfg.DHCPLeasetime},
			)
			// DNS upstream
			cmds = append(cmds,
				UciCommand{Action: "delete", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: ""},
				UciCommand{Action: "add_list", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: cfg.DNSPrimary},
				UciCommand{Action: "add_list", Config: "dhcp", Section: "@dnsmasq[0]", Option: "server", Value: cfg.DNSSecondary},
			)
		}

		// ── FIREWALL (Gateway only) ──────────────────────────────────
		if role == "Gateway" {
			synFlood := "0"
			if cfg.FirewallSynFlood {
				synFlood = "1"
			}
			dropInvalid := "0"
			if cfg.FirewallDropInvalid {
				dropInvalid = "1"
			}
			cmds = append(cmds,
				UciCommand{Action: "set", Config: "firewall", Section: "@defaults[0]", Option: "syn_flood", Value: synFlood},
				UciCommand{Action: "set", Config: "firewall", Section: "@defaults[0]", Option: "drop_invalid", Value: dropInvalid},
			)
		}

		// ── DROPBEAR (ALL roles) ─────────────────────────────────────
		cmds = append(cmds,
			UciCommand{Action: "set", Config: "dropbear", Section: "@dropbear[0]", Option: "Port", Value: fmt.Sprintf("%d", cfg.DropbearPort)},
		)
		pwAuth := "off"
		if cfg.DropbearPasswordAuth {
			pwAuth = "on"
		}
		cmds = append(cmds,
			UciCommand{Action: "set", Config: "dropbear", Section: "@dropbear[0]", Option: "PasswordAuth", Value: pwAuth},
		)

		results = append(results, RenderResult{
			DeviceID: dev.DeviceID,
			Hostname: hostname,
			Role:     role,
			Commands: cmds,
		})
	}

	return results
}

// ─── Database Operations ─────────────────────────────────────────────────────

func GetSiteConfig(siteID string) (*SiteConfig, error) {
	var sc SiteConfig
	err := database.DB.QueryRow(`
		SELECT id, site_id, global_ssid, global_wpa_key, global_encryption,
		       lan_ipaddr, lan_netmask, dhcp_start, dhcp_limit, dhcp_leasetime,
		       dns_primary, dns_secondary, timezone, hostname_prefix,
		       firewall_syn_flood, firewall_drop_invalid,
		       dropbear_port, dropbear_password_auth
		FROM site_configs WHERE site_id = $1
	`, siteID).Scan(
		&sc.ID, &sc.SiteID, &sc.GlobalSSID, &sc.GlobalWPAKey, &sc.GlobalEncryption,
		&sc.LanIPAddr, &sc.LanNetmask, &sc.DHCPStart, &sc.DHCPLimit, &sc.DHCPLeasetime,
		&sc.DNSPrimary, &sc.DNSSecondary, &sc.Timezone, &sc.HostnamePrefix,
		&sc.FirewallSynFlood, &sc.FirewallDropInvalid,
		&sc.DropbearPort, &sc.DropbearPasswordAuth,
	)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func UpsertSiteConfig(sc SiteConfig) error {
	_, err := database.DB.Exec(`
		INSERT INTO site_configs (
			site_id, global_ssid, global_wpa_key, global_encryption,
			lan_ipaddr, lan_netmask, dhcp_start, dhcp_limit, dhcp_leasetime,
			dns_primary, dns_secondary, timezone, hostname_prefix,
			firewall_syn_flood, firewall_drop_invalid,
			dropbear_port, dropbear_password_auth, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,CURRENT_TIMESTAMP)
		ON CONFLICT (site_id) DO UPDATE SET
			global_ssid=EXCLUDED.global_ssid, global_wpa_key=EXCLUDED.global_wpa_key,
			global_encryption=EXCLUDED.global_encryption,
			lan_ipaddr=EXCLUDED.lan_ipaddr, lan_netmask=EXCLUDED.lan_netmask,
			dhcp_start=EXCLUDED.dhcp_start, dhcp_limit=EXCLUDED.dhcp_limit,
			dhcp_leasetime=EXCLUDED.dhcp_leasetime,
			dns_primary=EXCLUDED.dns_primary, dns_secondary=EXCLUDED.dns_secondary,
			timezone=EXCLUDED.timezone, hostname_prefix=EXCLUDED.hostname_prefix,
			firewall_syn_flood=EXCLUDED.firewall_syn_flood,
			firewall_drop_invalid=EXCLUDED.firewall_drop_invalid,
			dropbear_port=EXCLUDED.dropbear_port,
			dropbear_password_auth=EXCLUDED.dropbear_password_auth,
			updated_at=CURRENT_TIMESTAMP
	`, sc.SiteID, sc.GlobalSSID, sc.GlobalWPAKey, sc.GlobalEncryption,
		sc.LanIPAddr, sc.LanNetmask, sc.DHCPStart, sc.DHCPLimit, sc.DHCPLeasetime,
		sc.DNSPrimary, sc.DNSSecondary, sc.Timezone, sc.HostnamePrefix,
		sc.FirewallSynFlood, sc.FirewallDropInvalid,
		sc.DropbearPort, sc.DropbearPasswordAuth,
	)
	return err
}

func GetSiteDevicesWithRoles(siteID string) ([]DeviceRoleInfo, error) {
	rows, err := database.DB.Query(`
		SELECT id, COALESCE(state_json->'board'->>'hostname','UNKNOWN'),
		       COALESCE(last_ip,''), COALESCE(device_role,'AP')
		FROM devices WHERE site_id = $1 AND status != 'OFFLINE'
		ORDER BY device_role, id
	`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devs []DeviceRoleInfo
	for rows.Next() {
		var d DeviceRoleInfo
		if err := rows.Scan(&d.DeviceID, &d.Hostname, &d.LastIP, &d.Role); err == nil {
			devs = append(devs, d)
		}
	}
	return devs, nil
}

func UpdateDeviceRole(deviceID, role string) error {
	_, err := database.DB.Exec(`UPDATE devices SET device_role = $1 WHERE id = $2`, role, deviceID)
	return err
}
