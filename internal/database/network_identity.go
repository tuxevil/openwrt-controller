package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	networkMACPattern  = regexp.MustCompile(`(?i)(?:[0-9a-f]{2}:){5}[0-9a-f]{2}`)
	networkIPv4Pattern = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
)

// NetworkIdentity is the human-readable identity assembled from operator
// labels and the telemetry snapshots reported by the nodes.
type NetworkIdentity struct {
	SiteID         string
	MAC            string
	Kind           string
	Label          string
	Hostname       string
	Model          string
	IP             string
	UplinkDevice   string
	UplinkLabel    string
	Manual         bool
	Trusted        bool
	TrustLabel     string
	TrustReason    string
	TrustExpiresAt *time.Time
}

func (identity NetworkIdentity) IsTrusted(now time.Time) bool {
	return identity.Trusted && (identity.TrustExpiresAt == nil || now.Before(*identity.TrustExpiresAt))
}

func (identity NetworkIdentity) DisplayLabel() string {
	for _, value := range []string{identity.Label, identity.Hostname, identity.Model} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	prefix := identity.Kind
	if prefix == "" {
		prefix = "client"
	}
	if suffix := identityShortSuffix(identity.MAC); suffix != "" {
		return prefix + "-" + suffix
	}
	return prefix
}

// NormalizeMAC preserves non-MAC device IDs while canonicalizing real MACs.
func NormalizeMAC(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := net.ParseMAC(raw)
	if err == nil && len(parsed) == 6 {
		return strings.ToUpper(parsed.String())
	}
	return strings.ToUpper(raw)
}

func networkIdentityKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToUpper(NormalizeMAC(raw))
}

func identityShortSuffix(raw string) string {
	compact := strings.NewReplacer(":", "", "-", "").Replace(strings.ToUpper(strings.TrimSpace(raw)))
	if len(compact) < 4 {
		return compact
	}
	return compact[len(compact)-4:]
}

// AnnotateNetworkText appends identity details for MAC/IP references without
// destroying the original log message. This keeps raw evidence auditable while
// giving Sentinel a readable label, uplink, and trust state.
func AnnotateNetworkText(message string, directory map[string]NetworkIdentity) string {
	if strings.TrimSpace(message) == "" || len(directory) == 0 {
		return message
	}
	if strings.Contains(message, "| identity:") {
		return message
	}
	type reference struct {
		start int
		value string
	}
	refs := make([]reference, 0)
	for _, match := range networkMACPattern.FindAllStringIndex(message, -1) {
		refs = append(refs, reference{start: match[0], value: message[match[0]:match[1]]})
	}
	for _, match := range networkIPv4Pattern.FindAllStringIndex(message, -1) {
		refs = append(refs, reference{start: match[0], value: message[match[0]:match[1]]})
	}
	// Preserve the order in which references appear in the original message.
	for i := 1; i < len(refs); i++ {
		for j := i; j > 0 && refs[j].start < refs[j-1].start; j-- {
			refs[j], refs[j-1] = refs[j-1], refs[j]
		}
	}

	seen := map[string]bool{}
	annotations := make([]string, 0, len(refs))
	for _, ref := range refs {
		identity, ok := lookupNetworkIdentity(directory, ref.value)
		if !ok {
			continue
		}
		key := networkIdentityKey(identity.MAC)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		annotations = append(annotations, formatNetworkIdentity(identity, time.Now()))
	}
	if len(annotations) == 0 {
		return message
	}
	return message + " | identity: " + strings.Join(annotations, "; ")
}

