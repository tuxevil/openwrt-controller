package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
	"openwrt-controller/internal/services"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func getDeviceIPForSite(ctx context.Context, schema, siteID, deviceID string) (string, error) {
	sqlSchema, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return "", err
	}
	var ip sql.NullString
	err = database.DB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT last_ip FROM %s.devices WHERE id = $1 AND site_id = $2", sqlSchema,
	), deviceID, siteID).Scan(&ip)
	if err != nil {
		return "", fmt.Errorf("device not found in site: %w", err)
	}
	if !ip.Valid || ip.String == "" {
		return "", fmt.Errorf("device IP not found")
	}
	return ip.String, nil
}

func getDeviceIPForTenant(ctx context.Context, schema, deviceID string) (string, error) {
	sqlSchema, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return "", err
	}
	var ip sql.NullString
	err = database.DB.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT last_ip FROM %s.devices WHERE id = $1", sqlSchema,
	), deviceID).Scan(&ip)
	if err != nil {
		return "", fmt.Errorf("device not found in tenant: %w", err)
	}
	if !ip.Valid || ip.String == "" {
		return "", fmt.Errorf("device IP not found")
	}
	return ip.String, nil
}

func runSSHCommandForSite(ctx context.Context, schema, siteID, deviceID, cmd string) (string, error) {
	targetIP, err := getDeviceIPForSite(ctx, schema, siteID, deviceID)
	if err != nil {
		return "", err
	}
	return runSSHTransport(ctx, targetIP, cmd, nil, 30*time.Second)
}

func runSSHScriptForSite(ctx context.Context, schema, siteID, deviceID, script string) (string, error) {
	targetIP, err := getDeviceIPForSite(ctx, schema, siteID, deviceID)
	if err != nil {
		return "", err
	}
	return runSSHTransport(ctx, targetIP, "sh -s", strings.NewReader(script), 60*time.Second)
}

func runSSHCommandForRequest(r *http.Request, deviceID, cmd string) (string, error) {
	schema, err := getTenantSchema(r)
	if err != nil {
		return "", err
	}
	targetIP, err := getDeviceIPForTenant(r.Context(), schema, deviceID)
	if err != nil {
		return "", err
	}
	return runSSHTransport(r.Context(), targetIP, cmd, nil, 30*time.Second)
}

func runSSHScriptForRequest(r *http.Request, deviceID, script string) (string, error) {
	schema, err := getTenantSchema(r)
	if err != nil {
		return "", err
	}
	targetIP, err := getDeviceIPForTenant(r.Context(), schema, deviceID)
	if err != nil {
		return "", err
	}
	return runSSHTransport(r.Context(), targetIP, "sh -s", strings.NewReader(script), 60*time.Second)
}

type sshExitStatusError struct {
	status int
	err    error
}

func (e *sshExitStatusError) Error() string { return e.err.Error() }
func (e *sshExitStatusError) Unwrap() error { return e.err }

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func runSSHTransport(ctx context.Context, targetIP, command string, stdin io.Reader, timeout time.Duration) (string, error) {
	signer, err := getSSHSigner()
	if err != nil {
		return "", err
	}
	commandCtx, commandCancel := context.WithTimeout(ctx, timeout)
	defer commandCancel()

	target := net.JoinHostPort(targetIP, "22")
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
		Timeout:         timeout,
	}
	netConn, err := (&net.Dialer{}).DialContext(commandCtx, "tcp", target)
	if err != nil {
		return "", fmt.Errorf("SSH dial: %w", err)
	}
	if deadline, ok := commandCtx.Deadline(); ok {
		_ = netConn.SetDeadline(deadline)
	} else {
		_ = netConn.SetDeadline(time.Now().Add(timeout))
	}
	conn, channels, requests, err := ssh.NewClientConn(netConn, target, cfg)
	if err != nil {
		_ = netConn.Close()
		return "", fmt.Errorf("SSH handshake: %w", err)
	}
	_ = netConn.SetDeadline(time.Time{})
	client := ssh.NewClient(conn, channels, requests)
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session: %w", err)
	}
	defer sess.Close()
	if stdin != nil {
		sess.Stdin = stdin
	}
	var output synchronizedBuffer
	sess.Stdout = &output
	sess.Stderr = &output
	if err := sess.Start(command); err != nil {
		return output.String(), fmt.Errorf("SSH command start: %w", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- sess.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				return output.String(), &sshExitStatusError{status: exitErr.ExitStatus(), err: err}
			}
			return output.String(), fmt.Errorf("remote command failed: %w", err)
		}
		return output.String(), nil
	case <-commandCtx.Done():
		_ = sess.Close()
		_ = client.Close()
		<-wait
		return output.String(), commandCtx.Err()
	}
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

	raw, err := runSSHCommandForRequest(r, deviceID, "uci export network 2>/dev/null")
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

// GetEdgeDHCPHandler reads the device DHCP configuration over SSH.
func GetEdgeDHCPHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	dhcpRaw, dhcpErr := runSSHCommandForRequest(r, deviceID, "uci export dhcp 2>/dev/null")

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

// PutEdgeDHCPHandler validates and queues a device DHCP configuration change.
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

// GetEdgeFirewallHandler reads the device firewall configuration over SSH.
func GetEdgeFirewallHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	raw, err := runSSHCommandForRequest(r, deviceID, "uci export firewall 2>/dev/null")

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

// PutEdgeFirewallHandler validates and queues a device firewall change.
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
