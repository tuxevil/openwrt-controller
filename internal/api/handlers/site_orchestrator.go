package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

const fleetSyncMaxConcurrency = 4
const fleetSyncSequential = true
const maxRolloutDiagnosticBytes = 4096

var (
	errRolloutDraftStale     = errors.New("rollout draft observed state is stale")
	errRolloutDraftTransport = errors.New("rollout draft observed state could not be checked")
	errRolloutDeviceConflict = errors.New("rollout device state is no longer current")
	errRolloutClaimLost      = errors.New("rollout claim is no longer current")
)

func auditRolloutEvent(r *http.Request, username, action, siteID, payload string) error {
	if err := database.InsertAuditLog(username, action, "SITE", siteID, payload, r.RemoteAddr); err != nil {
		log.Printf("[SITE_ORCHESTRATOR][WARN] failed to write audit event %s for site %s: %v", action, siteID, err)
		return err
	}
	return nil
}

func releaseRolloutDraftForRetry(r *http.Request, schema, siteID, rolloutID, claimToken string) error {
	releaseCtx, releaseCancel := rolloutPersistenceContext(r)
	defer releaseCancel()
	return database.ReleaseRolloutDraft(releaseCtx, schema, siteID, rolloutID, claimToken)
}

func handleRolloutDraftValidationFailure(w http.ResponseWriter, r *http.Request, schema, siteID, rolloutID, claimToken, username string, validationErr error) {
	if errors.Is(validationErr, errRolloutDraftTransport) {
		if releaseErr := releaseRolloutDraftForRetry(r, schema, siteID, rolloutID, claimToken); releaseErr != nil {
			log.Printf("[SITE_ORCHESTRATOR][WARN] failed to release rollout draft %s: %v", rolloutID, releaseErr)
		}
		if auditErr := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_RETRYABLE_FAILURE", siteID,
			fmt.Sprintf("Rollout %s could not validate observed state: %v", rolloutID, validationErr)); auditErr != nil {
			http.Error(w, `{"error":"could not record rollout audit event"}`, http.StatusServiceUnavailable)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":%q}`, validationErr.Error()), http.StatusBadGateway)
		return
	}
	staleCtx, staleCancel := rolloutPersistenceContext(r)
	if err := database.MarkRolloutDraftStale(staleCtx, schema, siteID, rolloutID, claimToken); err != nil {
		log.Printf("[SITE_ORCHESTRATOR][WARN] failed to mark rollout draft %s stale: %v", rolloutID, err)
	}
	staleCancel()
	if auditErr := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_STALE", siteID,
		fmt.Sprintf("Rollout %s rejected: %v", rolloutID, validationErr)); auditErr != nil {
		http.Error(w, `{"error":"could not record rollout audit event"}`, http.StatusServiceUnavailable)
		return
	}
	http.Error(w, fmt.Sprintf(`{"error":%q}`, validationErr.Error()), http.StatusConflict)
}

type fleetSyncResult struct {
	DeviceID         string `json:"device_id"`
	Hostname         string `json:"hostname"`
	Role             string `json:"role"`
	Status           string `json:"status"`
	ChangeSetState   string `json:"change_set_state,omitempty"`
	Output           string `json:"output"`
	Error            string `json:"error,omitempty"`
	CmdCount         int    `json:"cmd_count"`
	ChangeSetID      string `json:"change_set_id,omitempty"`
	PlanHash         string `json:"plan_hash,omitempty"`
	DeviceGeneration int64  `json:"device_generation,omitempty"`
}

type rolloutDraft struct {
	SiteID         string               `json:"site_id"`
	TargetDeviceID string               `json:"target_device_id,omitempty"`
	Namespace      string               `json:"namespace,omitempty"`
	HealthChecks   []string             `json:"health_checks"`
	Devices        []rolloutDraftDevice `json:"devices"`
}

type rolloutDraftDevice struct {
	DeviceID        string                `json:"device_id"`
	Hostname        string                `json:"hostname"`
	Role            string                `json:"role"`
	Commands        []services.UciCommand `json:"commands"`
	PreviewCommands []string              `json:"preview_commands"`
	Scripts         map[string]string     `json:"scripts"`
	ObservedState   map[string]string     `json:"observed_state"`
}

type rolloutDevicePreview struct {
	DeviceID      string            `json:"device_id"`
	Hostname      string            `json:"hostname"`
	Role          string            `json:"role"`
	Commands      []string          `json:"commands"`
	Count         int               `json:"count"`
	Scripts       map[string]string `json:"scripts"`
	ObservedState map[string]string `json:"observed_state"`
}

// ─── SITE_ORCHESTRATOR Handlers ──────────────────────────────────────────────

// GetSiteConfigHandler returns the desired-state template for a site.
func GetSiteConfigHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		// No config yet — return defaults
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services.SiteConfig{
			SiteID:               siteID,
			GlobalEncryption:     "psk2",
			LanIPAddr:            "192.168.1.1",
			LanNetmask:           "255.255.255.0",
			DHCPStart:            100,
			DHCPLimit:            150,
			DHCPLeasetime:        "12h",
			DNSPrimary:           "9.9.9.9",
			DNSSecondary:         "1.1.1.1",
			Timezone:             "UTC",
			HostnamePrefix:       "nerve",
			SecureTunnelEnabled:  true,
			FirewallSynFlood:     true,
			FirewallDropInvalid:  true,
			DropbearPort:         22,
			DropbearPasswordAuth: true,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sc)
}