func formatNetworkIdentity(identity NetworkIdentity, now time.Time) string {
	parts := []string{identity.DisplayLabel()}
	if identity.MAC != "" {
		parts = append(parts, "MAC="+identity.MAC)
	}
	if identity.IP != "" {
		parts = append(parts, "IP="+identity.IP)
	}
	if identity.UplinkLabel != "" {
		parts = append(parts, "uplink="+identity.UplinkLabel)
	}
	if identity.IsTrusted(now) {
		parts = append(parts, "TRUSTED")
	} else if identity.Trusted {
		parts = append(parts, "TRUST_EXPIRED")
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func lookupNetworkIdentity(directory map[string]NetworkIdentity, reference string) (NetworkIdentity, bool) {
	if identity, ok := directory[networkIdentityKey(reference)]; ok {
		return identity, true
	}
	for _, identity := range directory {
		if identity.IP == reference {
			return identity, true
		}
	}
	return NetworkIdentity{}, false
}

// ResolveNetworkIdentity exposes the same MAC/IP lookup used by report
// enrichment to callers that need to normalize structured device lists.
func ResolveNetworkIdentity(directory map[string]NetworkIdentity, reference string) (NetworkIdentity, bool) {
	return lookupNetworkIdentity(directory, reference)
}

// LoadNetworkIdentityDirectory returns node and client identities for one
// tenant, optionally limited to a site. It joins operator labels/trust with
// DHCP, ARP, bridge, and wireless telemetry snapshots.
func LoadNetworkIdentityDirectory(schema, siteID string) (map[string]NetworkIdentity, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return nil, err
	}
	directory := map[string]NetworkIdentity{}
	if DB == nil {
		return directory, fmt.Errorf("database is unavailable")
	}

	manualRows, err := DB.Query(fmt.Sprintf(`SELECT mac, COALESCE(site_id::text, ''), hostname,
		COALESCE(trusted, false), COALESCE(trusted_label, ''), COALESCE(trusted_reason, ''), trust_expires_at
		FROM %s.client_hostnames WHERE ($1 = '' OR site_id::text = $1)`, safeSchema), siteID)
	if err != nil {
		return directory, err
	}
	for manualRows.Next() {
		var mac, rowSite, hostname, trustLabel, trustReason string
		var trusted bool
		var expires sql.NullTime
		if scanErr := manualRows.Scan(&mac, &rowSite, &hostname, &trusted, &trustLabel, &trustReason, &expires); scanErr != nil {
			continue
		}
		trimmedRef := strings.TrimSpace(mac)
		identity := NetworkIdentity{
			SiteID: rowSite, MAC: NormalizeMAC(trimmedRef), Kind: "client", Label: strings.TrimSpace(trustLabel),
			Hostname: strings.TrimSpace(hostname), Manual: true, Trusted: trusted, TrustReason: strings.TrimSpace(trustReason),
		}
		if net.ParseIP(trimmedRef) != nil {
			identity.IP = trimmedRef
		}
		if expires.Valid {
			value := expires.Time
			identity.TrustExpiresAt = &value
		}
		if identity.Label == "" {
			identity.Label = identity.Hostname
		}
		mergeNetworkIdentity(directory, identity)
	}
	manualRows.Close()

	deviceRows, err := DB.Query(fmt.Sprintf(`SELECT id, COALESCE(site_id::text, ''), COALESCE(name, ''), COALESCE(model, ''), state_json
		FROM %s.devices WHERE ($1 = '' OR site_id::text = $1)`, safeSchema), siteID)
	if err != nil {
		return directory, err
	}
	defer deviceRows.Close()
	for deviceRows.Next() {
		var deviceID, rowSite, name, model string
		var stateJSON []byte
		if scanErr := deviceRows.Scan(&deviceID, &rowSite, &name, &model, &stateJSON); scanErr != nil {
			continue
		}
		var state map[string]interface{}
		_ = json.Unmarshal(stateJSON, &state)
		board, _ := state["board"].(map[string]interface{})
		node := NetworkIdentity{
			SiteID: rowSite, MAC: NormalizeMAC(deviceID), Kind: "node", Label: strings.TrimSpace(name),
			Hostname: identityMapString(board, "hostname"), Model: strings.TrimSpace(model),
		}
		mergeNetworkIdentity(directory, node)
		uplinkLabel := node.DisplayLabel()
		addNetworkTelemetryIdentities(directory, rowSite, deviceID, uplinkLabel, state)
	}
	return directory, nil
}

func mergeNetworkIdentity(directory map[string]NetworkIdentity, candidate NetworkIdentity) {
	key := networkIdentityKey(candidate.MAC)
	if key == "" {
		return
	}
	candidate.MAC = NormalizeMAC(candidate.MAC)
	existing, ok := directory[key]
	if !ok {
		directory[key] = candidate
		return
	}
	if candidate.Kind == "node" || existing.Kind != "node" {
		if candidate.SiteID != "" {
			existing.SiteID = candidate.SiteID
		}
		if candidate.Kind == "node" {
			existing.Kind = "node"
		}
		for field, value := range map[string]string{
			"Label": candidate.Label, "Hostname": candidate.Hostname, "Model": candidate.Model,
			"IP": candidate.IP, "UplinkDevice": candidate.UplinkDevice, "UplinkLabel": candidate.UplinkLabel,
		} {
			if strings.TrimSpace(value) == "" {
				continue
			}
			switch field {
			case "Label":
				if !existing.Manual || existing.Label == "" {
					existing.Label = value
				}
			case "Hostname":
				if existing.Hostname == "" {
					existing.Hostname = value
				}
			case "Model":
				if existing.Model == "" {
					existing.Model = value
				}
			case "IP":
				if existing.IP == "" {
					existing.IP = value
				}
			case "UplinkDevice":
				if existing.UplinkDevice == "" {
					existing.UplinkDevice = value
				}
			case "UplinkLabel":
				if existing.UplinkLabel == "" {
					existing.UplinkLabel = value
				}
			}
		}
	}
	if candidate.Manual {
		existing.Manual = true
	}
	if candidate.Trusted {
		existing.Trusted = true
		existing.TrustLabel = candidate.TrustLabel
		existing.TrustReason = candidate.TrustReason
		existing.TrustExpiresAt = candidate.TrustExpiresAt
	}
	existing.MAC = candidate.MAC
	directory[key] = existing
}

func addNetworkTelemetryIdentities(directory map[string]NetworkIdentity, siteID, deviceID, uplinkLabel string, state map[string]interface{}) {
	addStationBlock := func(raw interface{}) {
		stations, ok := raw.([]interface{})
		if !ok {
			return
		}
		for _, entryRaw := range stations {
			entry, ok := entryRaw.(map[string]interface{})
			if !ok {
				continue
			}
			mac := identityMapString(entry, "mac")
			if mac == "" {
				continue
			}
			mergeNetworkIdentity(directory, NetworkIdentity{
				SiteID: siteID, MAC: mac, Kind: "client", Label: identityMapString(entry, "hostname"),
				IP: identityMapString(entry, "ip"), UplinkDevice: deviceID, UplinkLabel: uplinkLabel,
			})
		}
	}
	if stations, ok := state["wireless_stations"].(map[string]interface{}); ok {
		for _, raw := range stations {
			addStationBlock(raw)
		}
	}
	if wireless, ok := state["wireless"].(map[string]interface{}); ok {
		for _, radioRaw := range wireless {
			radio, ok := radioRaw.(map[string]interface{})
			if !ok {
				continue
			}
			interfaces, _ := radio["interfaces"].([]interface{})
			for _, ifaceRaw := range interfaces {
				iface, _ := ifaceRaw.(map[string]interface{})
				addStationBlock(iface["stations"])
			}
		}
	}

	var arpRaw, bridgeRaw interface{}
	if neighbors, ok := state["neighbor_stats"].(map[string]interface{}); ok {
		arpRaw, bridgeRaw = neighbors["arp_table"], neighbors["bridge_table"]
	}
	if arpRaw == nil {
		arpRaw = state["arp_table"]
	}
	if bridgeRaw == nil {
		bridgeRaw = state["bridge_table"]
	}
	addTable := func(raw interface{}, includeIP bool) {
		entries, ok := raw.([]interface{})
		if !ok {
			return
		}
		for _, entryRaw := range entries {
			entry, ok := entryRaw.(map[string]interface{})
			if !ok {
				continue
			}
			mac := identityMapString(entry, "mac")
			if mac == "" {
				continue
			}
			identity := NetworkIdentity{SiteID: siteID, MAC: mac, Kind: "client", UplinkDevice: deviceID, UplinkLabel: uplinkLabel}
			if includeIP {
				identity.IP = identityMapString(entry, "ip")
			}
			mergeNetworkIdentity(directory, identity)
		}
	}
	addTable(arpRaw, true)
	addTable(bridgeRaw, false)
	if dhcp, ok := state["dhcp"].(map[string]interface{}); ok {
		if leases, ok := dhcp["leases"].([]interface{}); ok {
			for _, leaseRaw := range leases {
				lease, ok := leaseRaw.(map[string]interface{})
				if !ok {
					continue
				}
				mac := identityMapString(lease, "mac")
				if mac == "" {
					continue
				}
				mergeNetworkIdentity(directory, NetworkIdentity{
					SiteID: siteID, MAC: mac, Kind: "client", Label: identityMapString(lease, "hostname"),
					Hostname: identityMapString(lease, "hostname"), IP: identityMapString(lease, "ip"),
					UplinkDevice: deviceID, UplinkLabel: uplinkLabel,
				})
			}
		}
	}
}

func identityMapString(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

// SiteClientHostnameLookup maps normalized MAC addresses and IPv4 addresses
// to operator-assigned or DHCP-reported hostnames within a site.
type SiteClientHostnameLookup struct {
	ByMAC map[string]string
	ByIP  map[string]string
}

// NewSiteClientHostnameLookup returns an initialized empty lookup table.
func NewSiteClientHostnameLookup() SiteClientHostnameLookup {
	return SiteClientHostnameLookup{
		ByMAC: make(map[string]string),
		ByIP:  make(map[string]string),
	}
}

// LoadSiteClientHostnameLookup loads custom client hostnames for a site using
// the tenant-scoped transaction on ctx.
func LoadSiteClientHostnameLookup(ctx context.Context, siteID string) SiteClientHostnameLookup {
	lookup := NewSiteClientHostnameLookup()
	if strings.TrimSpace(siteID) == "" {
		return lookup
	}
	tx := Tx(ctx)
	if tx == nil {
		return lookup
	}
	rows, err := tx.Query("SELECT mac, hostname FROM client_hostnames WHERE site_id = $1", siteID)
	if err != nil {
		return lookup
	}
	defer rows.Close()
	for rows.Next() {
		var mac, hostname string
		if err := rows.Scan(&mac, &hostname); err != nil {
			continue
		}
		hostname = strings.TrimSpace(hostname)
		if hostname == "" || hostname == "*" {
			continue
		}
		trimmedRef := strings.TrimSpace(mac)
		if net.ParseIP(trimmedRef) != nil {
			lookup.ByIP[trimmedRef] = hostname
		}
		if key := NormalizeMAC(trimmedRef); key != "" {
			lookup.ByMAC[key] = hostname
		}
	}
	return lookup
}

// MergeDeviceState indexes node identity, DHCP leases, and ARP entries from a
// device telemetry snapshot without overwriting custom operator hostnames.
func (l *SiteClientHostnameLookup) MergeDeviceState(deviceID, deviceName, lastIP string, state map[string]interface{}) {
	if l.ByMAC == nil {
		l.ByMAC = make(map[string]string)
	}
	if l.ByIP == nil {
		l.ByIP = make(map[string]string)
	}

	nodeName := strings.TrimSpace(deviceName)
	if nodeName == "" && state != nil {
		if board, ok := state["board"].(map[string]interface{}); ok {
			nodeName = identityMapString(board, "hostname")
		}
	}
	if key := NormalizeMAC(deviceID); key != "" {
		if nodeName != "" && l.ByMAC[key] == "" {
			l.ByMAC[key] = nodeName
		}
		if ip := strings.TrimSpace(lastIP); ip != "" {
			if resolved := l.ByMAC[key]; resolved != "" && l.ByIP[ip] == "" {
				l.ByIP[ip] = resolved
			}
		}
	}

	if state == nil {
		return
	}

	if dhcpBlock, ok := state["dhcp"].(map[string]interface{}); ok {
		if leases, ok := dhcpBlock["leases"].([]interface{}); ok {
			for _, leaseRaw := range leases {
				lease, ok := leaseRaw.(map[string]interface{})
				if !ok {
					continue
				}
				mac := NormalizeMAC(identityMapString(lease, "mac"))
				ip := identityMapString(lease, "ip")
				host := identityMapString(lease, "hostname")
				if host == "*" {
					host = ""
				}
				if mac != "" && host != "" && l.ByMAC[mac] == "" {
					l.ByMAC[mac] = host
				}
				if ip != "" {
					resolved := host
					if mac != "" && l.ByMAC[mac] != "" {
						resolved = l.ByMAC[mac]
					}
					if resolved != "" {
						l.ByIP[ip] = resolved
					}
				}
			}
		}
	}

	var arpRaw interface{}
	if ns, ok := state["neighbor_stats"].(map[string]interface{}); ok {
		arpRaw = ns["arp_table"]
	}
	if arpRaw == nil {
		arpRaw = state["arp_table"]
	}
	if entries, ok := arpRaw.([]interface{}); ok {
		for _, entryRaw := range entries {
			entry, ok := entryRaw.(map[string]interface{})
			if !ok {
				continue
			}
			mac := NormalizeMAC(identityMapString(entry, "mac"))
			ip := identityMapString(entry, "ip")
			if mac != "" && ip != "" {
				if resolved := l.ByMAC[mac]; resolved != "" {
					l.ByIP[ip] = resolved
				}
			}
		}
	}
}

// ResolveMAC returns the hostname for a MAC address, if known.
func (l SiteClientHostnameLookup) ResolveMAC(mac string) string {
	key := NormalizeMAC(mac)
	if key == "" {
		return ""
	}
	return l.ByMAC[key]
}

// ResolveIP returns the hostname for an IPv4 address, if known.
func (l SiteClientHostnameLookup) ResolveIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	return l.ByIP[ip]
}

// Resolve returns the hostname for either a MAC address or an IP address.
func (l SiteClientHostnameLookup) Resolve(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if host := l.ResolveMAC(ref); host != "" {
		return host
	}
	return l.ResolveIP(ref)
}

// DeviceDisplayName picks the most human-readable name for an OpenWrt node.
func DeviceDisplayName(id string, name *string, state map[string]interface{}, lookup SiteClientHostnameLookup) string {
	if name != nil && strings.TrimSpace(*name) != "" {
		return strings.TrimSpace(*name)
	}
	if state != nil {
		if board, ok := state["board"].(map[string]interface{}); ok {
			if h := identityMapString(board, "hostname"); h != "" {
				return h
			}
		}
	}
	if h := lookup.ResolveMAC(id); h != "" {
		return h
	}
	return id
}

var (
	dhcpHostnameSyncMu   sync.Mutex
	dhcpHostnameSyncSeen = map[string]string{}
)

// ExtractDHCPHostnameMap extracts canonical MAC -> hostname pairs from a device
// telemetry payload's dhcp.leases array, ignoring empty or wildcard names.
func ExtractDHCPHostnameMap(state map[string]interface{}) map[string]string {
	out := make(map[string]string)
	if state == nil {
		return out
	}
	dhcpBlock, ok := state["dhcp"].(map[string]interface{})
	if !ok {
		return out
	}
	leases, ok := dhcpBlock["leases"].([]interface{})
	if !ok {
		return out
	}
	for _, leaseRaw := range leases {
		lease, ok := leaseRaw.(map[string]interface{})
		if !ok {
			continue
		}
		rawMAC := identityMapString(lease, "mac")
		host := identityMapString(lease, "hostname")
		if rawMAC == "" || host == "" || host == "*" {
			continue
		}
		parsed, err := net.ParseMAC(rawMAC)
		if err != nil || len(parsed) != 6 {
			continue
		}
		out[strings.ToUpper(parsed.String())] = host
	}
	return out
}

// SyncDHCPHostnamesFromState persists newly discovered DHCP client hostnames
// into client_hostnames without overwriting existing operator-defined names.
func SyncDHCPHostnamesFromState(schema, siteID string, state map[string]interface{}) {
	if DB == nil || strings.TrimSpace(siteID) == "" {
		return
	}
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return
	}
	discovered := ExtractDHCPHostnameMap(state)
	if len(discovered) == 0 {
		return
	}

	pending := make(map[string]string)
	dhcpHostnameSyncMu.Lock()
	for mac, host := range discovered {
		cacheKey := safeSchema + "|" + siteID + "|" + mac
		if dhcpHostnameSyncSeen[cacheKey] != host {
			pending[mac] = host
		}
	}
	dhcpHostnameSyncMu.Unlock()

	if len(pending) == 0 {
		return
	}

	// #nosec G201 -- safeSchema is validated by SafeSchemaIdent; values are parameterized.
	query := fmt.Sprintf(`
		INSERT INTO %s.client_hostnames (mac, site_id, hostname, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (mac) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			updated_at = CURRENT_TIMESTAMP
		WHERE NULLIF(%s.client_hostnames.hostname, '') IS NULL
	`, safeSchema, safeSchema)

	for mac, host := range pending {
		if _, err := DB.Exec(query, mac, siteID, host); err != nil {
			continue
		}
		dhcpHostnameSyncMu.Lock()
		dhcpHostnameSyncSeen[safeSchema+"|"+siteID+"|"+mac] = host
		dhcpHostnameSyncMu.Unlock()
	}
}
