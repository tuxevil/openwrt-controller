package handlers

import (
	"bytes"
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
func runSSHCommand(deviceID string, cmd string) (string, error) {
	if PrivateKey == nil {
		return "", fmt.Errorf("controller SSH key not configured")
	}

	var targetIP string
	err := database.DB.QueryRow("SELECT COALESCE(last_ip,'') FROM devices WHERE id = $1", deviceID).Scan(&targetIP)
	if err != nil || targetIP == "" {
		return "", fmt.Errorf("device IP not found")
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

	var out bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &out

	if err := sess.Run(cmd); err != nil {
		return out.String(), fmt.Errorf("remote command failed: %w", err)
	}
	return out.String(), nil
}

// runSSHScript uploads a shell script as stdin to `sh` and executes it.
func runSSHScript(deviceID string, script string) (string, error) {
	if PrivateKey == nil {
		return "", fmt.Errorf("controller SSH key not configured")
	}

	var targetIP string
	err := database.DB.QueryRow("SELECT COALESCE(last_ip,'') FROM devices WHERE id = $1", deviceID).Scan(&targetIP)
	if err != nil || targetIP == "" {
		return "", fmt.Errorf("device IP not found")
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

// readBody decodes JSON body into target and writes an error if it fails.
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
