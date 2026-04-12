package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"openwrt-controller/internal/database"
)

type GraphNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`      // 'router' or 'client'
	HasAlert  bool   `json:"has_alert"` // From The Signal incidents
	Hostname  string `json:"hostname,omitempty"`
	CPULoad   string `json:"cpu_load,omitempty"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // 'wired' or 'wireless'
}

type TopologyGraph struct {
	Nodes map[string]GraphNode `json:"nodes"`
	Edges map[string]GraphEdge `json:"edges"`
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

	// 1. Fetch devices and state_json
	rows, err := database.DB.Query("SELECT id, state_json FROM devices WHERE site_id = $1", siteID)
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
		if err := rows.Scan(&id, &stateJSON); err == nil {
			knownRouters[id] = true
			if len(stateJSON) > 0 {
				var payload map[string]interface{}
				if err := json.Unmarshal(stateJSON, &payload); err == nil {
					payload["_id"] = id
					allDevices = append(allDevices, payload)
				}
			}
		}
	}

	// 2. Fetch active incidents for the site to flag nodes
	activeIncidents := make(map[string]bool)
	incRows, err := database.DB.Query("SELECT device_id FROM incidents WHERE site_id = $1 AND status = 'OPEN'", siteID)
	if err == nil {
		defer incRows.Close()
		for incRows.Next() {
			var devID string
			if err := incRows.Scan(&devID); err == nil {
				activeIncidents[devID] = true
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

		// Add the router node
		graph.Nodes[devMAC] = GraphNode{
			ID:       devMAC,
			Name:     hostname,
			Type:     "router",
			HasAlert: activeIncidents[devMAC],
			Hostname: hostname,
			CPULoad:  cpuLoad,
		}

		// Wired Links via BridgeTable
		if brTable, ok := dev["bridge_table"].([]interface{}); ok {
			for _, entry := range brTable {
				if brEntry, ok := entry.(map[string]interface{}); ok {
					childMAC, okMac := brEntry["mac"].(string)
					isLocal, _ := brEntry["is_local"].(string) // "no" means it's learned passing through
					if okMac && knownRouters[childMAC] && childMAC != devMAC && isLocal == "no" {
						edgeID := fmt.Sprintf("edge%d", edgeCounter)
						edgeCounter++
						graph.Edges[edgeID] = GraphEdge{
							Source: devMAC,
							Target: childMAC,
							Type:   "wired",
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
									graph.Nodes[clientMAC] = GraphNode{
										ID:       clientMAC,
										Name:     "CLIENT_" + clientMAC[len(clientMAC)-5:],
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": graph})
}