// PutSiteConfigHandler saves the desired-state template for a site.
func PutSiteConfigHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	username := GetUsernameFromReq(r)

	// DTO: uses json.RawMessage for JSONB blobs so Go does not attempt
	// base64-decoding (which happens with []byte fields).
	var dto struct {
		EnableGlobalSSID     bool            `json:"enable_global_ssid"`
		GlobalSSID           string          `json:"global_ssid"`
		GlobalWPAKey         string          `json:"global_wpa_key"`
		GlobalEncryption     string          `json:"global_encryption"`
		LanIPAddr            string          `json:"lan_ipaddr"`
		LanNetmask           string          `json:"lan_netmask"`
		DHCPStart            int             `json:"dhcp_start"`
		DHCPLimit            int             `json:"dhcp_limit"`
		DHCPLeasetime        string          `json:"dhcp_leasetime"`
		DNSPrimary           string          `json:"dns_primary"`
		DNSSecondary         string          `json:"dns_secondary"`
		Timezone             string          `json:"timezone"`
		HostnamePrefix       string          `json:"hostname_prefix"`
		FirewallSynFlood     bool            `json:"firewall_syn_flood"`
		FirewallDropInvalid  bool            `json:"firewall_drop_invalid"`
		DropbearPort         int             `json:"dropbear_port"`
		DropbearPasswordAuth bool            `json:"dropbear_password_auth"`
		DHCPReservations     json.RawMessage `json:"dhcp_reservations"`
		PortForwardingRules  json.RawMessage `json:"port_forwarding_rules"`
		ThreatShieldEnabled  bool            `json:"threat_shield_enabled"`
		SQMCakeEnabled       bool            `json:"sqm_cake_enabled"`
		SqmDownload          int             `json:"sqm_download"`
		SqmUpload            int             `json:"sqm_upload"`
		DPIEnabled           bool            `json:"dpi_enabled"`
		SecureTunnelEnabled  bool            `json:"secure_tunnel_enabled"`
		TailscaleEnabled     bool            `json:"tailscale_enabled"`
		TailscaleAuthKey     string          `json:"tailscale_auth_key"`
		AllowPublicSurveys   bool            `json:"allow_public_surveys"`
		BenchmarkBaseline    json.RawMessage `json:"benchmark_baseline"`
		TopologyMetadata     json.RawMessage `json:"topology_metadata"`
		HealthChecks         json.RawMessage `json:"health_checks"`
	}
	if !readBody(w, r, &dto) {
		return
	}

	// Normalize JSONB blobs — default to empty array if null/missing
	dhcpRes := []byte(dto.DHCPReservations)
	if len(dhcpRes) == 0 || string(dhcpRes) == "null" {
		dhcpRes = []byte("[]")
	}
	pfRules := []byte(dto.PortForwardingRules)
	if len(pfRules) == 0 || string(pfRules) == "null" {
		pfRules = []byte("[]")
	}

	if dto.EnableGlobalSSID && dto.GlobalSSID != "" {
		schema, schemaErr := getTenantSchema(r)
		if schemaErr != nil {
			http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
			return
		}
		conflicts, conflictErr := siteWLANPolicyConflicts(r.Context(), schema, siteID, dto.GlobalSSID, dto.GlobalEncryption, dto.GlobalWPAKey)
		if conflictErr != nil {
			http.Error(w, `{"error":"could not validate canonical WLAN policy"}`, http.StatusInternalServerError)
			return
		}
		if len(conflicts) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":     "site WLAN fields conflict with canonical WLAN rows",
				"conflicts": conflicts,
				"next_step": "update the WLAN row first; site global_* fields are compatibility fields",
			})
			return
		}
	}

	sc := services.SiteConfig{
		SQMCakeEnabled:       dto.SQMCakeEnabled,
		SqmDownload:          dto.SqmDownload,
		SqmUpload:            dto.SqmUpload,
		DPIEnabled:           dto.DPIEnabled,
		SecureTunnelEnabled:  dto.SecureTunnelEnabled,
		TailscaleEnabled:     dto.TailscaleEnabled,
		TailscaleAuthKey:     dto.TailscaleAuthKey,
		SiteID:               siteID,
		EnableGlobalSSID:     dto.EnableGlobalSSID,
		GlobalSSID:           dto.GlobalSSID,
		GlobalWPAKey:         dto.GlobalWPAKey,
		GlobalEncryption:     dto.GlobalEncryption,
		LanIPAddr:            dto.LanIPAddr,
		LanNetmask:           dto.LanNetmask,
		DHCPStart:            dto.DHCPStart,
		DHCPLimit:            dto.DHCPLimit,
		DHCPLeasetime:        dto.DHCPLeasetime,
		DNSPrimary:           dto.DNSPrimary,
		DNSSecondary:         dto.DNSSecondary,
		Timezone:             dto.Timezone,
		HostnamePrefix:       dto.HostnamePrefix,
		FirewallSynFlood:     dto.FirewallSynFlood,
		FirewallDropInvalid:  dto.FirewallDropInvalid,
		DropbearPort:         dto.DropbearPort,
		DropbearPasswordAuth: dto.DropbearPasswordAuth,
		DHCPReservations:     dhcpRes,
		PortForwardingRules:  pfRules,
		ThreatShieldEnabled:  dto.ThreatShieldEnabled,
		AllowPublicSurveys:   dto.AllowPublicSurveys,
		BenchmarkBaseline:    dto.BenchmarkBaseline,
		TopologyMetadata:     dto.TopologyMetadata,
		HealthChecks:         dto.HealthChecks,
	}

	if err := services.UpsertSiteConfig(r.Context(), sc); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	database.InsertAuditLog(username, "SITE_ORCHESTRATOR_CONFIG_SAVE", "SITE", siteID,
		fmt.Sprintf("Updated site desired-state template (SSID: %s)", sc.GlobalSSID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}

func siteWLANPolicyConflicts(ctx context.Context, schema, siteID, siteSSID, siteEncryption, sitePassword string) ([]string, error) {
	rows, err := database.Tx(ctx).Query(`
		SELECT security, COALESCE(password, '')
		FROM `+schema+`.wlans
		WHERE site_id = $1 AND ssid = $2 AND enabled = true
	`, siteID, siteSSID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []string
	seen := make(map[string]struct{})
	for rows.Next() {
		var security, password string
		if err := rows.Scan(&security, &password); err != nil {
			return nil, err
		}
		resolution := services.ResolveCanonicalWLAN(siteSSID, siteEncryption, sitePassword, siteSSID, security, password)
		for _, field := range resolution.Conflicts {
			if _, ok := seen[field]; ok {
				continue
			}
			seen[field] = struct{}{}
			conflicts = append(conflicts, field)
		}
	}
	return conflicts, rows.Err()
}

// GetSiteDeviceRolesHandler returns devices with their assigned roles.
func GetSiteDeviceRolesHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	devs, err := services.GetSiteDevicesWithRoles(r.Context(), siteID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": devs,
	})
}

func selectRolloutDevices(devs []services.DeviceRoleInfo, targetID string) ([]services.DeviceRoleInfo, error) {
	if targetID == "" {
		return devs, nil
	}
	for _, dev := range devs {
		if dev.DeviceID == targetID {
			return []services.DeviceRoleInfo{dev}, nil
		}
	}
	return nil, fmt.Errorf("target device not found in site")
}

func rolloutHealthTargets(sc services.SiteConfig) ([]string, error) {
	var targets []string
	if len(sc.HealthChecks) > 0 {
		if err := json.Unmarshal(sc.HealthChecks, &targets); err != nil {
			return nil, fmt.Errorf("invalid health_checks configuration")
		}
	}
	validated, err := services.ValidateHealthTargets(targets)
	if err != nil {
		return nil, err
	}
	if len(validated) == 0 {
		// Keep the effective default in the immutable plan because the script
		// builder uses the same target when no site targets are configured.
		return []string{"1.1.1.1"}, nil
	}
	return validated, nil
}

func buildRolloutDraft(siteID, targetDeviceID string, healthChecks []string, results []services.RenderResult, observedState map[string]map[string]string) (rolloutDraft, error) {
	draft := rolloutDraft{
		SiteID:         siteID,
		TargetDeviceID: targetDeviceID,
		HealthChecks:   append([]string{}, healthChecks...),
		Devices:        make([]rolloutDraftDevice, 0, len(results)),
	}
	for _, phase := range sequentialFleetRolloutPhases(results) {
		for _, index := range phase {
			result := results[index]
			state := make(map[string]string)
			for config, hash := range observedState[result.DeviceID] {
				state[config] = hash
			}
			if len(result.Commands) > 0 && len(state) == 0 {
				return rolloutDraft{}, fmt.Errorf("missing observed state for device %s", result.DeviceID)
			}
			scripts := make(map[string]string)
			for config, commands := range groupCommandsByConfig(result.Commands) {
				script, err := buildGuardedRolloutScript(config, commands, healthChecks, state[config])
				if err != nil {
					return rolloutDraft{}, fmt.Errorf("could not build rollout script for device %s: %w", result.DeviceID, err)
				}
				scripts[config] = script
			}
			draft.Devices = append(draft.Devices, rolloutDraftDevice{
				DeviceID:        result.DeviceID,
				Hostname:        result.Hostname,
				Role:            result.Role,
				Commands:        append([]services.UciCommand{}, result.Commands...),
				PreviewCommands: append([]string{}, services.PreviewCommands(result.Commands)...),
				Scripts:         scripts,
				ObservedState:   state,
			})
		}
	}
	return draft, nil
}

func filterRolloutResultsByNamespace(results []services.RenderResult, namespace string) ([]services.RenderResult, error) {
	if namespace == "" {
		return results, nil
	}
	namespaces, err := requestedChangeSetNamespaces(namespace)
	if err != nil {
		return nil, err
	}
	filtered := make([]services.RenderResult, len(results))
	for index, result := range results {
		commands := result.Commands
		result.Commands = make([]services.UciCommand, 0, len(result.Commands))
		for _, command := range commands {
			if namespaces[command.Config] {
				result.Commands = append(result.Commands, command)
			}
		}
		filtered[index] = result
	}
	return filtered, nil
}

func dropEmptyRolloutResults(results []services.RenderResult, observedState map[string]map[string]string) ([]services.RenderResult, map[string]map[string]string) {
	filtered := make([]services.RenderResult, 0, len(results))
	filteredState := make(map[string]map[string]string, len(observedState))
	for _, result := range results {
		if len(result.Commands) == 0 {
			continue
		}
		filtered = append(filtered, result)
		if state, ok := observedState[result.DeviceID]; ok {
			filteredState[result.DeviceID] = state
		}
	}
	return filtered, filteredState
}

