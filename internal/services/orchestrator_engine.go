package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"openwrt-controller/internal/database"
)

// ─── SITE_ORCHESTRATOR Engine ────────────────────────────────────────────────
// Renders UCI commands from a site_config template, differentiated by device role.
// Roles: "Gateway" (full L3 + DHCP + firewall), "AP" (wireless + system), "IoT_Node" (system only)

// WANInterface represents a single WAN uplink for mwan3 SD-WAN / multi-WAN failover.
type WANInterface struct {
	Name      string `json:"name"`       // human label, e.g. "Primary WAN"
	IfaceName string `json:"iface_name"` // UCI/Linux interface name, e.g. "wan", "wan2", "lte"
	TrackIP   string `json:"track_ip"`   // IP to ping for link health, e.g. "8.8.8.8"
	Tier      int    `json:"tier"`       // mwan3 member metric (1 = primary, 2+ = backup)
	Weight    int    `json:"weight"`     // mwan3 member weight (1 = default)
}

// SiteConfig represents the desired state of a site.
type SiteConfig struct {
	ID                   string          `json:"id"`
	SiteID               string          `json:"site_id"`
	EnableGlobalSSID     bool            `json:"enable_global_ssid"`
	GlobalSSID           string          `json:"global_ssid"`
	GlobalWPAKey         string          `json:"global_wpa_key"`
	GlobalEncryption     string          `json:"global_encryption"`
	LanIPAddr            string          `json:"lan_ipaddr"`
	LanNetmask           string          `json:"lan_netmask"`
	DHCPStart            int             `json:"dhcp_start"`
	DHCPLimit            int             `json:"dhcp_limit"`
	DHCPLeasetime        string          `json:"dhcp_leasetime"`
	DNSPrimary           string          `json:"dns_primary"`
	DNSSecondary         string          `json:"dns_secondary"`
	Timezone             string          `json:"timezone"`
	HostnamePrefix       string          `json:"hostname_prefix"`
	SQMCakeEnabled       bool            `json:"sqm_cake_enabled"`
	SqmDownload          int             `json:"sqm_download"`
	SqmUpload            int             `json:"sqm_upload"`
	DPIEnabled           bool            `json:"dpi_enabled"`
	SecureTunnelEnabled  bool            `json:"secure_tunnel_enabled"`
	TailscaleEnabled     bool            `json:"tailscale_enabled"`
	TailscaleAuthKey     string          `json:"tailscale_auth_key"`
	FirewallSynFlood     bool            `json:"firewall_syn_flood"`
	FirewallDropInvalid  bool            `json:"firewall_drop_invalid"`
	DropbearPort         int             `json:"dropbear_port"`
	DropbearPasswordAuth bool            `json:"dropbear_password_auth"`
	DHCPReservations     json.RawMessage `json:"dhcp_reservations"`
	PortForwardingRules  json.RawMessage `json:"port_forwarding_rules"`
	ThreatShieldEnabled  bool            `json:"threat_shield_enabled"`
	GuestPortalEnabled   bool            `json:"guest_portal_enabled"`
	AllowPublicSurveys   bool            `json:"allow_public_surveys"`
	// SD-WAN: array of WAN uplinks for mwan3 multi-WAN / failover orchestration.
	// If len >= 2 the Gateway will receive a full mwan3 ruleset.
	WANInterfaces json.RawMessage `json:"wan_interfaces"`
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
			if cfg.EnableGlobalSSID && cfg.GlobalSSID != "" {
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

		// ── SMART QOS (CAKE) (Gateway only) ─────────────────────────

		if role == "Gateway" {

			if cfg.SQMCakeEnabled {

				cmds = append(cmds,

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "enabled", Value: "1"},

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "interface", Value: "eth1"},

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "download", Value: strconv.Itoa(cfg.SqmDownload)},

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "upload", Value: strconv.Itoa(cfg.SqmUpload)},

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "qdisc", Value: "cake"},

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "script", Value: "piece_of_cake.qos"},

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "linklayer", Value: "none"},
				)

			} else {

				cmds = append(cmds,

					UciCommand{Action: "set", Config: "sqm", Section: "@queue[0]", Option: "enabled", Value: "0"},
				)

			}

			// ── DEEP PACKET INSPECTION (nDPI) ──────────────────────────

			if cfg.DPIEnabled {

				cmds = append(cmds,

					UciCommand{Action: "set", Config: "firewall", Section: "dpi_rule", Option: "type", Value: "include"},

					UciCommand{Action: "set", Config: "firewall", Section: "dpi_rule", Option: "path", Value: "/etc/firewall.dpi"},

					UciCommand{Action: "set", Config: "firewall", Section: "dpi_rule", Option: "reload", Value: "1"},
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

			// ── DHCP RESERVATIONS (Gateway only) ─────────────────────────
			if len(cfg.DHCPReservations) > 0 {
				var dhcpList []StaticLease
				if err := json.Unmarshal(cfg.DHCPReservations, &dhcpList); err == nil && len(dhcpList) > 0 {
					for _, dl := range dhcpList {
						cmds = append(cmds,
							UciCommand{Action: "add", Config: "dhcp", Section: "host", Option: "", Value: ""},
							UciCommand{Action: "set", Config: "dhcp", Section: "@host[-1]", Option: "name", Value: dl.Name},
							UciCommand{Action: "set", Config: "dhcp", Section: "@host[-1]", Option: "mac", Value: dl.MAC},
							UciCommand{Action: "set", Config: "dhcp", Section: "@host[-1]", Option: "ip", Value: dl.IP},
						)
					}
				}
			}

			// ── PORT FORWARDING (Gateway only) ───────────────────────────
			if len(cfg.PortForwardingRules) > 0 {
				var pfList []PortForwardRule
				if err := json.Unmarshal(cfg.PortForwardingRules, &pfList); err == nil && len(pfList) > 0 {
					for _, pf := range pfList {
						cmds = append(cmds,
							UciCommand{Action: "add", Config: "firewall", Section: "redirect", Option: "", Value: ""},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "name", Value: pf.Name},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "target", Value: "DNAT"},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "src", Value: "wan"},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "src_dport", Value: fmt.Sprintf("%d", pf.SrcPort)},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "proto", Value: pf.Proto},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "dest_ip", Value: pf.DestIP},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "dest_port", Value: fmt.Sprintf("%d", pf.DestPort)},
							UciCommand{Action: "set", Config: "firewall", Section: "@redirect[-1]", Option: "dest", Value: "lan"},
						)
					}
				}
			}

			// ── SQM CAKE (Gateway only) ──────────────────────────────────
			if cfg.SQMCakeEnabled {
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "enabled", Value: "1"},
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "interface", Value: "eth0"},
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "download", Value: "0"},
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "upload", Value: "0"},
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "qdisc", Value: "cake"},
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "script", Value: "piece_of_cake.qos"},
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "linklayer", Value: "none"},
				)
			} else {
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "sqm", Section: "@sqm[0]", Option: "enabled", Value: "0"},
				)
			}

			// ── DEEP PACKET INSPECTION (nDPI) (Gateway only) ────────────
			if cfg.DPIEnabled {
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "firewall", Section: "@defaults[0]", Option: "dpi_enabled", Value: "1"},
				)
			} else {
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "firewall", Section: "@defaults[0]", Option: "dpi_enabled", Value: "0"},
				)
			}

			// ── GUEST PORTAL (Gateway only) ──────────────────────────────
			if cfg.GuestPortalEnabled {
				cmds = append(cmds,
					UciCommand{Action: "set", Config: "opennds", Section: "@opennds[0]", Option: "enabled", Value: "1"},
					UciCommand{Action: "set", Config: "opennds", Section: "@opennds[0]", Option: "gatewayinterface", Value: "br-lan"},
					// Assuming the controller URL is known, or just fasremoteip
					// The controller could be the WAN IP or a known DNS
					// For now, setting fasremoteip to the controller's IP via env or a placeholder
					UciCommand{Action: "set", Config: "opennds", Section: "@opennds[0]", Option: "fasport", Value: "3000"},
					UciCommand{Action: "set", Config: "opennds", Section: "@opennds[0]", Option: "faspath", Value: "/portal/auth"},
					// Will require manual setup of fasremoteip on OpenNDS
				)
			}

			// ── SD-WAN / mwan3 (Gateway only, ≥2 WANs) ───────────────────
			if len(cfg.WANInterfaces) > 2 { // '[]' is 2 bytes — only act when populated
				var wans []WANInterface
				if err := json.Unmarshal(cfg.WANInterfaces, &wans); err == nil && len(wans) >= 2 {
					cmds = append(cmds, renderMwan3Commands(wans)...)
				}
			}
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

