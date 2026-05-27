package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// runSSHCommand opens a short-lived SSH session to the device and runs cmd,
// returning combined stdout+stderr output.

// getDeviceIPAndSchema looks up a device in the public and all tenant schemas
// to find its target IP and which schema it belongs to, preferring the schema
// with the most recent last_seen_at timestamp.
func getDeviceIPAndSchema(deviceID string) (string, string, error) {
	type devMatch struct {
		ip         string
		schema     string
		lastSeenAt time.Time
	}
	var matches []devMatch

	// 1. Check public schema
	var publicIP sql.NullString
	var publicLastSeen sql.NullTime
	err := database.DB.QueryRow("SELECT last_ip, last_seen_at FROM public.devices WHERE id = $1", deviceID).Scan(&publicIP, &publicLastSeen)
	if err == nil && publicIP.Valid && publicIP.String != "" {
		lastSeen := time.Time{}
		if publicLastSeen.Valid {
			lastSeen = publicLastSeen.Time
		}
		matches = append(matches, devMatch{
			ip:         publicIP.String,
			schema:     "public",
			lastSeenAt: lastSeen,
		})
	}

	// 2. Query all active tenant schemas
	rows, err := database.DB.Query("SELECT schema_alias FROM public.tenants WHERE is_active = true")
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			continue
		}
		schema := "tenant_" + alias
		var tenantIP sql.NullString
		var tenantLastSeen sql.NullTime
		err := database.DB.QueryRow(fmt.Sprintf("SELECT last_ip, last_seen_at FROM %s.devices WHERE id = $1", schema), deviceID).Scan(&tenantIP, &tenantLastSeen)
		if err == nil && tenantIP.Valid && tenantIP.String != "" {
			lastSeen := time.Time{}
			if tenantLastSeen.Valid {
				lastSeen = tenantLastSeen.Time
			}
			matches = append(matches, devMatch{
				ip:         tenantIP.String,
				schema:     schema,
				lastSeenAt: lastSeen,
			})
		}
	}

	if len(matches) == 0 {
		return "", "", fmt.Errorf("device not found in any tenant")
	}

	// Find the match with the latest lastSeenAt
	bestMatch := matches[0]
	for _, m := range matches {
		if m.lastSeenAt.After(bestMatch.lastSeenAt) {
			bestMatch = m
		}
	}

	return bestMatch.ip, bestMatch.schema, nil
}

