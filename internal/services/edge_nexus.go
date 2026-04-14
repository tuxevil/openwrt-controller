package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─── EDGE_NEXUS Models ────────────────────────────────────────────────────────

// NetworkInterface represents a single UCI network interface entry.
type NetworkInterface struct {
	Name    string `json:"name"`     // e.g. "lan", "wan", "vlan10"
	VlanID  int    `json:"vlan_id"`  // 0 = not a VLAN
	Proto   string `json:"proto"`    // "static" | "dhcp" | "dhcpv6" | "wireguard"
	IPAddr  string `json:"ip_addr"`  // for static
	Netmask string `json:"netmask"`  // for static
	Gateway string `json:"gateway"`  // for static WAN
	Device  string `json:"device"`   // underlying physical device / bridge
	Enabled bool   `json:"enabled"`
}

// StaticLease maps a MAC address to a fixed IP for dnsmasq.
type StaticLease struct {
	Name string `json:"name"` // friendly label
	MAC  string `json:"mac"`
	IP   string `json:"ip"`
}

// DHCPInterface is the DHCP/DNS configuration for one interface.
type DHCPInterface struct {
	Interface  string        `json:"interface"`   // UCI interface name, e.g. "lan"
	Enabled    bool          `json:"enabled"`     // DHCP ignore = false
	Start      int           `json:"start"`       // .100
	Limit      int           `json:"limit"`       // 150
	LeaseTime  string        `json:"lease_time"`  // "12h"
	UpstreamDNS []string     `json:"upstream_dns"` // e.g. ["192.168.1.53"]
	StaticLeases []StaticLease `json:"static_leases"`
}

// PortForwardRule is a single DNAT rule in /etc/config/firewall.
type PortForwardRule struct {
	Name     string `json:"name"`
	Proto    string `json:"proto"`    // "tcp" | "udp" | "tcp udp"
	SrcPort  int    `json:"src_port"` // external / WAN port (0 = any)
	DestIP   string `json:"dest_ip"`  // internal host IP
	DestPort int    `json:"dest_port"` // internal port
	Enabled  bool   `json:"enabled"`
}

// EdgeNetworkConfig is the composite object returned/accepted by the API.
type EdgeNetworkConfig struct {
	DeviceID    string             `json:"device_id"`
	Interfaces  []NetworkInterface `json:"interfaces"`
	DHCP        []DHCPInterface    `json:"dhcp"`
	PortForward []PortForwardRule  `json:"port_forwarding"`
}

// ─── UCI Serialisers ─────────────────────────────────────────────────────────

// BuildNetworkUCI serialises the Interfaces slice into OpenWrt UCI text commands
// suitable for sourcing in a shell script on the device.
func BuildNetworkUCI(ifaces []NetworkInterface) string {
	var sb strings.Builder
	for _, i := range ifaces {
		n := i.Name
		sb.WriteString(fmt.Sprintf("uci set network.%s=interface\n", n))
		sb.WriteString(fmt.Sprintf("uci set network.%s.proto='%s'\n", n, i.Proto))
		if i.Device != "" {
			sb.WriteString(fmt.Sprintf("uci set network.%s.device='%s'\n", n, i.Device))
		}
		if i.VlanID > 0 {
			sb.WriteString(fmt.Sprintf("uci set network.%s.vid='%d'\n", n, i.VlanID))
		}
		if i.Proto == "static" {
			if i.IPAddr != "" {
				sb.WriteString(fmt.Sprintf("uci set network.%s.ipaddr='%s'\n", n, i.IPAddr))
			}
			if i.Netmask != "" {
				sb.WriteString(fmt.Sprintf("uci set network.%s.netmask='%s'\n", n, i.Netmask))
			}
			if i.Gateway != "" {
				sb.WriteString(fmt.Sprintf("uci set network.%s.gateway='%s'\n", n, i.Gateway))
			}
		}
	}
	sb.WriteString("uci commit network\n")
	return sb.String()
}