// renderMwan3Commands generates the full mwan3 UCI command set for failover SD-WAN.
// It configures:
//  1. network interfaces (one WAN per uplink, proto dhcp)
//  2. mwan3 interface tracking (ping checks) per WAN
//  3. mwan3 members (iface + metric + weight)
//  4. mwan3 policy "failover" — strictly ordered by Tier
//  5. mwan3 default rule pointing to the policy
func renderMwan3Commands(wans []WANInterface) []UciCommand {
	var cmds []UciCommand

	// Ensure mwan3 globals exist
	cmds = append(cmds,
		UciCommand{Action: "set", Config: "mwan3", Section: "globals", Option: "mmx_mask", Value: "0x3F00"},
	)

	for _, w := range wans {
		iface := w.IfaceName
		if iface == "" {
			continue
		}
		trackIP := w.TrackIP
		if trackIP == "" {
			trackIP = "8.8.8.8"
		}
		weight := w.Weight
		if weight <= 0 {
			weight = 1
		}
		tier := w.Tier
		if tier <= 0 {
			tier = 1
		}

		// Network interface (idempotent — won't break if already exists)
		cmds = append(cmds,
			UciCommand{Action: "set", Config: "network", Section: iface, Option: "proto", Value: "dhcp"},
			UciCommand{Action: "set", Config: "network", Section: iface, Option: "ifname", Value: iface},
		)

		// mwan3 interface section
		memberName := fmt.Sprintf("%s_m%d_%d", iface, tier, weight)
		cmds = append(cmds,
			// mwan3.interface tracking
			UciCommand{Action: "set", Config: "mwan3", Section: iface, Option: "", Value: "interface"},
			UciCommand{Action: "set", Config: "mwan3", Section: iface, Option: "enabled", Value: "1"},
			UciCommand{Action: "set", Config: "mwan3", Section: iface, Option: "count", Value: "1"},
			UciCommand{Action: "set", Config: "mwan3", Section: iface, Option: "timeout", Value: "2"},
			UciCommand{Action: "set", Config: "mwan3", Section: iface, Option: "interval", Value: "5"},
			UciCommand{Action: "set", Config: "mwan3", Section: iface, Option: "reliability", Value: "1"},
			UciCommand{Action: "add_list", Config: "mwan3", Section: iface, Option: "track_ip", Value: trackIP},
			// mwan3 member
			UciCommand{Action: "set", Config: "mwan3", Section: memberName, Option: "", Value: "member"},
			UciCommand{Action: "set", Config: "mwan3", Section: memberName, Option: "interface", Value: iface},
			UciCommand{Action: "set", Config: "mwan3", Section: memberName, Option: "metric", Value: fmt.Sprintf("%d", tier)},
			UciCommand{Action: "set", Config: "mwan3", Section: memberName, Option: "weight", Value: fmt.Sprintf("%d", weight)},
		)
	}

	// Build member list in tier order for the failover policy
	memberList := make([]string, 0, len(wans))
	for _, w := range wans {
		if w.IfaceName == "" {
			continue
		}
		weight := w.Weight
		if weight <= 0 {
			weight = 1
		}
		tier := w.Tier
		if tier <= 0 {
			tier = 1
		}
		memberList = append(memberList, fmt.Sprintf("%s_m%d_%d", w.IfaceName, tier, weight))
	}

	// mwan3 policy: strict failover (members ordered by tier via metric)
	cmds = append(cmds,
		UciCommand{Action: "set", Config: "mwan3", Section: "failover", Option: "", Value: "policy"},
	)
	for _, m := range memberList {
		cmds = append(cmds,
			UciCommand{Action: "add_list", Config: "mwan3", Section: "failover", Option: "use_member", Value: m},
		)
	}

	// mwan3 default rule → failover policy
	cmds = append(cmds,
		UciCommand{Action: "set", Config: "mwan3", Section: "default_rule", Option: "", Value: "rule"},
		UciCommand{Action: "set", Config: "mwan3", Section: "default_rule", Option: "proto", Value: "all"},
		UciCommand{Action: "set", Config: "mwan3", Section: "default_rule", Option: "sticky", Value: "0"},
		UciCommand{Action: "set", Config: "mwan3", Section: "default_rule", Option: "use_policy", Value: "failover"},
	)

	return cmds
}