func requestedChangeSetNamespaces(raw string) (map[string]bool, error) {
	allowed := map[string]bool{"system": true, "dhcp": true, "firewall": true, "dropbear": true, "sqm": true}
	namespaces := make(map[string]bool)
	for _, namespace := range strings.Split(raw, ",") {
		namespace = strings.TrimSpace(namespace)
		if namespace == "" || !allowed[namespace] {
			return nil, fmt.Errorf("namespace %s is not supported by the safe changeset slice", namespace)
		}
		namespaces[namespace] = true
	}
	return namespaces, nil
}

func rolloutPlanHash(draft rolloutDraft) string {
	encoded, _ := json.Marshal(draft)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func decodeRolloutDraft(record database.RolloutDraftRecord) (rolloutDraft, error) {
	var draft rolloutDraft
	if err := json.Unmarshal(record.Plan, &draft); err != nil {
		return rolloutDraft{}, fmt.Errorf("invalid rollout draft: %w", err)
	}
	if draft.SiteID != record.SiteID || rolloutPlanHash(draft) != record.PlanHash {
		return rolloutDraft{}, fmt.Errorf("rollout draft identity does not match its stored plan")
	}
	if draft.Namespace != "" {
		if _, err := requestedChangeSetNamespaces(draft.Namespace); err != nil {
			return rolloutDraft{}, fmt.Errorf("rollout draft namespace is unsupported")
		}
	}
	return draft, nil
}

func rolloutResultsFromDraft(draft rolloutDraft) []services.RenderResult {
	results := make([]services.RenderResult, 0, len(draft.Devices))
	for _, device := range draft.Devices {
		results = append(results, services.RenderResult{
			DeviceID: device.DeviceID,
			Hostname: device.Hostname,
			Role:     device.Role,
			Commands: append([]services.UciCommand{}, device.Commands...),
		})
	}
	return results
}

func buildSingleDeviceChangeSet(rolloutID string, draft rolloutDraft) (services.DeviceChangeSet, error) {
	if len(draft.Devices) != 1 {
		return services.DeviceChangeSet{}, fmt.Errorf("a changeset slice requires exactly one target device")
	}
	return buildDeviceChangeSet(rolloutID, draft, draft.Devices[0])
}

func buildDeviceChangeSet(rolloutID string, draft rolloutDraft, device rolloutDraftDevice) (services.DeviceChangeSet, error) {
	namespaces, err := requestedChangeSetNamespaces(draft.Namespace)
	if err != nil {
		return services.DeviceChangeSet{}, err
	}
	if len(device.Commands) == 0 || len(namespaces) == 0 {
		return services.DeviceChangeSet{}, fmt.Errorf("device %s has no typed commands", device.DeviceID)
	}
	commandsByNamespace := make(map[string][]services.UciCommand)
	for _, command := range device.Commands {
		if !namespaces[command.Config] {
			return services.DeviceChangeSet{}, fmt.Errorf("namespace %s is not selected for the safe changeset", command.Config)
		}
		commandsByNamespace[command.Config] = append(commandsByNamespace[command.Config], command)
	}
	orderedNamespaces := []string{"system", "dhcp", "firewall", "dropbear", "sqm"}
	operations := make([]services.DeviceChangeOperation, 0, len(commandsByNamespace))
	for _, namespace := range orderedNamespaces {
		commands := commandsByNamespace[namespace]
		if len(commands) == 0 {
			continue
		}
		observedHash := device.ObservedState[namespace]
		if observedHash == "" {
			return services.DeviceChangeSet{}, fmt.Errorf("missing observed %s state for device %s", namespace, device.DeviceID)
		}
		operations = append(operations, services.DeviceChangeOperation{Config: namespace, Commands: commands, ObservedStateHash: observedHash})
	}
	changeSet, err := services.NewDeviceChangeSetForRolloutOperations(
		rolloutID,
		device.DeviceID,
		operations,
		draft.HealthChecks,
		services.ConfirmationLocalAuto,
	)
	if err != nil {
		return services.DeviceChangeSet{}, fmt.Errorf("could not build device changeset: %w", err)
	}
	return changeSet, nil
}

func changeSetCommandCount(changeSet services.DeviceChangeSet) int {
	count := 0
	for _, operation := range changeSet.Operations {
		count += len(operation.Commands)
	}
	return count
}

func buildFleetDeviceChangeSets(rolloutID string, draft rolloutDraft) ([]services.DeviceChangeSet, error) {
	if len(draft.Devices) == 0 {
		return nil, fmt.Errorf("a changeset rollout requires at least one target device")
	}
	changeSets := make([]services.DeviceChangeSet, 0, len(draft.Devices))
	seenDevices := make(map[string]bool, len(draft.Devices))
	for _, device := range draft.Devices {
		deviceKey := strings.ToLower(device.DeviceID)
		if seenDevices[deviceKey] {
			return nil, fmt.Errorf("duplicate target device %s", device.DeviceID)
		}
		seenDevices[deviceKey] = true
		changeSet, err := buildDeviceChangeSet(rolloutID, draft, device)
		if err != nil {
			return nil, err
		}
		changeSets = append(changeSets, changeSet)
	}
	return changeSets, nil
}

func queuedChangeSetResult(record database.RolloutDraftRecord, changeSet services.DeviceChangeSet) (fleetSyncResult, bool) {
	return existingChangeSetResult(record, changeSet, "QUEUED")
}

func writeQueuedChangeSetResponse(w http.ResponseWriter, rolloutID string, record database.RolloutDraftRecord, changeSet services.DeviceChangeSet, result fleetSyncResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "queued",
		"rollout_id":        rolloutID,
		"generation":        record.Generation,
		"target_device_id":  changeSet.DeviceID,
		"change_set_id":     changeSet.ChangeSetID,
		"plan_hash":         changeSet.PlanHash,
		"device_generation": result.DeviceGeneration,
	})
}

func queueFleetDeviceChangeSets(r *http.Request, schema, siteID, rolloutID, username string, record database.RolloutDraftRecord, draft rolloutDraft) (map[string]interface{}, error) {
	changeSets, err := buildFleetDeviceChangeSets(rolloutID, draft)
	if err != nil {
		return nil, err
	}
	queuedResults := make([]fleetSyncResult, 0, len(changeSets))
	if record.Status == "QUEUED" || record.Status == "completed" || record.Status == "failed" {
		for _, changeSet := range changeSets {
			requiredStatus := ""
			if record.Status == "completed" || record.Status == "failed" {
				requiredStatus = "TERMINAL"
			}
			result, ok := existingChangeSetResult(record, changeSet, requiredStatus)
			if !ok {
				return nil, fmt.Errorf("rollout is already queued with a different changeset for device %s", changeSet.DeviceID)
			}
			queuedResults = append(queuedResults, result)
		}
	} else {
		items := make([]database.DeviceChangeSetQueueItem, 0, len(changeSets))
		for index, changeSet := range changeSets {
			changeSetRaw, err := json.Marshal(changeSet)
			if err != nil {
				return nil, fmt.Errorf("could not encode changeset for device %s: %w", changeSet.DeviceID, err)
			}
			device := draft.Devices[index]
			items = append(items, database.DeviceChangeSetQueueItem{DeviceRole: device.Role, ChangeSet: changeSetRaw})
			queuedResults = append(queuedResults, fleetSyncResult{
				DeviceID:    changeSet.DeviceID,
				Hostname:    device.Hostname,
				Role:        device.Role,
				Status:      "QUEUED",
				Output:      "device agent will apply and report the durable changeset result",
				CmdCount:    changeSetCommandCount(changeSet),
				ChangeSetID: changeSet.ChangeSetID,
				PlanHash:    changeSet.PlanHash,
			})
		}
		queuedRaw, err := json.Marshal(queuedResults)
		if err != nil {
			return nil, fmt.Errorf("could not encode queued results: %w", err)
		}
		queueCtx, queueCancel := rolloutPersistenceContext(r)
		generations, err := database.ClaimAndQueueDeviceChangeSets(queueCtx, schema, siteID, rolloutID, items, queuedRaw, username, r.RemoteAddr)
		queueCancel()
		if err != nil {
			return nil, err
		}
		for index := range queuedResults {
			queuedResults[index].DeviceGeneration = generations[queuedResults[index].DeviceID]
		}
	}
	responseStatus := record.Status
	if responseStatus == "DRAFT" {
		responseStatus = "QUEUED"
	}
	return map[string]interface{}{
		"status":     strings.ToLower(responseStatus),
		"rollout_id": rolloutID,
		"generation": record.Generation,
		"target_device_ids": func() []string {
			ids := make([]string, 0, len(changeSets))
			for _, changeSet := range changeSets {
				ids = append(ids, changeSet.DeviceID)
			}
			return ids
		}(),
		"changesets": queuedResults,
	}, nil
}

