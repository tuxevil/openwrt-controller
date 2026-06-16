package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"openwrt-controller/internal/database"
)

type EchoNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // 'gateway', 'ap', or 'client'
	HasAlert bool   `json:"has_alert"`
	Hostname string `json:"hostname,omitempty"`
	CPULoad  string `json:"cpu_load,omitempty"`
}

type EchoEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // 'wired' or 'wireless'
	Speed  string `json:"speed,omitempty"`
}

type EchoGraph struct {
	Nodes []EchoNode `json:"nodes"`
	Links []EchoEdge `json:"links"`
}

func GenerateEchoLocation(ctx context.Context, siteID string) (EchoGraph, error) {
	graph := EchoGraph{
		Nodes: []EchoNode{},
		Links: []EchoEdge{},
	}

	nodesMap := make(map[string]EchoNode)

	rows, err := database.Tx(ctx).Query("SELECT id, state_json, device_role FROM devices WHERE site_id = $1", siteID)
	if err != nil {
		log.Printf("Topology query error: %v", err)
		return graph, err
	}
	defer rows.Close()

	var allDevices []map[string]interface{}
	knownRouters := make(map[string]bool)
	gatewayMACs := make(map[string]bool)

	for rows.Next() {
		var id string
		var stateJSON []byte
		var role string
		if err := rows.Scan(&id, &stateJSON, &role); err == nil {
			knownRouters[id] = true
			if role == "Gateway" {
				gatewayMACs[id] = true
			}
			if len(stateJSON) > 0 {
				var payload map[string]interface{}
				if err := json.Unmarshal(stateJSON, &payload); err == nil {
					payload["_id"] = id
					allDevices = append(allDevices, payload)
				}
			}
		}
	}

	activeIncidents := make(map[string]bool)
	incRows, err := database.Tx(ctx).Query("SELECT device_id FROM incidents WHERE site_id = $1 AND status = 'OPEN'", siteID)
	if err == nil {
		defer incRows.Close()
		for incRows.Next() {
			var devID string
			if err := incRows.Scan(&devID); err == nil {
				activeIncidents[devID] = true
			}
		}
	}

	customHostnames := make(map[string]string)
	hRows, err := database.Tx(ctx).Query("SELECT mac, hostname FROM client_hostnames WHERE site_id = $1", siteID)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var m, h string
			if err := hRows.Scan(&m, &h); err == nil {
				customHostnames[m] = h
			}
		}
	}

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

		nodeType := "ap"
		if gatewayMACs[devMAC] || hostname == "OpenWrt" {
			// heuristic if DB schema doesn't have is_gateway set reliably
			nodeType = "gateway"
		}

		nodesMap[devMAC] = EchoNode{
			ID:       devMAC,
			Name:     hostname,
			Type:     nodeType,
			HasAlert: activeIncidents[devMAC],
			Hostname: hostname,
			CPULoad:  cpuLoad,
		}

		var bridgeTable []interface{}
		if neighborStats, ok := dev["neighbor_stats"].(map[string]interface{}); ok {
			if bt, ok := neighborStats["bridge_table"].([]interface{}); ok {
				bridgeTable = bt
			}
		} else if bt, ok := dev["bridge_table"].([]interface{}); ok { // fallback
			bridgeTable = bt
		}

		for _, entry := range bridgeTable {
			if brEntry, ok := entry.(map[string]interface{}); ok {
				childMAC, okMac := brEntry["mac"].(string)
				isLocal, _ := brEntry["is_local"].(string)
				if okMac && knownRouters[childMAC] && childMAC != devMAC && isLocal == "no" {
					graph.Links = append(graph.Links, EchoEdge{
						Source: devMAC,
						Target: childMAC,
						Type:   "wired",
					})
				}
			}
		}

		if wStations, ok := dev["wireless_stations"].(map[string]interface{}); ok {
			for _, clientsList := range wStations {
				if clients, ok := clientsList.([]interface{}); ok {
					for _, cIf := range clients {
						if cMap, ok := cIf.(map[string]interface{}); ok {
							clientMAC, okMac := cMap["mac"].(string)
							if okMac {
								if _, exists := nodesMap[clientMAC]; !exists {
									clientName := "CLIENT_" + clientMAC[len(clientMAC)-5:]
									if name, ok := customHostnames[clientMAC]; ok && name != "" {
										clientName = name
									} else if name, ok := dhcpHostnames[clientMAC]; ok && name != "" {
										clientName = name
									}

									nodesMap[clientMAC] = EchoNode{
										ID:       clientMAC,
										Name:     clientName,
										Type:     "client",
										HasAlert: false,
									}
								}
								graph.Links = append(graph.Links, EchoEdge{
									Source: devMAC,
									Target: clientMAC,
									Type:   "wireless",
								})
							}
						}
					}
				}
			}
		}
	}

	for _, n := range nodesMap {
		graph.Nodes = append(graph.Nodes, n)
	}

	return graph, nil
}