// ─── Database Operations ─────────────────────────────────────────────────────

func GetSiteConfig(ctx context.Context, siteID string) (*SiteConfig, error) {
	var sc SiteConfig
	err := database.Tx(ctx).QueryRow(`
		SELECT id, site_id, enable_global_ssid, global_ssid, global_wpa_key, global_encryption,
		       lan_ipaddr, COALESCE(sqm_cake_enabled, false), COALESCE(sqm_download, 0), COALESCE(sqm_upload, 0), COALESCE(dpi_enabled, false), COALESCE(secure_tunnel_enabled, true), COALESCE(tailscale_enabled, false), COALESCE(tailscale_auth_key, ''), lan_netmask, dhcp_start, dhcp_limit, dhcp_leasetime,
		       dns_primary, dns_secondary, timezone, hostname_prefix,
		       firewall_syn_flood, firewall_drop_invalid,
		       dropbear_port, dropbear_password_auth,
		       dhcp_reservations, port_forwarding_rules,
		       COALESCE(threat_shield_enabled, false),
		       COALESCE(guest_portal_enabled, false),
		       COALESCE(wan_interfaces, '[]'::jsonb),
		       COALESCE(allow_public_surveys, false)
		FROM site_configs WHERE site_id = $1
	`, siteID).Scan(
		&sc.ID, &sc.SiteID, &sc.EnableGlobalSSID, &sc.GlobalSSID, &sc.GlobalWPAKey, &sc.GlobalEncryption,
		&sc.LanIPAddr, &sc.SQMCakeEnabled, &sc.SqmDownload, &sc.SqmUpload, &sc.DPIEnabled, &sc.SecureTunnelEnabled, &sc.TailscaleEnabled, &sc.TailscaleAuthKey, &sc.LanNetmask, &sc.DHCPStart, &sc.DHCPLimit, &sc.DHCPLeasetime,
		&sc.DNSPrimary, &sc.DNSSecondary, &sc.Timezone, &sc.HostnamePrefix,
		&sc.FirewallSynFlood, &sc.FirewallDropInvalid,
		&sc.DropbearPort, &sc.DropbearPasswordAuth,
		&sc.DHCPReservations, &sc.PortForwardingRules,
		&sc.ThreatShieldEnabled, &sc.GuestPortalEnabled,
		&sc.WANInterfaces, &sc.AllowPublicSurveys,
	)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func UpsertSiteConfig(ctx context.Context, sc SiteConfig) error {
	// Ensure wan_interfaces is a valid JSON array — never NULL
	if len(sc.WANInterfaces) == 0 {
		sc.WANInterfaces = json.RawMessage(`[]`)
	}
	_, err := database.Tx(ctx).Exec(`
		INSERT INTO site_configs (
			site_id, enable_global_ssid, global_ssid, global_wpa_key, global_encryption,
			lan_ipaddr, sqm_cake_enabled, sqm_download, sqm_upload, dpi_enabled, secure_tunnel_enabled, tailscale_enabled, tailscale_auth_key, lan_netmask, dhcp_start, dhcp_limit, dhcp_leasetime,
			dns_primary, dns_secondary, timezone, hostname_prefix,
			firewall_syn_flood, firewall_drop_invalid,
			dropbear_port, dropbear_password_auth,
			dhcp_reservations, port_forwarding_rules, threat_shield_enabled, guest_portal_enabled,
			wan_interfaces, allow_public_surveys, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,CURRENT_TIMESTAMP)
		ON CONFLICT (site_id) DO UPDATE SET
			enable_global_ssid=EXCLUDED.enable_global_ssid, global_ssid=EXCLUDED.global_ssid, global_wpa_key=EXCLUDED.global_wpa_key,
			global_encryption=EXCLUDED.global_encryption,
			lan_ipaddr=EXCLUDED.lan_ipaddr, sqm_cake_enabled=EXCLUDED.sqm_cake_enabled, sqm_download=EXCLUDED.sqm_download, sqm_upload=EXCLUDED.sqm_upload, dpi_enabled=EXCLUDED.dpi_enabled, secure_tunnel_enabled=EXCLUDED.secure_tunnel_enabled, tailscale_enabled, tailscale_auth_key, lan_netmask=EXCLUDED.lan_netmask,
			dhcp_start=EXCLUDED.dhcp_start, dhcp_limit=EXCLUDED.dhcp_limit,
			dhcp_leasetime=EXCLUDED.dhcp_leasetime,
			dns_primary=EXCLUDED.dns_primary, dns_secondary=EXCLUDED.dns_secondary,
			timezone=EXCLUDED.timezone, hostname_prefix=EXCLUDED.hostname_prefix,
			firewall_syn_flood=EXCLUDED.firewall_syn_flood,
			firewall_drop_invalid=EXCLUDED.firewall_drop_invalid,
			dropbear_port=EXCLUDED.dropbear_port,
			dropbear_password_auth=EXCLUDED.dropbear_password_auth,
			dhcp_reservations=EXCLUDED.dhcp_reservations,
			port_forwarding_rules=EXCLUDED.port_forwarding_rules,
			threat_shield_enabled=EXCLUDED.threat_shield_enabled,
			guest_portal_enabled=EXCLUDED.guest_portal_enabled,
			wan_interfaces=EXCLUDED.wan_interfaces,
			allow_public_surveys=EXCLUDED.allow_public_surveys,
			updated_at=CURRENT_TIMESTAMP
	`, sc.SiteID, sc.EnableGlobalSSID, sc.GlobalSSID, sc.GlobalWPAKey, sc.GlobalEncryption,
		sc.LanIPAddr, sc.SQMCakeEnabled, sc.SqmDownload, sc.SqmUpload, sc.DPIEnabled, sc.SecureTunnelEnabled, sc.TailscaleEnabled, sc.TailscaleAuthKey, sc.LanNetmask, sc.DHCPStart, sc.DHCPLimit, sc.DHCPLeasetime,
		sc.DNSPrimary, sc.DNSSecondary, sc.Timezone, sc.HostnamePrefix,
		sc.FirewallSynFlood, sc.FirewallDropInvalid,
		sc.DropbearPort, sc.DropbearPasswordAuth,
		sc.DHCPReservations, sc.PortForwardingRules, sc.ThreatShieldEnabled, sc.GuestPortalEnabled,
		sc.WANInterfaces, sc.AllowPublicSurveys,
	)
	return err
}

func GetSiteDevicesWithRoles(ctx context.Context, siteID string) ([]DeviceRoleInfo, error) {
	rows, err := database.Tx(ctx).Query(`
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