func existingChangeSetResult(record database.RolloutDraftRecord, changeSet services.DeviceChangeSet, requiredStatus string) (fleetSyncResult, bool) {
	var results []fleetSyncResult
	if err := json.Unmarshal(record.Results, &results); err != nil {
		return fleetSyncResult{}, false
	}
	for _, result := range results {
		if strings.EqualFold(result.DeviceID, changeSet.DeviceID) && result.ChangeSetID == changeSet.ChangeSetID && result.PlanHash == changeSet.PlanHash && result.DeviceGeneration > 0 {
			if requiredStatus == "QUEUED" && !strings.EqualFold(result.Status, "QUEUED") {
				continue
			}
			if requiredStatus == "TERMINAL" && strings.EqualFold(result.Status, "QUEUED") {
				continue
			}
			return result, true
		}
	}
	return fleetSyncResult{}, false
}

func rejectUnexecutableRolloutDraft(r *http.Request, schema, siteID, rolloutID, username string, reason error) {
	ctx, cancel := rolloutPersistenceContext(r)
	defer cancel()
	if err := database.RejectRolloutDraft(ctx, schema, siteID, rolloutID, reason.Error()); err != nil {
		log.Printf("[SITE_ORCHESTRATOR][WARN] failed to durably reject rollout %s: %v", rolloutID, err)
	}
	if err := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_REJECTED", siteID,
		fmt.Sprintf("Rejected rollout %s: %v", rolloutID, reason)); err != nil {
		log.Printf("[SITE_ORCHESTRATOR][WARN] failed to audit rejected rollout %s: %v", rolloutID, err)
	}
}

func verifyRolloutDraftObservedState(ctx context.Context, schema, siteID string, draft rolloutDraft) error {
	for _, device := range draft.Devices {
		configs := make(map[string][]services.UciCommand, len(device.ObservedState))
		for config := range device.ObservedState {
			configs[config] = nil
		}
		for _, config := range sortedConfigNames(configs) {
			expected := device.ObservedState[config]
			if expected == "" {
				return fmt.Errorf("%w: no observed hash for device %s config %s", errRolloutDraftStale, device.DeviceID, config)
			}
			observed, err := runSSHCommandForSite(ctx, schema, siteID, device.DeviceID, "uci show "+config+" 2>&1")
			if err != nil {
				return fmt.Errorf("%w: device %s config %s: %v", errRolloutDraftTransport, device.DeviceID, config, err)
			}
			if observedStateHash(observed) != expected {
				return fmt.Errorf("%w: device %s config %s", errRolloutDraftStale, device.DeviceID, config)
			}
		}
	}
	return nil
}

func verifyRolloutDraftTargets(ctx context.Context, schema, siteID string, draft rolloutDraft) error {
	sqlSchema, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return fmt.Errorf("%w: invalid tenant schema", errRolloutDraftTransport)
	}
	for _, device := range draft.Devices {
		var role string
		var pendingOperation, pendingChangeSet []byte
		err := database.DB.QueryRowContext(ctx, fmt.Sprintf(
			"SELECT COALESCE(device_role, 'AP'), pending_operation, pending_change_set FROM %s.devices WHERE id = $1 AND site_id = $2 AND COALESCE(last_operation->>'state', '') <> 'RECOVERY_REQUIRED' AND COALESCE(last_change_set->>'state', '') <> 'RECOVERY_REQUIRED'", sqlSchema,
		), device.DeviceID, siteID).Scan(&role, &pendingOperation, &pendingChangeSet)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: device %s role changed or is no longer in site", errRolloutDraftStale, device.DeviceID)
			}
			return fmt.Errorf("%w: could not validate device %s: %v", errRolloutDraftTransport, device.DeviceID, err)
		}
		if role != device.Role {
			return fmt.Errorf("%w: device %s role changed", errRolloutDraftStale, device.DeviceID)
		}
		if len(pendingOperation) > 0 && string(pendingOperation) != "null" {
			return fmt.Errorf("%w: device %s has a pending typed operation", errRolloutDraftStale, device.DeviceID)
		}
		if len(pendingChangeSet) > 0 && string(pendingChangeSet) != "null" {
			return fmt.Errorf("%w: device %s has a pending changeset", errRolloutDraftStale, device.DeviceID)
		}
	}
	return nil
}

func observedStateHash(raw string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}

