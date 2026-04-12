package main

import (
"encoding/json"
"fmt"
"log"

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

func main() {
database.InitPostgres()
siteID := ""
database.DB.QueryRow("SELECT id FROM sites LIMIT 1").Scan(&siteID)

graph := TopologyGraph{
odes: make(map[string]GraphNode),
g]GraphEdge),
}

rows, err := database.DB.Query("SELECT id, state_json FROM devices WHERE site_id = $1", siteID)
if err != nil {
allDevices []map[string]interface{}
knownRouters := make(map[string]bool)

for rows.Next() {
string
 []byte
:= rows.Scan(&id, &stateJSON); err == nil {
ownRouters[id] = true
(stateJSON) > 0 {
load map[string]interface{}
:= json.Unmarshal(stateJSON, &payload); err == nil {
= id
append(allDevices, payload)
ter := 0
for _, dev := range allDevices {
dev["_id"].(string)
ame := devMAC
ok := dev["board"].(map[string]interface{}); ok {
ok := board["hostname"].(string); ok {
ame = h
"N/A"
s, ok := dev["system"].(map[string]interface{}); ok {
ok := sys["load"].([]interface{}); ok && len(loadStr) > 0 {
ok := loadStr[0].(float64); ok {
fmt.Sprintf("%.2f%%", (l1/65535.0)*100)
odes[devMAC] = GraphNode{
     devMAC,
ame:     hostname,
    "router",
ame: hostname,
cpuLoad,
ok := dev["bridge_table"].([]interface{}); ok {
entry := range brTable {
try, ok := entry.(map[string]interface{}); ok {
:= brEntry["mac"].(string)
:= brEntry["is_local"].(string) 
&& knownRouters[childMAC] && childMAC != devMAC && isLocal == "no" {
fmt.Sprintf("edge%d", edgeCounter)
ter++
GraphEdge{
childMAC,
  "wired",
_ := json.MarshalIndent(graph, "", "  ")
fmt.Println(string(b))
}