func runSSHCommand(deviceID string, cmd string) (string, error) {
	if PrivateKey == nil {
		return "", fmt.Errorf("controller SSH key not configured")
	}

	targetIP, _, err := getDeviceIPAndSchema(deviceID)
	if err != nil || targetIP == "" {
		return "", fmt.Errorf("device IP not found: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(PrivateKey)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	conn, err := ssh.Dial("tcp", targetIP+":22", cfg)
	if err != nil {
		return "", fmt.Errorf("SSH dial: %w", err)
	}
	defer conn.Close()

	sess, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session: %w", err)
	}
	defer sess.Close()

	outBytes, err := sess.CombinedOutput(cmd)
	if err != nil {
		return string(outBytes), fmt.Errorf("remote command failed: %w", err)
	}
	return string(outBytes), nil
}

func runSSHScript(deviceID string, script string) (string, error) {
	if PrivateKey == nil {
		return "", fmt.Errorf("controller SSH key not configured")
	}

	targetIP, _, err := getDeviceIPAndSchema(deviceID)
	if err != nil || targetIP == "" {
		return "", fmt.Errorf("device IP not found: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(PrivateKey)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         60 * time.Second,
	}
	conn, err := ssh.Dial("tcp", targetIP+":22", cfg)
	if err != nil {
		return "", fmt.Errorf("SSH dial: %w", err)
	}
	defer conn.Close()

	sess, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session: %w", err)
	}
	defer sess.Close()

	var out bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &out
	sess.Stdin = strings.NewReader(script)

	if err := sess.Run("sh"); err != nil {
		return out.String(), fmt.Errorf("script execution failed: %w", err)
	}
	return out.String(), nil
}

func readBody(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"cannot read body"}`, http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(body, target); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return false
	}
	return true
}

// ─── GET /api/devices/{id}/edge-network ──────────────────────────────────────

// GetEdgeNetworkHandler reads the current network interfaces from the device
// via `uci export network` and returns a structured JSON response.
func GetEdgeNetworkHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	raw, err := runSSHCommand(deviceID, "uci export network 2>/dev/null")
	if err != nil {
		// Return a graceful stub so the UI can still render
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_id":  deviceID,
			"interfaces": []interface{}{},
			"raw":        "",
			"error":      err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id":  deviceID,
		"interfaces": []interface{}{}, // UCI parsing happens client-side from raw
		"raw":        raw,
	})
}

// ─── PUT /api/devices/{id}/edge-network ──────────────────────────────────────

// PutEdgeNetworkHandler accepts a list of NetworkInterface objects, builds
// the corresponding UCI commands and pushes them to the device with validation.
func PutEdgeNetworkHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	username := GetUsernameFromReq(r)

	var payload struct {
		Interfaces []services.NetworkInterface `json:"interfaces"`
	}
	if !readBody(w, r, &payload) {
		return
	}

	uciCmds := services.BuildNetworkUCI(payload.Interfaces)
	script := services.BuildValidatedReloadScript(uciCmds, "", "")

	out, err := runSSHScript(deviceID, script)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":  err.Error(),
			"output": out,
		})
		return
	}

	database.InsertAuditLog(username, "EDGE_NEXUS_NETWORK_PUSH", "DEVICE", deviceID,
		fmt.Sprintf("Pushed %d interface(s)", len(payload.Interfaces)), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"output": out,
	})
}

// ─── GET /api/devices/{id}/edge-dhcp ─────────────────────────────────────────

func GetEdgeDHCPHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	dhcpRaw, dhcpErr := runSSHCommand(deviceID, "uci export dhcp 2>/dev/null")

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"device_id": deviceID,
		"dhcp":      []interface{}{},
		"raw":       dhcpRaw,
	}
	if dhcpErr != nil {
		resp["error"] = dhcpErr.Error()
	}
	json.NewEncoder(w).Encode(resp)
}

// ─── PUT /api/devices/{id}/edge-dhcp ─────────────────────────────────────────

func PutEdgeDHCPHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	username := GetUsernameFromReq(r)

	var payload struct {
		DHCP []services.DHCPInterface `json:"dhcp"`
	}
	if !readBody(w, r, &payload) {
		return
	}

	uciCmds := services.BuildDHCPUCI(payload.DHCP)
	script := services.BuildValidatedReloadScript("", uciCmds, "")

	out, err := runSSHScript(deviceID, script)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":  err.Error(),
			"output": out,
		})
		return
	}

	database.InsertAuditLog(username, "EDGE_NEXUS_DHCP_PUSH", "DEVICE", deviceID,
		fmt.Sprintf("Pushed DHCP config for %d interface(s)", len(payload.DHCP)), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"output": out,
	})
}

// ─── GET /api/devices/{id}/edge-firewall ─────────────────────────────────────

func GetEdgeFirewallHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	raw, err := runSSHCommand(deviceID, "uci export firewall 2>/dev/null")

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"device_id":       deviceID,
		"port_forwarding": []interface{}{},
		"raw":             raw,
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	json.NewEncoder(w).Encode(resp)
}

// ─── PUT /api/devices/{id}/edge-firewall ─────────────────────────────────────

func PutEdgeFirewallHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")
	username := GetUsernameFromReq(r)

	var payload struct {
		PortForward []services.PortForwardRule `json:"port_forwarding"`
	}
	if !readBody(w, r, &payload) {
		return
	}

	uciCmds := services.BuildFirewallUCI(payload.PortForward)
	script := services.BuildValidatedReloadScript("", "", uciCmds)

	out, err := runSSHScript(deviceID, script)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":  err.Error(),
			"output": out,
		})
		return
	}

	database.InsertAuditLog(username, "EDGE_NEXUS_FIREWALL_PUSH", "DEVICE", deviceID,
		fmt.Sprintf("Pushed %d port-forward rule(s)", len(payload.PortForward)), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"output": out,
	})
}