func requestedRolloutID(r *http.Request) (string, error) {
	if rolloutID := strings.TrimSpace(r.URL.Query().Get("rollout_id")); rolloutID != "" {
		return validateRolloutID(rolloutID)
	}
	if r.Body == nil {
		return "", fmt.Errorf("rollout_id is required")
	}
	var payload struct {
		RolloutID string `json:"rollout_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("invalid rollout request")
	}
	if strings.TrimSpace(payload.RolloutID) == "" {
		return "", fmt.Errorf("rollout_id is required")
	}
	return validateRolloutID(strings.TrimSpace(payload.RolloutID))
}

func validateRolloutID(rolloutID string) (string, error) {
	parsed, err := uuid.Parse(rolloutID)
	if err != nil {
		return "", fmt.Errorf("invalid rollout_id")
	}
	return parsed.String(), nil
}

func rolloutDevicePreviews(draft rolloutDraft) []rolloutDevicePreview {
	previews := make([]rolloutDevicePreview, 0, len(draft.Devices))
	for _, device := range draft.Devices {
		lines := append([]string{}, device.PreviewCommands...)
		if device.PreviewCommands == nil {
			// Drafts created before preview_commands was persisted can still be
			// displayed, but new drafts always use the immutable rendered text.
			lines = services.PreviewCommands(device.Commands)
		}
		previews = append(previews, rolloutDevicePreview{
			DeviceID:      device.DeviceID,
			Hostname:      device.Hostname,
			Role:          device.Role,
			Commands:      lines,
			Count:         len(lines),
			Scripts:       copyStringMap(device.Scripts),
			ObservedState: copyStringMap(device.ObservedState),
		})
	}
	return previews
}

func copyStringMap(values map[string]string) map[string]string {
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func rolloutDevicePreviewsFromRecord(record rolloutRecord) []rolloutDevicePreview {
	draft, err := decodeRolloutDraft(database.RolloutDraftRecord{
		SiteID:   record.SiteID,
		PlanHash: record.PlanHash,
		Plan:     record.Plan,
	})
	if err != nil {
		return nil
	}
	return rolloutDevicePreviews(draft)
}

func buildGuardedRolloutScript(config string, commands []services.UciCommand, healthChecks []string, expectedStateHash string) (string, error) {
	script := services.BuildSafeBatchScript(config, commands, healthChecks)
	if script == "" {
		return "", fmt.Errorf("invalid rollout commands for config %s", config)
	}
	if expectedStateHash == "" {
		return "", fmt.Errorf("missing observed state for config %s", config)
	}
	guard := fmt.Sprintf(`observed_rollout_hash=$(uci show %s 2>&1 | sha256sum | awk '{print $1}')
if [ "$observed_rollout_hash" != %q ]; then
  printf 'SITE_ORCHESTRATOR: rollout draft stale for config %s\n' >&2
  exit 75
fi

`, config, expectedStateHash, config)
	const mutationMarker = "# Phase 2: Apply UCI mutations\n"
	markerIndex := strings.Index(script, mutationMarker)
	if markerIndex == -1 {
		return "", fmt.Errorf("invalid rollout script for config %s", config)
	}
	return script[:markerIndex] + guard + script[markerIndex:], nil
}

// PutDeviceRoleHandler updates a device's role (Gateway, AP, IoT_Node).
func PutDeviceRoleHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	username := GetUsernameFromReq(r)

	var body struct {
		Role string `json:"role"`
	}
	if !readBody(w, r, &body) {
		return
	}

	valid := map[string]bool{"Gateway": true, "AP": true, "IoT_Node": true}
	if !valid[body.Role] {
		http.Error(w, `{"error":"invalid role, must be Gateway, AP, or IoT_Node"}`, http.StatusBadRequest)
		return
	}

	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	if err := services.UpdateDeviceRoleForTenant(r.Context(), schema, deviceID, body.Role); err != nil {
		if errors.Is(err, services.ErrDeviceRoleUpdateConflict) {
			http.Error(w, `{"error":"device role cannot change while a rollout is running"}`, http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	database.InsertAuditLog(username, "SITE_ORCHESTRATOR_ROLE_CHANGE", "DEVICE", deviceID,
		fmt.Sprintf("Role changed to: %s", body.Role), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "role": body.Role})
}

// PreviewSyncHandler renders and persists what WOULD be deployed without
// executing. The returned rollout_id is the only input accepted by apply.
func PreviewSyncHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	username := GetUsernameFromReq(r)

	sc, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		http.Error(w, `{"error":"no site config found — save a template first"}`, http.StatusBadRequest)
		return
	}

	devs, err := services.GetSiteDevicesWithRoles(r.Context(), siteID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	targetDeviceID := r.URL.Query().Get("target_device_id")
	namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
	if namespace != "" {
		if _, err := requestedChangeSetNamespaces(namespace); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
	}
	devs, err = selectRolloutDevices(devs, targetDeviceID)
	if err != nil {
		http.Error(w, `{"error":"target device not found in site"}`, http.StatusBadRequest)
		return
	}
	if len(devs) == 0 {
		http.Error(w, `{"error":"no devices found for this site"}`, http.StatusBadRequest)
		return
	}
	healthChecks, err := rolloutHealthTargets(*sc)
	if err != nil {
		http.Error(w, `{"error":"invalid health check target"}`, http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}

	results := services.RenderSiteConfig(*sc, devs)
	results, err = filterRolloutResultsByNamespace(results, namespace)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	preflightCtx, preflightCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Minute)
	results, observedState, err := preflightRenderedResultsWithState(preflightCtx, schema, siteID, results)
	preflightCancel()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	if namespace != "" {
		results, observedState = dropEmptyRolloutResults(results, observedState)
	}
	if namespace != "" && len(results) == 0 {
		http.Error(w, `{"error":"the selected device has no changes to queue"}`, http.StatusConflict)
		return
	}
	if err := rejectUnsafeNetworkMutations(results); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
		return
	}
	draft, err := buildRolloutDraft(siteID, targetDeviceID, healthChecks, results, observedState)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	draft.Namespace = namespace
	plan, err := json.Marshal(draft)
	if err != nil {
		http.Error(w, `{"error":"could not encode rollout draft"}`, http.StatusInternalServerError)
		return
	}
	targets := make([]string, 0, len(draft.Devices))
	for _, device := range draft.Devices {
		targets = append(targets, device.DeviceID)
	}
	targetDeviceIDs, err := json.Marshal(targets)
	if err != nil {
		http.Error(w, `{"error":"could not encode rollout targets"}`, http.StatusInternalServerError)
		return
	}
	persistCtx, persistCancel := rolloutPersistenceContext(r)
	defer persistCancel()
	record, err := database.CreateRolloutDraft(persistCtx, schema, siteID, username, rolloutPlanHash(draft), targetDeviceIDs, plan)
	if err != nil {
		http.Error(w, `{"error":"could not create rollout draft"}`, http.StatusInternalServerError)
		return
	}
	if err := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_DRAFT", siteID,
		fmt.Sprintf("Created rollout draft %s rollout sequence %d plan_hash %s", record.ID, record.Generation, record.PlanHash)); err != nil {
		http.Error(w, `{"error":"could not record rollout audit event"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site_id":          siteID,
		"target_device_id": targetDeviceID,
		"namespace":        namespace,
		"rollout_id":       record.ID,
		"generation":       record.Generation,
		"plan_hash":        record.PlanHash,
		"health_checks":    draft.HealthChecks,
		"devices":          rolloutDevicePreviews(draft),
		"total":            len(draft.Devices),
	})
}

