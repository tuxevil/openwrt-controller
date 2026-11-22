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

	// Fetch custom hostnames
	customHostnames := make(map[string]string)
	hRows, err := database.Tx(r.Context()).Query("SELECT mac, hostname FROM client_hostnames WHERE site_id = $1", siteID)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var m, h string
			if err := hRows.Scan(&m, &h); err == nil {
				customHostnames[m] = h
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
						if mac != "" && hostname != "" && hostname != "*" {
							dhcpHostnames[mac] = hostname
						}
					}
				}
			}
		}
	}

	edgeCounter := 0
	// 3. Process each router
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

		// Wired Links via BridgeTable
		var brTable []interface{}
		if neighborStats, ok := dev["neighbor_stats"].(map[string]interface{}); ok {
			if bt, ok := neighborStats["bridge_table"].([]interface{}); ok {
				brTable = bt
			}
		} else if bt, ok := dev["bridge_table"].([]interface{}); ok {
			brTable = bt
		}

		if len(brTable) > 0 {
			for _, entry := range brTable {
				if brEntry, ok := entry.(map[string]interface{}); ok {
					childMAC, okMac := brEntry["mac"].(string)
					childMAC = strings.ToLower(childMAC)
					isLocal, _ := brEntry["is_local"].(string) // "no" means it's learned passing through
					if okMac && knownRouters[childMAC] && childMAC != strings.ToLower(devMAC) && isLocal == "no" {
						linkMeta := metadataLinks[strings.ToLower(devMAC)+"->"+childMAC]
						edgeID := fmt.Sprintf("edge%d", edgeCounter)
						edgeCounter++
						graph.Edges[edgeID] = GraphEdge{
							Source: strings.ToLower(devMAC),
							Target: childMAC,
							Type:   "wired",
							Label:  linkMeta.label,
							Speed:  linkMeta.speed,
						}
					}
				}
			}
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
									clientName := "CLIENT_" + clientMAC[len(clientMAC)-5:]
									if name, ok := customHostnames[clientMAC]; ok && name != "" {
										clientName = name
									} else if name, ok := dhcpHostnames[clientMAC]; ok && name != "" {
										clientName = name
									}

									graph.Nodes[clientMAC] = GraphNode{
										ID:       clientMAC,
										Name:     clientName,
										Type:     "client",
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
	for _, link := range metadata.Links {
		if link.Source == "" || link.Target == "" {
			continue
		}
		graph.Edges[fmt.Sprintf("metadata%d", edgeCounter)] = GraphEdge{Source: strings.ToLower(link.Source), Target: strings.ToLower(link.Target), Type: "wired", Label: link.Label, Speed: link.Speed}
		edgeCounter++
	}
	if graph.Metadata == nil {
		graph.Metadata = map[string]interface{}{}
	}
	graph.Metadata["roles"] = metadata.Roles

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": graph})
}
