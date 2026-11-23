package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"openwrt-controller/internal/database"
)

type GraphNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`      // 'router' or 'client'
	HasAlert bool   `json:"has_alert"` // From The Signal incidents
	Hostname string `json:"hostname,omitempty"`
	CPULoad  string `json:"cpu_load,omitempty"`
	Label    string `json:"label,omitempty"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // 'wired' or 'wireless'
	Label  string `json:"label,omitempty"`
	Speed  string `json:"speed,omitempty"`
}

type TopologyGraph struct {
	Nodes    map[string]GraphNode   `json:"nodes"`
	Edges    map[string]GraphEdge   `json:"edges"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type topologyMetadata struct {
	WAN *struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Address   string `json:"address"`
		LinkLabel string `json:"link_label"`
		Speed     string `json:"speed"`
		Target    string `json:"target,omitempty"`
	} `json:"wan"`
	Links []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Label  string `json:"label"`
		Speed  string `json:"speed"`
	} `json:"links"`
	Roles map[string]string `json:"roles"`
}

func isGatewayRole(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), "gateway")
}

func hasTelemetryWAN(dev map[string]interface{}) bool {
	wanRaw, ok := dev["wan"]
	if !ok || wanRaw == nil {
		return false
	}
	if wanMap, ok := wanRaw.(map[string]interface{}); ok {
		return len(wanMap) > 0
	}
	return true
}

func undirectedPairKey(a, b string) string {
	if a < b {
		return a + "<->" + b
	}
	return b + "<->" + a
}

func extractBridgeTable(dev map[string]interface{}) []interface{} {
	if neighborStats, ok := dev["neighbor_stats"].(map[string]interface{}); ok {
		if bt, ok := neighborStats["bridge_table"].([]interface{}); ok {
			return bt
		}
	}
	if bt, ok := dev["bridge_table"].([]interface{}); ok {
		return bt
	}
	return nil
}

func bridgePortString(entry map[string]interface{}) string {
	switch v := entry["port"].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return ""
	}
}

func GetSiteTopologyHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id required"}`, http.StatusBadRequest)
		return
	}

	graph := TopologyGraph{
		Nodes: make(map[string]GraphNode),
		Edges: make(map[string]GraphEdge),
	}
	var metadataJSON []byte
	_ = database.Tx(r.Context()).QueryRow("SELECT COALESCE(topology_metadata, '{}'::jsonb) FROM site_configs WHERE site_id = $1", siteID).Scan(&metadataJSON)
	var metadata topologyMetadata
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &metadata)
	}
	metadataLinks := make(map[string]struct {
		label string
		speed string
	})
	for _, link := range metadata.Links {
		key := strings.ToLower(link.Source) + "->" + strings.ToLower(link.Target)
		metadataLinks[key] = struct {
			label string
			speed string
		}{label: link.Label, speed: link.Speed}
	}
	roleMeta := make(map[string]string, len(metadata.Roles))
	for k, v := range metadata.Roles {
		roleMeta[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	if metadata.WAN != nil {
		wanID := metadata.WAN.ID
		if wanID == "" {
			wanID = "wan"
		}
		graph.Nodes[wanID] = GraphNode{ID: wanID, Name: metadata.WAN.Name, Type: "wan", Hostname: metadata.WAN.Address, Label: metadata.WAN.Name}
	}

	// 1. Fetch devices, state_json, and role
	rows, err := database.Tx(r.Context()).Query("SELECT id, state_json, COALESCE(device_role, '') FROM devices WHERE site_id = $1", siteID)
	if err != nil {
		log.Printf("Topology query error: %v", err)
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allDevices []map[string]interface{}
	knownRouters := make(map[string]bool)

	for rows.Next() {
		var id string
		var stateJSON []byte
		var deviceRole string
		if err := rows.Scan(&id, &stateJSON, &deviceRole); err == nil {
			lowerID := strings.ToLower(id)
			knownRouters[lowerID] = true
			if len(stateJSON) > 0 {
				var payload map[string]interface{}
				if err := json.Unmarshal(stateJSON, &payload); err == nil {
					payload["_id"] = lowerID
					payload["_device_role"] = deviceRole
					allDevices = append(allDevices, payload)
				}
			}
		}
	}

	// 2. Fetch active incidents for the site to flag nodes
	activeIncidents := make(map[string]bool)
	incRows, err := database.Tx(r.Context()).Query("SELECT device_id FROM incidents WHERE site_id = $1 AND status = 'OPEN'", siteID)
	if err == nil {
		defer incRows.Close()
		for incRows.Next() {
			var devID string
			if err := incRows.Scan(&devID); err == nil {
				activeIncidents[devID] = true
			}
		}
	}

	// Fetch custom hostnames (normalized MAC keys so DB uppercase matches telemetry lowercase)
	customHostnames := make(map[string]string)
	hRows, err := database.Tx(r.Context()).Query("SELECT mac, hostname FROM client_hostnames WHERE site_id = $1", siteID)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var m, h string
			if err := hRows.Scan(&m, &h); err == nil {
				h = strings.TrimSpace(h)
				if h != "" && h != "*" {
					customHostnames[database.NormalizeMAC(m)] = h
				}
			}
		}
	}

	// Extract DHCP hostnames
	dhcpHostnames := make(map[string]string)
	for _, dev := range allDevices {
		if dhcpBlock, ok := dev["dhcp"].(map[string]interface{}); ok {
			if leases, ok := dhcpBlock["leases"].([]interface{}); ok {
				for _, leaseRaw := range leases {
					if lease, ok := leaseRaw.(map[string]interface{}); ok {
						mac, _ := lease["mac"].(string)
						hostname, _ := lease["hostname"].(string)
						hostname = strings.TrimSpace(hostname)
						if mac != "" && hostname != "" && hostname != "*" {
							dhcpHostnames[database.NormalizeMAC(mac)] = hostname
						}
					}
				}
			}
		}
	}

	edgeCounter := 0
	nodeTypes := make(map[string]string, len(allDevices))
	peerPorts := make(map[string]map[string]string, len(allDevices))

	// 3. Process each router: nodes, WAN edges, bridge-table observations, and wireless clients
	for _, dev := range allDevices {
		devMAC := dev["_id"].(string)

		hostname := devMAC
		if board, ok := dev["board"].(map[string]interface{}); ok {
			if h, ok := board["hostname"].(string); ok {
				hostname = h
			}
		}

		cpuLoad := "N/A"
		if sys, ok := dev["system"].(map[string]interface{}); ok {
			if loadStr, ok := sys["load"].([]interface{}); ok && len(loadStr) > 0 {
				if l1, ok := loadStr[0].(float64); ok {
					cpuLoad = fmt.Sprintf("%.2f%%", (l1/65535.0)*100)
				}
			}
		}

		deviceRole, _ := dev["_device_role"].(string)
		nodeType := "ap"
		if isGatewayRole(deviceRole) || isGatewayRole(roleMeta[devMAC]) || hasTelemetryWAN(dev) {
			nodeType = "gateway"
		}
		nodeTypes[devMAC] = nodeType

		// Add the router node
		graph.Nodes[devMAC] = GraphNode{
			ID:       devMAC,
			Name:     hostname,
			Type:     nodeType,
			HasAlert: activeIncidents[devMAC],
			Hostname: hostname,
			CPULoad:  cpuLoad,
		}

		// Connect WAN Root Node only to the gateway (or an explicit metadata.wan.target)
		if metadata.WAN != nil {
			wanTarget := strings.ToLower(strings.TrimSpace(metadata.WAN.Target))
			attachWAN := (wanTarget != "" && wanTarget == devMAC) || (wanTarget == "" && nodeType == "gateway")
			if attachWAN {
				wanID := metadata.WAN.ID
				if wanID == "" {
					wanID = "wan"
				}
				if _, exists := graph.Edges["wan-"+devMAC]; !exists {
					wanName := metadata.WAN.Name
					wanAddress := metadata.WAN.Address
					graph.Nodes[wanID] = GraphNode{
						ID:       wanID,
						Name:     wanName,
						Type:     "wan",
						Hostname: wanAddress,
						HasAlert: false,
					}
					edgeID := "wan-" + devMAC
					edgeCounter++
					graph.Edges[edgeID] = GraphEdge{
						Source: wanID,
						Target: devMAC,
						Type:   "wan", Label: metadata.WAN.LinkLabel, Speed: metadata.WAN.Speed,
					}
				}
			}
		}

		// Record remote router MACs seen on each bridge port for pass 2 wired inference
		if brTable := extractBridgeTable(dev); len(brTable) > 0 {
			ports := make(map[string]string)
			for _, entry := range brTable {
				brEntry, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				childMAC, okMac := brEntry["mac"].(string)
				childMAC = strings.ToLower(strings.TrimSpace(childMAC))
				isLocal, _ := brEntry["is_local"].(string)
				if okMac && knownRouters[childMAC] && childMAC != devMAC && isLocal == "no" {
					ports[childMAC] = bridgePortString(brEntry)
				}
			}
			peerPorts[devMAC] = ports
		}

		// Wireless Links via WirelessStations
		if wStations, ok := dev["wireless_stations"].(map[string]interface{}); ok {
			for _, clientsList := range wStations {
				if clients, ok := clientsList.([]interface{}); ok {
					for _, cIf := range clients {
						if cMap, ok := cIf.(map[string]interface{}); ok {
							clientMAC, okMac := cMap["mac"].(string)
							if okMac {
								if _, exists := graph.Nodes[clientMAC]; !exists {
									normClientMAC := database.NormalizeMAC(clientMAC)
									clientName := "CLIENT_" + clientMAC[len(clientMAC)-5:]
									resolvedHost := ""
									if name, ok := customHostnames[normClientMAC]; ok && name != "" {
										clientName = name
										resolvedHost = name
									} else if name, ok := dhcpHostnames[normClientMAC]; ok && name != "" {
										clientName = name
										resolvedHost = name
									}

									graph.Nodes[clientMAC] = GraphNode{
										ID:       clientMAC,
										Name:     clientName,
										Type:     "client",
										Hostname: resolvedHost,
										HasAlert: false,
									}
								}
								edgeID := fmt.Sprintf("edge%d", edgeCounter)
								edgeCounter++
								graph.Edges[edgeID] = GraphEdge{
									Source: devMAC,
									Target: clientMAC,
									Type:   "wireless",
								}
							}
						}
					}
				}
			}
		}
	}

	// 4. Resolve wired links (deduplicated undirected pairs).
	// Declared physical links in topology_metadata.links are authoritative for the
	// nodes they cover; bridge_table inference fills in undeclared nodes while
	// suppressing transitive L2 sibling links learned behind an uplink port.
	wiredPairs := make(map[string]bool)
	declaredWiredNodes := make(map[string]bool)
	addWiredEdge := func(edgePrefix, src, dst, label, speed string) {
		if src == "" || dst == "" || src == dst {
			return
		}
		pairKey := undirectedPairKey(src, dst)
		if wiredPairs[pairKey] {
			return
		}
		wiredPairs[pairKey] = true
		edgeCounter++
		graph.Edges[fmt.Sprintf("%s%d", edgePrefix, edgeCounter)] = GraphEdge{
			Source: src,
			Target: dst,
			Type:   "wired",
			Label:  label,
			Speed:  speed,
		}
	}

	for _, link := range metadata.Links {
		src := strings.ToLower(strings.TrimSpace(link.Source))
		dst := strings.ToLower(strings.TrimSpace(link.Target))
		if src == "" || dst == "" || src == dst {
			continue
		}
		declaredWiredNodes[src] = true
		declaredWiredNodes[dst] = true
		addWiredEdge("metadata", src, dst, link.Label, link.Speed)
	}

	hasGateway := false
	for _, role := range nodeTypes {
		if role == "gateway" {
			hasGateway = true
			break
		}
	}

	if hasGateway {
		// Identify downstream daisy-chained APs seen on a non-uplink port of another AP.
		downstreamParent := make(map[string]string)
		for _, dev := range allDevices {
			devMAC := dev["_id"].(string)
			if nodeTypes[devMAC] == "gateway" {
				continue
			}
			ports := peerPorts[devMAC]
			if len(ports) == 0 {
				continue
			}
			uplinkPorts := make(map[string]bool)
			for peerMAC, port := range ports {
				if nodeTypes[peerMAC] == "gateway" && port != "" {
					uplinkPorts[port] = true
				}
			}
			for peerMAC, port := range ports {
				if nodeTypes[peerMAC] == "gateway" {
					continue
				}
				if declaredWiredNodes[devMAC] && declaredWiredNodes[peerMAC] {
					continue
				}
				// If this AP sees a gateway on the same bridge port as peerMAC,
				// peerMAC is reachable transitively via the uplink, not directly cabled.
				if port == "" || uplinkPorts[port] {
					continue
				}
				linkMeta := metadataLinks[devMAC+"->"+peerMAC]
				addWiredEdge("edge", devMAC, peerMAC, linkMeta.label, linkMeta.speed)
				if _, exists := downstreamParent[peerMAC]; !exists {
					downstreamParent[peerMAC] = devMAC
				}
			}
		}

		// Connect gateways to directly attached routers, skipping grandchildren that
		// already attach behind a downstream AP on the same gateway bridge port.
		for _, dev := range allDevices {
			gwMAC := dev["_id"].(string)
			if nodeTypes[gwMAC] != "gateway" {
				continue
			}
			gwPorts := peerPorts[gwMAC]
			for peerMAC, gwPort := range gwPorts {
				if declaredWiredNodes[gwMAC] && declaredWiredNodes[peerMAC] {
					continue
				}
				if parentAP, ok := downstreamParent[peerMAC]; ok {
					if parentPort, hasParent := gwPorts[parentAP]; hasParent && parentPort == gwPort {
						continue
					}
				}
				linkMeta := metadataLinks[gwMAC+"->"+peerMAC]
				addWiredEdge("edge", gwMAC, peerMAC, linkMeta.label, linkMeta.speed)
			}
		}
	} else {
		// Fallback when no gateway is identified: deduplicate bridge_table edges.
		for _, dev := range allDevices {
			devMAC := dev["_id"].(string)
			for peerMAC := range peerPorts[devMAC] {
				if declaredWiredNodes[devMAC] && declaredWiredNodes[peerMAC] {
					continue
				}
				linkMeta := metadataLinks[devMAC+"->"+peerMAC]
				addWiredEdge("edge", devMAC, peerMAC, linkMeta.label, linkMeta.speed)
			}
		}
	}

	if graph.Metadata == nil {
		graph.Metadata = map[string]interface{}{}
	}
	graph.Metadata["roles"] = metadata.Roles

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": graph})
}