// SyncFleetHandler executes a previously previewed immutable rollout draft.
// It validates one canary device before pushing bounded batches to the rest.
// The draft, not the current site template, determines the target devices and
// commands.
func SyncFleetHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	username := GetUsernameFromReq(r)
	rolloutID, err := requestedRolloutID(r)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	draftCtx, draftCancel := rolloutPersistenceContext(r)
	record, err := database.GetRolloutDraft(draftCtx, schema, siteID, rolloutID)
	draftCancel()
	if err != nil {
		if errors.Is(err, database.ErrRolloutDraftNotFound) {
			http.Error(w, `{"error":"rollout draft was not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"could not load rollout draft"}`, http.StatusInternalServerError)
		}
		return
	}
	draft, err := decodeRolloutDraft(record)
	if err != nil {
		rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
		return
	}
	results := rolloutResultsFromDraft(draft)
	if len(results) == 0 {
		reason := fmt.Errorf("rollout draft contains no devices")
		rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, reason)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, reason.Error()), http.StatusConflict)
		return
	}
	if err := rejectUnsafeNetworkMutations(results); err != nil {
		rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
		return
	}
	executionCtx, executionCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Minute)
	defer executionCancel()
	if draft.Namespace != "" && len(draft.Devices) > 1 {
		response, err := queueFleetDeviceChangeSets(r, schema, siteID, rolloutID, username, record, draft)
		if err != nil {
			if errors.Is(err, database.ErrDeviceChangeSetCapability) {
				rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, err)
				http.Error(w, `{"error":"one or more target devices do not support DeviceChangeSet delivery"}`, http.StatusConflict)
			} else {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	if draft.Namespace != "" && len(draft.Devices) == 1 {
		changeSet, err := buildSingleDeviceChangeSet(rolloutID, draft)
		if err != nil {
			rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, err)
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusConflict)
			return
		}
		if record.Status == "QUEUED" {
			if queuedResult, ok := queuedChangeSetResult(record, changeSet); ok {
				writeQueuedChangeSetResponse(w, rolloutID, record, changeSet, queuedResult)
				return
			}
			http.Error(w, `{"error":"rollout is already queued with a different changeset"}`, http.StatusConflict)
			return
		}
		changeSetRaw, err := json.Marshal(changeSet)
		if err != nil {
			handleRolloutDraftValidationFailure(w, r, schema, siteID, rolloutID, "", username,
				fmt.Errorf("%w: could not encode changeset: %v", errRolloutDraftTransport, err))
			return
		}
		queuedResultRaw, err := json.Marshal([]fleetSyncResult{{
			DeviceID:    changeSet.DeviceID,
			Hostname:    draft.Devices[0].Hostname,
			Role:        draft.Devices[0].Role,
			Status:      "QUEUED",
			Output:      "device agent will apply and report the durable changeset result",
			CmdCount:    changeSetCommandCount(changeSet),
			ChangeSetID: changeSet.ChangeSetID,
			PlanHash:    changeSet.PlanHash,
		}})
		if err != nil {
			handleRolloutDraftValidationFailure(w, r, schema, siteID, rolloutID, "", username,
				fmt.Errorf("%w: could not encode queued result: %v", errRolloutDraftTransport, err))
			return
		}
		queueCtx, queueCancel := rolloutPersistenceContext(r)
		deviceGeneration, err := database.ClaimAndQueueDeviceChangeSet(queueCtx, schema, siteID, rolloutID, draft.Devices[0].Role, changeSetRaw, queuedResultRaw, username, r.RemoteAddr)
		queueCancel()
		if err != nil {
			if errors.Is(err, database.ErrDeviceChangeSetCapability) {
				rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, err)
				http.Error(w, `{"error":"target device does not support DeviceChangeSet delivery"}`, http.StatusConflict)
				return
			}
			if errors.Is(err, database.ErrRolloutDraftNotAvailable) {
				if retryRecord, retryErr := database.GetRolloutDraft(r.Context(), schema, siteID, rolloutID); retryErr == nil && retryRecord.Status == "QUEUED" {
					if queuedResult, ok := queuedChangeSetResult(retryRecord, changeSet); ok {
						writeQueuedChangeSetResponse(w, rolloutID, retryRecord, changeSet, queuedResult)
						return
					}
				}
				http.Error(w, `{"error":"rollout draft is no longer available"}`, http.StatusConflict)
			} else {
				http.Error(w, `{"error":"could not queue changeset"}`, http.StatusConflict)
			}
			return
		}
		writeQueuedChangeSetResponse(w, rolloutID, record, changeSet, fleetSyncResult{DeviceGeneration: deviceGeneration})
		return
	}
	if draft.Namespace != "" {
		reason := fmt.Errorf("namespace %s is not supported by the safe changeset slice", draft.Namespace)
		rejectUnexecutableRolloutDraft(r, schema, siteID, rolloutID, username, reason)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, reason.Error()), http.StatusConflict)
		return
	}
	if err := verifyRolloutDraftTargets(executionCtx, schema, siteID, draft); err != nil {
		handleRolloutDraftValidationFailure(w, r, schema, siteID, rolloutID, "", username, err)
		return
	}
	claimCtx, claimCancel := rolloutPersistenceContext(r)
	record, err = database.ClaimRolloutDraft(claimCtx, schema, siteID, rolloutID)
	claimCancel()
	if err != nil {
		if errors.Is(err, database.ErrRolloutDraftNotAvailable) {
			http.Error(w, `{"error":"rollout draft is no longer available"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error":"could not claim rollout draft"}`, http.StatusInternalServerError)
		}
		return
	}
	if err := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_START", siteID,
		fmt.Sprintf("Claimed rollout draft %s rollout sequence %d plan_hash %s", rolloutID, record.Generation, record.PlanHash)); err != nil {
		if releaseErr := releaseRolloutDraftForRetry(r, schema, siteID, rolloutID, record.ClaimToken); releaseErr != nil {
			log.Printf("[SITE_ORCHESTRATOR][WARN] failed to release rollout draft %s after audit failure: %v", rolloutID, releaseErr)
		}
		http.Error(w, `{"error":"could not record rollout audit event; rollout was not applied"}`, http.StatusServiceUnavailable)
		return
	}
	rolloutSequence := record.Generation
	if err := verifyRolloutDraftObservedState(executionCtx, schema, siteID, draft); err != nil {
		handleRolloutDraftValidationFailure(w, r, schema, siteID, rolloutID, record.ClaimToken, username, err)
		return
	}
	for _, result := range results {
		if len(result.Commands) == 0 {
			continue
		}
		if err := services.CreateBackupForSite(executionCtx, schema, siteID, result.DeviceID); err != nil {
			if releaseErr := releaseRolloutDraftForRetry(r, schema, siteID, rolloutID, record.ClaimToken); releaseErr != nil {
				log.Printf("[SITE_ORCHESTRATOR][WARN] failed to release rollout draft %s after backup failure: %v", rolloutID, releaseErr)
			}
			if auditErr := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_BACKUP_FAILED", siteID,
				fmt.Sprintf("Rollout %s backup failed for device %s: %v", rolloutID, result.DeviceID, err)); auditErr != nil {
				http.Error(w, `{"error":"could not record rollout audit event"}`, http.StatusServiceUnavailable)
				return
			}
			http.Error(w, `{"error":"pre-change backup failed; rollout was not applied"}`, http.StatusServiceUnavailable)
			return
		}
	}
	if err := verifyRolloutDraftObservedState(executionCtx, schema, siteID, draft); err != nil {
		handleRolloutDraftValidationFailure(w, r, schema, siteID, rolloutID, record.ClaimToken, username, err)
		return
	}
	if err := markRolloutDevicesRunning(r, siteID, rolloutID, record.ClaimToken, draft.Devices); err != nil {
		if errors.Is(err, errRolloutDeviceConflict) || errors.Is(err, errRolloutClaimLost) {
			staleCtx, staleCancel := rolloutPersistenceContext(r)
			if staleErr := database.MarkRolloutDraftStale(staleCtx, schema, siteID, rolloutID, record.ClaimToken); staleErr != nil {
				log.Printf("[SITE_ORCHESTRATOR][WARN] failed to mark device-conflicted rollout %s stale: %v", rolloutID, staleErr)
			}
			staleCancel()
			if auditErr := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_STALE", siteID,
				fmt.Sprintf("Rollout %s rejected because a target device changed state", rolloutID)); auditErr != nil {
				http.Error(w, `{"error":"could not record rollout audit event"}`, http.StatusServiceUnavailable)
				return
			}
			http.Error(w, `{"error":"a target device changed state; rollout draft is stale"}`, http.StatusConflict)
			return
		}
		if releaseErr := releaseRolloutDraftForRetry(r, schema, siteID, rolloutID, record.ClaimToken); releaseErr != nil {
			log.Printf("[SITE_ORCHESTRATOR][WARN] failed to release rollout draft %s after device-state failure: %v", rolloutID, releaseErr)
		}
		if auditErr := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_ROLLOUT_DEVICE_STATE_FAILED", siteID,
			fmt.Sprintf("Rollout %s could not mark target devices: %v", rolloutID, err)); auditErr != nil {
			http.Error(w, `{"error":"could not record rollout audit event"}`, http.StatusServiceUnavailable)
			return
		}
		http.Error(w, `{"error":"could not mark target devices for rollout"}`, http.StatusInternalServerError)
		return
	}

	// Execute the first device as a canary, then process the remaining devices
	// in bounded batches only after the canary succeeds.
	syncResults := make([]fleetSyncResult, len(results))
	executeBatch := func(indices []int) {
		jobs := make(chan int)
		workerCount := fleetSyncMaxConcurrency
		if len(indices) < workerCount {
			workerCount = len(indices)
		}
		var wg sync.WaitGroup
		for worker := 0; worker < workerCount; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range jobs {
					rr := results[idx]

					sr := fleetSyncResult{
						DeviceID: rr.DeviceID,
						Hostname: rr.Hostname,
						Role:     rr.Role,
						CmdCount: len(rr.Commands),
					}

					if len(rr.Commands) == 0 {
						sr.Status = "SKIPPED"
						sr.Output = "No commands to execute"
						syncResults[idx] = sr
						continue
					}

					// Group commands by config namespace to build per-namespace batch scripts
					configGroups := groupCommandsByConfig(rr.Commands)
					var allOutput string
					for _, cfg := range sortedConfigNames(configGroups) {
						script := draft.Devices[idx].Scripts[cfg]
						if script == "" {
							sr.Status = "FAILED"
							sr.Error = boundedRolloutDiagnostic(fmt.Sprintf("missing immutable rollout script for config %s", cfg))
							sr.Output = boundedRolloutDiagnostic(allOutput)
							syncResults[idx] = sr
							break
						}
						out, err := runSSHScriptForSite(executionCtx, schema, siteID, rr.DeviceID, script)
						allOutput += out + "\n"
						if err != nil {
							sr.Status = "FAILED"
							var exitErr *sshExitStatusError
							if errors.As(err, &exitErr) && exitErr.status == 75 {
								sr.Status = "STALE"
							}
							sr.Error = boundedRolloutDiagnostic(fmt.Sprintf("config namespace %s: %v\n%s", cfg, err, out))
							sr.Output = boundedRolloutDiagnostic(allOutput)
							syncResults[idx] = sr
							break
						}
					}
					if sr.Status == "FAILED" || sr.Status == "STALE" {
						continue
					}
					sr.Status = "SUCCESS"
					sr.Output = boundedRolloutDiagnostic(allOutput)
					syncResults[idx] = sr
					log.Printf("[SITE_ORCHESTRATOR] Synced %s (%s) - %d commands", rr.Hostname, rr.Role, len(rr.Commands))
				}
			}()
		}
		for _, idx := range indices {
			jobs <- idx
		}
		close(jobs)
		wg.Wait()
	}

	phases := fleetRolloutPhases(len(results))
	if fleetSyncSequential {
		phases = orderedFleetRolloutPhases(len(results))
	}
	executeBatch(phases[0])
	canaryOK := rolloutResultSuccess(syncResults[phases[0][0]].Status)
	rolloutStatus := "completed"
	if !canaryOK {
		rolloutStatus = "canary_failed"
		if syncResults[phases[0][0]].Status == "STALE" {
			rolloutStatus = "STALE"
		}
		for _, phase := range phases[1:] {
			for _, idx := range phase {
				syncResults[idx] = fleetSyncResult{DeviceID: results[idx].DeviceID, Hostname: results[idx].Hostname, Role: results[idx].Role, Status: "ABORTED", CmdCount: len(results[idx].Commands), Error: "canary failed; rollout not started"}
			}
		}
	} else {
		for _, phase := range phases[1:] {
			executeBatch(phase)
			if !rolloutResultSuccess(syncResults[phase[0]].Status) {
				rolloutStatus = "aborted"
				if syncResults[phase[0]].Status == "STALE" {
					rolloutStatus = "STALE"
				}
				for _, laterPhase := range phases[1:] {
					for _, idx := range laterPhase {
						if syncResults[idx].Status == "" {
							syncResults[idx] = fleetSyncResult{DeviceID: results[idx].DeviceID, Hostname: results[idx].Hostname, Role: results[idx].Role, Status: "ABORTED", CmdCount: len(results[idx].Commands), Error: "previous device failed; rollout not started"}
						}
					}
				}
				break
			}
		}
	}
	if err := persistRolloutResults(r, siteID, rolloutID, record.ClaimToken, rolloutStatus, syncResults); err != nil {
		log.Printf("[SITE_ORCHESTRATOR][ERROR] failed to persist rollout %s: %v", rolloutID, err)
		if recoveryErr := markRolloutPersistenceFailure(r, siteID, rolloutID, record.ClaimToken, syncResults, err); recoveryErr != nil {
			log.Printf("[SITE_ORCHESTRATOR][ERROR] failed to close rollout %s after persistence failure: %v", rolloutID, recoveryErr)
		}
		http.Error(w, `{"error":"could not persist rollout results"}`, http.StatusInternalServerError)
		return
	}

	// Count successes/failures
	successes, failures := 0, 0
	for _, sr := range syncResults {
		if sr.Status == "SUCCESS" || sr.Status == "SKIPPED" {
			successes++
		} else {
			failures++
		}
	}

	auditErr := auditRolloutEvent(r, username, "SITE_ORCHESTRATOR_FLEET_SYNC", siteID,
		fmt.Sprintf("Fleet sync: %d success, %d failed, rollout: %s", successes, failures, rolloutID))
	if auditErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "rollout completed but audit persistence failed",
			"status":     rolloutStatus,
			"rollout_id": rolloutID,
			"generation": rolloutSequence,
			"successes":  successes,
			"failures":   failures,
			"results":    syncResults,
		})
		return
	}
	if rolloutStatus == "STALE" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "rollout draft became stale during execution",
			"status":     rolloutStatus,
			"rollout_id": rolloutID,
			"generation": rolloutSequence,
			"successes":  successes,
			"failures":   failures,
			"results":    syncResults,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           rolloutStatus,
		"rollout_id":       rolloutID,
		"generation":       rolloutSequence,
		"target_device_id": draft.TargetDeviceID,
		"successes":        successes,
		"failures":         failures,
		"results":          syncResults,
	})
}

