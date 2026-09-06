package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
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
		schema, err := database.SafeTenantSchema(alias)
		if err != nil {
			continue
		}
		sqlSchema, err := database.SafeSQLSchemaIdent(schema)
		if err != nil {
			continue
		}
		var tenantIP sql.NullString
		var tenantLastSeen sql.NullTime
		err = database.DB.QueryRow(fmt.Sprintf("SELECT last_ip, last_seen_at FROM %s.devices WHERE id = $1", sqlSchema), deviceID).Scan(&tenantIP, &tenantLastSeen)
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

func getSSHSigner() (ssh.Signer, error) {
	if ks := orchestrator.GetKeyStore(); ks != nil {
		if s, err := ks.Get(); err == nil && s != nil {
			return s, nil
		}
	}
	if PrivateKey != nil {
		return PrivateKey, nil
	}
	return nil, fmt.Errorf("controller SSH key not configured")
}

func runSSHCommand(deviceID string, cmd string) (string, error) {
	signer, err := getSSHSigner()
	if err != nil {
		return "", err
	}

	targetIP, _, err := getDeviceIPAndSchema(deviceID)
	if err != nil || targetIP == "" {
		return "", fmt.Errorf("device IP not found: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
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

	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return string(out), fmt.Errorf("remote command failed: %w", err)
	}
	return string(out), nil
}

func runSSHScript(deviceID string, script string) (string, error) {
	signer, err := getSSHSigner()
	if err != nil {
		return "", err
	}

	targetIP, _, err := getDeviceIPAndSchema(deviceID)
	if err != nil || targetIP == "" {
		return "", fmt.Errorf("device IP not found: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
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

	sess.Stdin = strings.NewReader(script)
	out, err := sess.CombinedOutput("sh -s")
	if err != nil {
		return string(out), fmt.Errorf("script execution failed: %w", err)
	}
	return string(out), nil
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
		Interfaces   []services.NetworkInterface `json:"interfaces"`
		Confirm      bool                        `json:"confirm"`
		HealthChecks []string                    `json:"health_checks"`
	}
	if !readBody(w, r, &payload) {
		return
	}
	if err := services.ValidateNetworkInterfaces(payload.Interfaces); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uciCmds := services.BuildNetworkCommands(payload.Interfaces)
	if len(uciCmds) == 0 {
		http.Error(w, `{"error":"network interface payload produced no commands"}`, http.StatusBadRequest)
		return
	}
	if !payload.Confirm {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "preview",
			"device_id":     deviceID,
			"commands":      services.PreviewCommands(uciCmds),
			"next_step":     "repeat with confirm=true and health_checks to queue this operation",
			"health_checks": payload.HealthChecks,
		})
		return
	}

	plan, err := services.NewDeviceOperationPlan("network", uciCmds, payload.HealthChecks, true)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	if err := services.CreateBackup(context.Background(), schema, deviceID); err != nil {
		http.Error(w, `{"error":"pre-change backup failed; operation was not queued"}`, http.StatusServiceUnavailable)
		return
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		http.Error(w, `{"error":"could not serialize operation plan"}`, http.StatusInternalServerError)
		return
	}
	queuedGeneration, err := database.QueueDeviceOperation(r.Context(), schema, deviceID, planJSON)
	if err != nil {
		database.InsertAuditLog(username, "EDGE_NEXUS_NETWORK_QUEUE_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_queued", "error": err.Error()})
		return
	}
	if err := database.CommitRequestTx(r.Context()); err != nil {
		database.InsertAuditLog(username, "EDGE_NEXUS_NETWORK_QUEUE_COMMIT_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		http.Error(w, `{"error":"operation queue commit failed"}`, http.StatusInternalServerError)
		return
	}

	plan.Generation = queuedGeneration
	database.InsertAuditLog(username, "EDGE_NEXUS_NETWORK_QUEUED", "DEVICE", deviceID,
		fmt.Sprintf("Pushed %d interface(s)", len(payload.Interfaces)), r.RemoteAddr)

	writeQueuedDeviceOperationResponse(w, plan)
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
		DHCP         []services.DHCPInterface `json:"dhcp"`
		Confirm      bool                     `json:"confirm"`
		HealthChecks []string                 `json:"health_checks"`
	}
	if !readBody(w, r, &payload) {
		return
	}
	if err := services.ValidateDHCPInterfaces(payload.DHCP); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uciCmds := services.BuildDHCPCommands(payload.DHCP)
	if len(uciCmds) == 0 {
		http.Error(w, `{"error":"DHCP payload produced no commands"}`, http.StatusBadRequest)
		return
	}
	if !payload.Confirm {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "preview",
			"device_id":     deviceID,
			"commands":      services.PreviewCommands(uciCmds),
			"next_step":     "repeat with confirm=true to queue this operation",
			"health_checks": payload.HealthChecks,
		})
		return
	}
	plan, err := services.NewDeviceOperationPlan("dhcp", uciCmds, payload.HealthChecks, true)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	if err := services.CreateBackup(context.Background(), schema, deviceID); err != nil {
		http.Error(w, `{"error":"pre-change backup failed; operation was not queued"}`, http.StatusServiceUnavailable)
		return
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		http.Error(w, `{"error":"could not serialize operation plan"}`, http.StatusInternalServerError)
		return
	}
	queuedGeneration, err := database.QueueDeviceOperation(r.Context(), schema, deviceID, planJSON)
	if err != nil {
		database.InsertAuditLog(username, "EDGE_NEXUS_DHCP_QUEUE_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_queued", "error": err.Error()})
		return
	}
	if err := database.CommitRequestTx(r.Context()); err != nil {
		database.InsertAuditLog(username, "EDGE_NEXUS_DHCP_QUEUE_COMMIT_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		http.Error(w, `{"error":"operation queue commit failed"}`, http.StatusInternalServerError)
		return
	}

	plan.Generation = queuedGeneration
	database.InsertAuditLog(username, "EDGE_NEXUS_DHCP_QUEUED", "DEVICE", deviceID,
		fmt.Sprintf("Pushed DHCP config for %d interface(s)", len(payload.DHCP)), r.RemoteAddr)

	writeQueuedDeviceOperationResponse(w, plan)
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
		PortForward  []services.PortForwardRule `json:"port_forwarding"`
		Confirm      bool                       `json:"confirm"`
		HealthChecks []string                   `json:"health_checks"`
	}
	if !readBody(w, r, &payload) {
		return
	}
	if err := services.ValidatePortForwardRules(payload.PortForward); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	uciCmds := services.BuildFirewallCommands(payload.PortForward)
	if !payload.Confirm {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "preview",
			"device_id":     deviceID,
			"commands":      services.PreviewCommands(uciCmds),
			"next_step":     "repeat with confirm=true to queue this operation",
			"health_checks": payload.HealthChecks,
		})
		return
	}
	plan, err := services.NewDeviceOperationPlan("firewall", uciCmds, payload.HealthChecks, true)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	if err := services.CreateBackup(context.Background(), schema, deviceID); err != nil {
		http.Error(w, `{"error":"pre-change backup failed; operation was not queued"}`, http.StatusServiceUnavailable)
		return
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		http.Error(w, `{"error":"could not serialize operation plan"}`, http.StatusInternalServerError)
		return
	}
	queuedGeneration, err := database.QueueDeviceOperation(r.Context(), schema, deviceID, planJSON)
	if err != nil {
		database.InsertAuditLog(username, "EDGE_NEXUS_FIREWALL_QUEUE_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_queued", "error": err.Error()})
		return
	}
	if err := database.CommitRequestTx(r.Context()); err != nil {
		database.InsertAuditLog(username, "EDGE_NEXUS_FIREWALL_QUEUE_COMMIT_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		http.Error(w, `{"error":"operation queue commit failed"}`, http.StatusInternalServerError)
		return
	}

	plan.Generation = queuedGeneration
	database.InsertAuditLog(username, "EDGE_NEXUS_FIREWALL_QUEUED", "DEVICE", deviceID,
		fmt.Sprintf("Pushed %d port-forward rule(s)", len(payload.PortForward)), r.RemoteAddr)

	writeQueuedDeviceOperationResponse(w, plan)
}