// BuildDHCPUCI serialises the DHCP slice into UCI text commands.
func BuildDHCPUCI(dhcpList []DHCPInterface) string {
	var sb strings.Builder
	for _, d := range dhcpList {
		n := d.Interface
		sb.WriteString(fmt.Sprintf("uci set dhcp.%s=dhcp\n", n))
		sb.WriteString(fmt.Sprintf("uci set dhcp.%s.interface='%s'\n", n, n))
		if !d.Enabled {
			sb.WriteString(fmt.Sprintf("uci set dhcp.%s.ignore='1'\n", n))
		} else {
			sb.WriteString(fmt.Sprintf("uci set dhcp.%s.ignore='0'\n", n))
			sb.WriteString(fmt.Sprintf("uci set dhcp.%s.start='%d'\n", n, d.Start))
			sb.WriteString(fmt.Sprintf("uci set dhcp.%s.limit='%d'\n", n, d.Limit))
			if d.LeaseTime != "" {
				sb.WriteString(fmt.Sprintf("uci set dhcp.%s.leasetime='%s'\n", n, d.LeaseTime))
			}
		}
		// Upstream DNS (dnsmasq server list)
		if len(d.UpstreamDNS) > 0 {
			sb.WriteString("uci -q delete dhcp.@dnsmasq[0].server\n")
			for _, dns := range d.UpstreamDNS {
				sb.WriteString(fmt.Sprintf("uci add_list dhcp.@dnsmasq[0].server='%s'\n", dns))
			}
		}
		// Static leases
		for _, sl := range d.StaticLeases {
			sb.WriteString("uci add dhcp host\n")
			sb.WriteString(fmt.Sprintf("uci set dhcp.@host[-1].name='%s'\n", sl.Name))
			sb.WriteString(fmt.Sprintf("uci set dhcp.@host[-1].mac='%s'\n", sl.MAC))
			sb.WriteString(fmt.Sprintf("uci set dhcp.@host[-1].ip='%s'\n", sl.IP))
		}
	}
	sb.WriteString("uci commit dhcp\n")
	return sb.String()
}

// BuildFirewallUCI serialises the PortForwardRule slice into UCI redirect rules.
func BuildFirewallUCI(rules []PortForwardRule) string {
	var sb strings.Builder
	for _, r := range rules {
		sb.WriteString("uci add firewall redirect\n")
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].name='%s'\n", r.Name))
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].target='DNAT'\n"))
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].src='wan'\n"))
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].src_dport='%d'\n", r.SrcPort))
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].proto='%s'\n", r.Proto))
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].dest_ip='%s'\n", r.DestIP))
		sb.WriteString(fmt.Sprintf("uci set firewall.@redirect[-1].dest_port='%d'\n", r.DestPort))
		if !r.Enabled {
			sb.WriteString("uci set firewall.@redirect[-1].enabled='0'\n")
		} else {
			sb.WriteString("uci set firewall.@redirect[-1].enabled='1'\n")
		}
	}
	sb.WriteString("uci commit firewall\n")
	return sb.String()
}

// BuildValidatedReloadScript returns a shell script that:
//  1. Applies the UCI commands.
//  2. Runs a syntax validation pass.
//  3. Only calls `network reload` / `dnsmasq restart` if validation passes;
//     otherwise rolls back and returns a non-zero exit code.
func BuildValidatedReloadScript(networkUCI, dhcpUCI, firewallUCI string) string {
	return fmt.Sprintf(`#!/bin/sh
set -e

# ──────────────────────────────────────────────────────────────
# EDGE_NEXUS — Validated configuration push (Nerve Center)
# ──────────────────────────────────────────────────────────────

logger -t edge_nexus "EDGE_NEXUS: starting config push"

# 1. Backup current state
uci export network  > /tmp/edge_bak_network.conf  2>/dev/null  || true
uci export dhcp     > /tmp/edge_bak_dhcp.conf     2>/dev/null  || true
uci export firewall > /tmp/edge_bak_firewall.conf 2>/dev/null  || true

rollback() {
  logger -t edge_nexus "EDGE_NEXUS: ERROR — rolling back config"
  uci import network  < /tmp/edge_bak_network.conf  2>/dev/null || true
  uci import dhcp     < /tmp/edge_bak_dhcp.conf     2>/dev/null || true
  uci import firewall < /tmp/edge_bak_firewall.conf 2>/dev/null || true
  uci commit network
  uci commit dhcp
  uci commit firewall
  exit 1
}
trap rollback ERR

# 2. Apply UCI changes
%s
%s
%s

# 3. Validate: uci show network must parse cleanly
uci show network > /dev/null 2>&1 || { logger -t edge_nexus "network validation failed"; rollback; }
uci show dhcp    > /dev/null 2>&1 || { logger -t edge_nexus "dhcp validation failed";    rollback; }
uci show firewall> /dev/null 2>&1 || { logger -t edge_nexus "firewall validation failed"; rollback; }

# 4. Reload services
/etc/init.d/network reload  && logger -t edge_nexus "network reloaded"
/etc/init.d/dnsmasq restart && logger -t edge_nexus "dnsmasq restarted"
/etc/init.d/firewall restart && logger -t edge_nexus "firewall restarted"

logger -t edge_nexus "EDGE_NEXUS: config push complete"
exit 0
`, networkUCI, dhcpUCI, firewallUCI)
}

// MarshalEdgeConfig is a helper for JSON serialisation used in tests / DB storage.
func MarshalEdgeConfig(cfg EdgeNetworkConfig) (string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