// markRolloutDevicesRunning fences typed device operations while the legacy
// SSH rollout is active. Rollout generations are site-scoped and must never be
// copied into device-local generation columns.
func markRolloutDevicesRunning(r *http.Request, siteID, rolloutID, claimToken string, devices []rolloutDraftDevice) error {
	schema, err := getTenantSchema(r)
	if err != nil {
		return err
	}
	ctx, cancel := rolloutPersistenceContext(r)
	defer cancel()
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := assertRolloutClaim(ctx, tx, schema, rolloutID, claimToken); err != nil {
		return err
	}
	if len(devices) == 0 {
		if _, err = tx.ExecContext(ctx, "UPDATE "+schema+".devices SET last_rollout_status = 'RUNNING', last_rollout_at = CURRENT_TIMESTAMP WHERE site_id = $1 AND pending_operation IS NULL AND pending_change_set IS NULL AND COALESCE(last_rollout_status, '') <> 'RUNNING' AND COALESCE(last_operation->>'state', '') <> 'RECOVERY_REQUIRED' AND COALESCE(last_change_set->>'state', '') <> 'RECOVERY_REQUIRED'", siteID); err != nil {
			return err
		}
		return tx.Commit()
	}
	for _, device := range devices {
		result, err := tx.ExecContext(ctx, "UPDATE "+schema+".devices SET last_rollout_status = 'RUNNING', last_rollout_at = CURRENT_TIMESTAMP WHERE site_id = $1 AND id = $2 AND pending_operation IS NULL AND pending_change_set IS NULL AND COALESCE(last_rollout_status, '') <> 'RUNNING' AND COALESCE(last_operation->>'state', '') <> 'RECOVERY_REQUIRED' AND COALESCE(last_change_set->>'state', '') <> 'RECOVERY_REQUIRED' AND COALESCE(device_role, 'AP') = $3", siteID, device.DeviceID, device.Role)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("%w: device %s changed role, is already in a rollout, or has a pending operation", errRolloutDeviceConflict, device.DeviceID)
		}
	}
	return tx.Commit()
}

func persistRolloutResults(r *http.Request, siteID, rolloutID, claimToken, status string, results []fleetSyncResult) error {
	schema, err := getTenantSchema(r)
	if err != nil {
		return err
	}
	ctx, cancel := rolloutPersistenceContext(r)
	defer cancel()
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := assertRolloutClaim(ctx, tx, schema, rolloutID, claimToken); err != nil {
		return err
	}
	for _, result := range results {
		status := "FAILED"
		if rolloutResultSuccess(result.Status) {
			status = "SUCCESS"
		} else if result.Status == "ABORTED" {
			status = "ABORTED"
		} else if result.Status == "STALE" {
			status = "STALE"
		}
		query := "UPDATE " + schema + ".devices SET last_rollout_status = $1, last_rollout_at = CURRENT_TIMESTAMP WHERE site_id = $2 AND id = $3 AND last_rollout_status = 'RUNNING'"
		args := []any{status, siteID, result.DeviceID}
		execResult, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		affected, err := execResult.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("device %s rollout status is no longer current", result.DeviceID)
		}
	}
	encoded, err := json.Marshal(stripFleetSyncOutput(results))
	if err != nil {
		return err
	}
	runResult, err := tx.ExecContext(ctx, "UPDATE "+schema+".rollout_runs SET status = $1, results = $2, claim_token = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $3 AND status = 'RUNNING' AND claim_token::text = $4", status, encoded, rolloutID, claimToken)
	if err != nil {
		return err
	}
	affected, err := runResult.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errRolloutClaimLost
	}
	return tx.Commit()
}

func assertRolloutClaim(ctx context.Context, tx *sql.Tx, schema, rolloutID, claimToken string) error {
	var status string
	err := tx.QueryRowContext(ctx, "SELECT status FROM "+schema+".rollout_runs WHERE id = $1 AND claim_token::text = $2 FOR UPDATE", rolloutID, claimToken).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errRolloutClaimLost
		}
		return err
	}
	if status != "RUNNING" {
		return errRolloutClaimLost
	}
	return nil
}

func GetRolloutHistoryHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			http.Error(w, `{"error":"limit must be between 1 and 100"}`, http.StatusBadRequest)
			return
		}
	}
	rows, err := database.Tx(r.Context()).QueryContext(r.Context(), "SELECT id, site_id, generation, status, plan_hash, requested_by, target_device_ids, plan, results, worker_cursor, worker_lease_until, created_at, updated_at FROM "+schema+".rollout_runs WHERE site_id = $1 ORDER BY created_at DESC LIMIT $2", r.PathValue("site_id"), limit)
	if err != nil {
		http.Error(w, `{"error":"could not load rollout history"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	rollouts := make([]rolloutRecord, 0)
	for rows.Next() {
		var rollout rolloutRecord
		var leaseUntil sql.NullTime
		if err := rows.Scan(&rollout.ID, &rollout.SiteID, &rollout.Generation, &rollout.Status, &rollout.PlanHash, &rollout.RequestedBy, &rollout.TargetDeviceIDs, &rollout.Plan, &rollout.Results, &rollout.WorkerCursor, &leaseUntil, &rollout.CreatedAt, &rollout.UpdatedAt); err != nil {
			http.Error(w, `{"error":"could not read rollout history"}`, http.StatusInternalServerError)
			return
		}
		if leaseUntil.Valid {
			rollout.WorkerLeaseUntil = &leaseUntil.Time
		}
		if rollout.Status == "DRAFT" {
			rollout.Preview = rolloutDevicePreviewsFromRecord(rollout)
		}
		rollouts = append(rollouts, rollout)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, `{"error":"could not read rollout history"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollouts)
}

func GetRolloutHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	row := database.Tx(r.Context()).QueryRowContext(r.Context(), "SELECT id, site_id, generation, status, plan_hash, requested_by, target_device_ids, plan, results, worker_cursor, worker_lease_until, created_at, updated_at FROM "+schema+".rollout_runs WHERE id = $1 AND site_id = $2", r.PathValue("rollout_id"), r.PathValue("site_id"))
	var rollout rolloutRecord
	var leaseUntil sql.NullTime
	if err := row.Scan(&rollout.ID, &rollout.SiteID, &rollout.Generation, &rollout.Status, &rollout.PlanHash, &rollout.RequestedBy, &rollout.TargetDeviceIDs, &rollout.Plan, &rollout.Results, &rollout.WorkerCursor, &leaseUntil, &rollout.CreatedAt, &rollout.UpdatedAt); err != nil {
		http.Error(w, `{"error":"rollout not found"}`, http.StatusNotFound)
		return
	}
	if leaseUntil.Valid {
		rollout.WorkerLeaseUntil = &leaseUntil.Time
	}
	if rollout.Status == "DRAFT" {
		rollout.Preview = rolloutDevicePreviewsFromRecord(rollout)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollout)
}

type rolloutRecord struct {
	ID               string                 `json:"id"`
	SiteID           string                 `json:"site_id"`
	Generation       int64                  `json:"generation"`
	Status           string                 `json:"status"`
	PlanHash         string                 `json:"plan_hash"`
	RequestedBy      string                 `json:"requested_by"`
	TargetDeviceIDs  json.RawMessage        `json:"target_device_ids"`
	Plan             json.RawMessage        `json:"plan"`
	Results          json.RawMessage        `json:"results"`
	WorkerCursor     int                    `json:"worker_cursor"`
	WorkerLeaseUntil *time.Time             `json:"worker_lease_until,omitempty"`
	Preview          []rolloutDevicePreview `json:"preview,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

func markRolloutPersistenceFailure(r *http.Request, siteID, rolloutID, claimToken string, results []fleetSyncResult, cause error) error {
	schema, err := getTenantSchema(r)
	if err != nil {
		return err
	}
	failureResults := append([]fleetSyncResult{}, results...)
	failureResults = append(failureResults, fleetSyncResult{
		Status: "FAILED",
		Error:  boundedRolloutDiagnostic("rollout result persistence failed: " + cause.Error()),
	})
	encoded, err := json.Marshal(stripFleetSyncOutput(failureResults))
	if err != nil {
		return err
	}
	ctx, cancel := rolloutPersistenceContext(r)
	defer cancel()
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE "+schema+".devices SET last_rollout_status = 'FAILED', last_rollout_at = CURRENT_TIMESTAMP WHERE site_id = $1 AND last_rollout_status = 'RUNNING'", siteID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE "+schema+".rollout_runs SET status = 'FAILED', results = $1, claim_token = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $2 AND status = 'RUNNING' AND claim_token::text = $3", encoded, rolloutID, claimToken)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errRolloutClaimLost
	}
	return tx.Commit()
}

func boundedRolloutDiagnostic(value string) string {
	if len(value) <= maxRolloutDiagnosticBytes {
		return value
	}
	return value[:maxRolloutDiagnosticBytes] + "\n[diagnostic output truncated]"
}

func rolloutResultSuccess(status string) bool {
	return status == "SUCCESS" || status == "SKIPPED"
}

func stripFleetSyncOutput(results []fleetSyncResult) []fleetSyncResult {
	clean := make([]fleetSyncResult, len(results))
	copy(clean, results)
	for i := range clean {
		clean[i].Output = boundedRolloutDiagnostic(clean[i].Output)
	}
	return clean
}

// groupCommandsByConfig splits commands into per-namespace buckets for batch execution.
func groupCommandsByConfig(cmds []services.UciCommand) map[string][]services.UciCommand {
	groups := make(map[string][]services.UciCommand)
	for _, cmd := range cmds {
		groups[cmd.Config] = append(groups[cmd.Config], cmd)
	}
	return groups
}

func filterUCICommandsByObservedState(commands []services.UciCommand, sections []UCISection) []services.UciCommand {
	filtered := make([]services.UciCommand, 0, len(commands))
	for index, command := range commands {
		if command.Option != "" &&
			(uciCommandMatchesObservedInPlan(index, command, commands, sections) || isDefaultDifference(command, sections)) {
			continue
		}
		filtered = append(filtered, command)
	}
	return filtered
}

func rejectUnsafeNetworkMutations(results []services.RenderResult) error {
	for _, result := range results {
		for _, command := range result.Commands {
			if command.Config == "network" {
				return fmt.Errorf("network mutation for device %s is blocked until a transport-safe executor is available", result.DeviceID)
			}
		}
	}
	return nil
}

func preflightRenderedResultsWithState(ctx context.Context, schema, siteID string, results []services.RenderResult) ([]services.RenderResult, map[string]map[string]string, error) {
	filtered := make([]services.RenderResult, len(results))
	copy(filtered, results)
	observedState := make(map[string]map[string]string, len(results))
	for index, result := range filtered {
		groups := groupCommandsByConfig(result.Commands)
		commands := make([]services.UciCommand, 0, len(result.Commands))
		deviceState := make(map[string]string, len(groups))
		for _, config := range sortedConfigNames(groups) {
			observed, err := runSSHCommandForSite(ctx, schema, siteID, result.DeviceID, "uci show "+config+" 2>&1")
			if err != nil {
				return nil, nil, fmt.Errorf("preflight failed for device %s config %s: %w", result.DeviceID, config, err)
			}
			deviceState[config] = observedStateHash(observed)
			sections := parseUciShow(observed, config)
			commands = append(commands, filterUCICommandsByObservedState(groups[config], sections)...)
		}
		filtered[index].Commands = commands
		observedState[result.DeviceID] = deviceState
	}
	return filtered, observedState, nil
}

func rolloutPersistenceContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
}

func sortedConfigNames(groups map[string][]services.UciCommand) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fleetRolloutPhases(deviceCount int) [][]int {
	if deviceCount == 0 {
		return nil
	}
	phases := [][]int{{0}}
	for start := 1; start < deviceCount; start += fleetSyncMaxConcurrency {
		end := start + fleetSyncMaxConcurrency
		if end > deviceCount {
			end = deviceCount
		}
		phase := make([]int, end-start)
		for offset := range phase {
			phase[offset] = start + offset
		}
		phases = append(phases, phase)
	}
	return phases
}

func sequentialFleetRolloutPhases(results []services.RenderResult) [][]int {
	indices := make([]int, len(results))
	for idx := range results {
		indices[idx] = idx
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left, right := results[indices[i]], results[indices[j]]
		leftGateway := left.Role == "Gateway"
		rightGateway := right.Role == "Gateway"
		if leftGateway != rightGateway {
			return !leftGateway
		}
		return left.DeviceID < right.DeviceID
	})
	phases := make([][]int, 0, len(results))
	for _, idx := range indices {
		phases = append(phases, []int{idx})
	}
	return phases
}

func orderedFleetRolloutPhases(deviceCount int) [][]int {
	if deviceCount == 0 {
		return nil
	}
	phases := make([][]int, deviceCount)
	for index := range phases {
		phases[index] = []int{index}
	}
	return phases
}
