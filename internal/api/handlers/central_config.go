package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

var uciPathSegmentPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_-]+|@[A-Za-z0-9_-]+(?:\[-?[0-9]+\])?)$`)

var uciAnonymousSectionPattern = regexp.MustCompile(`^@([A-Za-z0-9_-]+)\[(-?[0-9]+)\]$`)

func buildCentralConfigCommand(config, path string) (string, error) {
	if !isAllowedUciConfig(config) {
		return "", fmt.Errorf("config namespace not allowed")
	}

	target := config
	if path != "" {
		if len(path) > 128 || strings.TrimSpace(path) != path {
			return "", fmt.Errorf("invalid UCI path")
		}
		parts := strings.Split(path, ".")
		if len(parts) < 1 || len(parts) > 3 || parts[0] != config {
			return "", fmt.Errorf("UCI path must stay inside the selected config")
		}
		for _, part := range parts {
			if !uciPathSegmentPattern.MatchString(part) {
				return "", fmt.Errorf("invalid UCI path segment")
			}
		}
		target = path
	}

	return fmt.Sprintf("uci show %s 2>&1", target), nil
}

// ─── CENTRAL_LUCI Handlers ───────────────────────────────────────────────────
// Centralised LuCI-grade configuration management for distributed OpenWrt fleets.

// GetCentralConfigHandler reads an entire UCI namespace from the device
// using `uci show <config>` in programmable notation, which returns
// structured key=value pairs parseable by the frontend.
//
// Also supports optional `?path=` query for scoped reads, e.g.:
//
//	GET /api/devices/{device_id}/central-config?config=wireless&path=wireless.radio0.channel
func GetCentralConfigHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	config := r.URL.Query().Get("config")
	path := r.URL.Query().Get("path")

	if config == "" {
		http.Error(w, `{"error":"missing 'config' parameter"}`, http.StatusBadRequest)
		return
	}
	if !isAllowedUciConfig(config) {
		http.Error(w, `{"error":"config namespace not allowed"}`, http.StatusBadRequest)
		return
	}

	cmd, err := buildCentralConfigCommand(config, path)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	out, err := runSSHCommand(deviceID, cmd)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  err.Error(),
			"output": out,
		})
		return
	}

	// Parse `uci show` output into structured JSON
	parsed := parseUciShow(out, config)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id": deviceID,
		"config":    config,
		"sections":  parsed,
		"raw":       out,
	})
}

// ListCentralConfigsHandler returns the list of available UCI config namespaces
// on the device by reading /etc/config/ directory listing.
func ListCentralConfigsHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")

	out, err := runSSHCommand(deviceID, "ls /etc/config/ 2>/dev/null")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	configs := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			configs = append(configs, line)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id": deviceID,
		"configs":   configs,
	})
}

// PreviewCentralConfigHandler translates UciCommand structs into their
// shell-safe UCI strings WITHOUT executing them — for the "command preview" pane.
func PreviewCentralConfigHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Commands []services.UciCommand `json:"commands"`
	}
	if !readBody(w, r, &payload) {
		return
	}

	lines := services.PreviewCommands(payload.Commands)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preview": lines,
	})
}

// PutCentralConfigHandler receives typed UCI commands, creates a Vault backup,
// then queues the operation for the device-local executor. It never performs a
// live mutation in the HTTP request.
func PutCentralConfigHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	config := r.URL.Query().Get("config")
	dryRun := r.URL.Query().Get("dry_run") == "true"
	username := GetUsernameFromReq(r)

	if config == "" {
		http.Error(w, `{"error":"missing 'config' parameter"}`, http.StatusBadRequest)
		return
	}
	if !isAllowedUciConfig(config) {
		http.Error(w, `{"error":"config namespace not allowed"}`, http.StatusBadRequest)
		return
	}

	var payload struct {
		Commands     []services.UciCommand `json:"commands"`
		Confirm      bool                  `json:"confirm"`
		HealthChecks []string              `json:"health_checks"`
	}
	if !readBody(w, r, &payload) {
		return
	}

	if len(payload.Commands) == 0 {
		http.Error(w, `{"error":"empty command list"}`, http.StatusBadRequest)
		return
	}
	if dryRun || !payload.Confirm {
		if services.BuildDryRunScript(config, payload.Commands) == "" {
			http.Error(w, `{"error":"invalid UCI command or config"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "preview",
			"config":    config,
			"commands":  services.PreviewCommands(payload.Commands),
			"next_step": "repeat with confirm=true to queue this operation",
		})
		return
	}
	plan, err := services.NewDeviceOperationPlan(config, payload.Commands, payload.HealthChecks, true)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}

	// ── VAULT INTEGRATION: Pre-change backup ─────────────────────────────
	// Before any destructive change, snapshot the entire /etc/config/<config>
	// into The Vault as a safety net.
	log.Printf("[CENTRAL_LUCI] Triggering pre-change Vault backup for device %s, config: %s", deviceID, config)
	if err := services.CreateBackup(context.Background(), schema, deviceID); err != nil {
		log.Printf("[CENTRAL_LUCI][WARN] Pre-change backup failed for %s: %v", deviceID, err)
		http.Error(w, `{"error":"pre-change backup failed; configuration was not applied"}`, http.StatusServiceUnavailable)
		return
	}

	planJSON, err := json.Marshal(plan)
	if err != nil {
		http.Error(w, `{"error":"could not serialize operation plan"}`, http.StatusInternalServerError)
		return
	}
	if err := database.QueueDeviceOperation(r.Context(), schema, deviceID, planJSON); err != nil {
		database.InsertAuditLog(username, "CENTRAL_LUCI_QUEUE_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_queued", "error": err.Error()})
		return
	}
	if err := database.CommitRequestTx(r.Context()); err != nil {
		database.InsertAuditLog(username, "CENTRAL_LUCI_QUEUE_COMMIT_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		http.Error(w, `{"error":"operation queue commit failed"}`, http.StatusInternalServerError)
		return
	}

	database.InsertAuditLog(username, "CENTRAL_LUCI_QUEUED", "DEVICE", deviceID,
		fmt.Sprintf("Queued %d UCI commands to namespace: %s", len(payload.Commands), config), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "queued",
		"operation_id": plan.OperationID,
		"plan_hash":    plan.PlanHash,
		"message":      "device agent will apply and report the durable result",
	})
}

// SafeRolloutHandler applies a validated UCI batch to exactly one device.
// It is intentionally opt-in: without confirm=true it only returns a plan.
func SafeRolloutHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	config := r.URL.Query().Get("config")
	if deviceID == "" || config == "" || !isAllowedUciConfig(config) {
		http.Error(w, `{"error":"device_id and a valid config are required"}`, http.StatusBadRequest)
		return
	}

	var payload struct {
		Commands     []services.UciCommand `json:"commands"`
		Confirm      bool                  `json:"confirm"`
		HealthChecks []string              `json:"health_checks"`
	}
	if !readBody(w, r, &payload) || len(payload.Commands) == 0 {
		return
	}
	for _, command := range payload.Commands {
		if command.Config != config {
			http.Error(w, `{"error":"all commands must target the selected config"}`, http.StatusBadRequest)
			return
		}
	}

	preview := services.PreviewCommands(payload.Commands)
	if !payload.Confirm {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "preview",
			"device_id": deviceID,
			"config":    config,
			"commands":  preview,
			"next_step": "repeat with confirm=true to apply to this device",
		})
		return
	}
	plan, err := services.NewDeviceOperationPlan(config, payload.Commands, payload.HealthChecks, true)
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
		http.Error(w, `{"error":"pre-change backup failed; rollout aborted"}`, http.StatusServiceUnavailable)
		return
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		http.Error(w, `{"error":"could not serialize operation plan"}`, http.StatusInternalServerError)
		return
	}
	if err := database.QueueDeviceOperation(r.Context(), schema, deviceID, planJSON); err != nil {
		database.InsertAuditLog(GetUsernameFromReq(r), "SAFE_ROLLOUT_QUEUE_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_queued", "error": err.Error()})
		return
	}
	if err := database.CommitRequestTx(r.Context()); err != nil {
		database.InsertAuditLog(GetUsernameFromReq(r), "SAFE_ROLLOUT_QUEUE_COMMIT_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		http.Error(w, `{"error":"operation queue commit failed"}`, http.StatusInternalServerError)
		return
	}

	database.InsertAuditLog(GetUsernameFromReq(r), "SAFE_ROLLOUT_QUEUED", "DEVICE", deviceID, fmt.Sprintf("Queued %d commands to %s", len(payload.Commands), config), r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "queued",
		"operation_id": plan.OperationID,
		"plan_hash":    plan.PlanHash,
		"message":      "device agent will apply and report the durable result",
	})
}

// GetDeviceOperationHandler returns the durable controller/agent operation
// state without attempting to reach the device over SSH.
func GetDeviceOperationHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error":"device_id is required"}`, http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	pending, pendingErr := database.GetPendingDeviceOperation(r.Context(), schema, deviceID)
	last, lastErr := database.GetLastDeviceOperation(r.Context(), schema, deviceID)
	if pendingErr != nil || lastErr != nil {
		http.Error(w, `{"error":"could not read device operation state"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id":         deviceID,
		"pending_operation": nullableJSON(pending),
		"last_operation":    nullableJSON(last),
		"transport":         "device_agent",
	})
}

func nullableJSON(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var value interface{}
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

// GetDeviceDriftHandler compares the rendered desired state with live UCI.
// It is read-only: no commit, restart, or remote mutation is performed.
func GetDeviceDriftHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error":"device_id is required"}`, http.StatusBadRequest)
		return
	}
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}
	var siteID, name, role string
	if err := database.Tx(r.Context()).QueryRow("SELECT site_id, COALESCE(NULLIF(state_json->'board'->>'hostname', ''), NULLIF(name, ''), NULLIF(model, ''), id), COALESCE(device_role, 'AP') FROM "+schema+".devices WHERE id = $1", deviceID).Scan(&siteID, &name, &role); err != nil {
		http.Error(w, `{"error":"device not found"}`, http.StatusNotFound)
		return
	}
	cfg, err := services.GetSiteConfig(r.Context(), siteID)
	if err != nil {
		http.Error(w, `{"error":"desired site config not found"}`, http.StatusUnprocessableEntity)
		return
	}
	results := services.RenderSiteConfig(*cfg, []services.DeviceRoleInfo{{DeviceID: deviceID, Hostname: name, Role: role}})
	if len(results) == 0 {
		http.Error(w, `{"error":"desired state rendered empty"}`, http.StatusUnprocessableEntity)
		return
	}

	byConfig := map[string][]services.UciCommand{}
	for _, command := range results[0].Commands {
		byConfig[command.Config] = append(byConfig[command.Config], command)
	}
	type NamespaceResult struct {
		Config   string   `json:"config"`
		Status   string   `json:"status"`
		Expected []string `json:"expected"`
		Observed string   `json:"observed,omitempty"`
		Missing  []string `json:"missing,omitempty"`
		Defaults []string `json:"default_differences,omitempty"`
		Error    string   `json:"error,omitempty"`
	}
	resultsByNamespace := make([]NamespaceResult, 0, len(byConfig))
	for config, commands := range byConfig {
		if config == "sqm" || config == "firewall" {
			// Preserve the comparison contract but avoid leaking secrets/large UCI dumps.
		}
		out, readErr := runSSHCommand(deviceID, "uci show "+config+" 2>&1")
		preview := services.PreviewCommands(commands)
		redacted := make([]string, 0, len(preview))
		for _, command := range preview {
			redacted = append(redacted, redactUCICommand(command))
		}
		result := NamespaceResult{Config: config, Expected: redacted}
		if readErr != nil {
			result.Status = "UNREACHABLE"
			result.Error = readErr.Error()
		} else {
			sections := parseUciShow(out, config)
			result.Observed = redactUCIRaw(out)
			for commandIndex, command := range commands {
				if command.Option != "" && !uciCommandMatchesObservedInPlan(commandIndex, command, commands, sections) {
					formatted := redactUCICommand(services.PreviewCommands([]services.UciCommand{command})[0])
					if isDefaultDifference(command, sections) {
						result.Defaults = append(result.Defaults, formatted)
					} else {
						result.Missing = append(result.Missing, formatted)
					}
				}
			}
			if len(result.Missing) > 0 {
				result.Status = "DRIFT_RELEVANT"
			} else if len(result.Defaults) > 0 {
				result.Status = "DRIFT_DEFAULT"
			} else {
				result.Status = "MATCH"
			}
		}
		resultsByNamespace = append(resultsByNamespace, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"device_id": deviceID, "site_id": siteID, "role": role, "namespaces": resultsByNamespace, "read_only": true})
}

// ─── UCI Show Parser ─────────────────────────────────────────────────────────
// Parses `uci show <config>` output into a structured map of sections.
// Input format (programmable notation):
//   network.lan=interface
//   network.lan.proto='static'
//   network.lan.ipaddr='192.168.1.1'
//   network.@switch[0]=switch
//   network.@switch[0].name='switch0'

type UCISection struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Name    string                 `json:"name"`
	IsAnon  bool                   `json:"is_anon"`
	Options map[string]interface{} `json:"options"` // string or []string
}

func redactUCICommand(command string) string {
	for _, option := range []string{"key", "password", "secret", "private_key", "auth_secret", "auth_key"} {
		marker := "." + option + "="
		if idx := strings.Index(command, marker); idx >= 0 {
			return command[:idx+len(marker)] + "'<redacted>'"
		}
	}
	return command
}

func redactUCIRaw(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		for _, option := range []string{"key", "password", "secret", "private_key", "auth_secret", "auth_key"} {
			if strings.HasSuffix(parts[0], "."+option) {
				lines[i] = parts[0] + "='<redacted>'"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func uciCommandMatchesObserved(command services.UciCommand, sections []UCISection) bool {
	if command.Action == "ensure_host" {
		desiredMACs, err := services.ParseMACList(command.Option)
		if err != nil {
			return false
		}
		for _, section := range sections {
			if section.Type != "host" {
				continue
			}
			name, nameOK := section.Options["name"].(string)
			mac, macOK := section.Options["mac"]
			ip, ipOK := section.Options["ip"]
			if nameOK && name == command.Section && macOK && ipOK &&
				uciMACListsEqual(mac, desiredMACs) && uciOptionContains(ip, command.Value, false) {
				return true
			}
		}
		return false
	}

	if command.Action == "delete" {
		for _, section := range sections {
			if !uciSectionMatchesReference(command.Section, section, sections) {
				continue
			}
			if command.Option == "" {
				return false
			}
			_, present := section.Options[command.Option]
			return !present
		}
		return true
	}

	for _, section := range sections {
		if !uciSectionMatchesReference(command.Section, section, sections) {
			continue
		}
		value, ok := section.Options[command.Option]
		if !ok {
			continue
		}
		if uciCommandValueMatches(command.Action, value, command.Value) {
			return true
		}
	}
	return false
}

func uciCommandMatchesObservedInPlan(index int, command services.UciCommand, commands []services.UciCommand, sections []UCISection) bool {
	if command.Action == "delete" {
		if desired, ok := plannedListAfterDelete(index, command, commands); ok {
			observed, present := observedUCIOption(command.Section, command.Option, sections)
			return present && uciListEquals(observed, desired)
		}
	}
	return uciCommandMatchesObserved(command, sections)
}

func plannedListAfterDelete(index int, command services.UciCommand, commands []services.UciCommand) ([]string, bool) {
	var desired []string
	for _, later := range commands[index+1:] {
		if later.Config != command.Config || later.Section != command.Section || later.Option != command.Option {
			continue
		}
		switch later.Action {
		case "add_list":
			desired = append(desired, later.Value)
		case "delete", "set", "del_list":
			return nil, false
		}
	}
	return desired, len(desired) > 0
}

func observedUCIOption(sectionID, option string, sections []UCISection) (interface{}, bool) {
	for _, section := range sections {
		if !uciSectionMatchesReference(sectionID, section, sections) {
			continue
		}
		value, ok := section.Options[option]
		return value, ok
	}
	return nil, false
}

func uciSectionMatchesReference(reference string, section UCISection, sections []UCISection) bool {
	if reference == section.ID {
		return true
	}
	match := uciAnonymousSectionPattern.FindStringSubmatch(reference)
	if match == nil || section.Type != match[1] {
		return false
	}
	index, err := strconv.Atoi(match[2])
	if err != nil {
		return false
	}
	typeIndex := 0
	lastTypeIndex := -1
	sectionTypeIndex := -1
	for _, candidate := range sections {
		if candidate.Type != match[1] {
			continue
		}
		if candidate.ID == section.ID {
			sectionTypeIndex = typeIndex
		}
		lastTypeIndex = typeIndex
		typeIndex++
	}
	if sectionTypeIndex == -1 {
		return false
	}
	if index < 0 {
		return sectionTypeIndex == lastTypeIndex+index+1
	}
	return sectionTypeIndex == index
}

func uciListEquals(observed interface{}, desired []string) bool {
	observedValues, ok := uciOptionValues(observed)
	if !ok || len(observedValues) != len(desired) {
		return false
	}
	for i := range desired {
		if observedValues[i] != desired[i] {
			return false
		}
	}
	return true
}

func uciMACListsEqual(observed interface{}, desired []string) bool {
	observedValues, ok := uciOptionValues(observed)
	if !ok || len(observedValues) != len(desired) {
		return false
	}
	for i := range desired {
		if !strings.EqualFold(observedValues[i], desired[i]) {
			return false
		}
	}
	return true
}

func uciOptionValues(value interface{}) ([]string, bool) {
	switch value := value.(type) {
	case string:
		return []string{value}, true
	case []string:
		return value, true
	default:
		return nil, false
	}
}

func uciCommandValueMatches(action string, observed interface{}, want string) bool {
	if action == "add_list" {
		return uciOptionContains(observed, want, false)
	}
	if action == "del_list" {
		return !uciOptionContains(observed, want, false)
	}
	if values, ok := observed.([]string); ok {
		return len(values) == 1 && uciOptionContains(values, want, false)
	}
	return uciOptionContains(observed, want, false)
}

func uciOptionContains(value interface{}, want string, caseInsensitive bool) bool {
	match := func(candidate string) bool {
		if caseInsensitive {
			return strings.EqualFold(candidate, want)
		}
		return candidate == want
	}
	switch value := value.(type) {
	case string:
		return match(value)
	case []string:
		for _, candidate := range value {
			if match(candidate) {
				return true
			}
		}
	}
	return false
}

func isDefaultDifference(command services.UciCommand, _ []UCISection) bool {
	// OpenWrt treats an omitted disabled option as enabled. Requiring an
	// explicit disabled=0 would create noise for otherwise equivalent radios.
	if command.Config == "wireless" && command.Option == "disabled" && command.Value == "0" {
		return true
	}
	return false
}

func parseUciShow(raw string, config string) []UCISection {
	sectionMap := map[string]*UCISection{}
	var order []string

	prefix := config + "."

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) {
			continue
		}

		// Remove config prefix
		rest := line[len(prefix):]

		eqIdx := strings.Index(rest, "=")
		if eqIdx == -1 {
			continue
		}

		keyPart := rest[:eqIdx]
		value := parseUCIValue(rest[eqIdx+1:])

		dotIdx := strings.Index(keyPart, ".")
		if dotIdx == -1 {
			// Section declaration: lan=interface or @switch[0]=switch
			sectionID := keyPart
			isAnon := strings.HasPrefix(sectionID, "@")
			name := sectionID
			if !isAnon {
				name = sectionID
			}

			if _, exists := sectionMap[sectionID]; !exists {
				sectionType, _ := value.(string)
				sectionMap[sectionID] = &UCISection{
					ID:      sectionID,
					Type:    sectionType,
					Name:    name,
					IsAnon:  isAnon,
					Options: map[string]interface{}{},
				}
				order = append(order, sectionID)
			} else {
				if sectionType, ok := value.(string); ok {
					sectionMap[sectionID].Type = sectionType
				}
			}
		} else {
			// Option: lan.proto='static' or @switch[0].name='switch0'
			sectionID := keyPart[:dotIdx]
			optKey := keyPart[dotIdx+1:]

			// Ensure section exists
			if _, exists := sectionMap[sectionID]; !exists {
				isAnon := strings.HasPrefix(sectionID, "@")
				sectionMap[sectionID] = &UCISection{
					ID:      sectionID,
					Name:    sectionID,
					IsAnon:  isAnon,
					Options: map[string]interface{}{},
				}
				order = append(order, sectionID)
			}

			sec := sectionMap[sectionID]

			// Handle list values: if the value contains space-separated items
			// from `uci show`, it's a list representation
			// Actually uci show renders lists as multiple lines with same key,
			// so we detect duplication:
			existing, exists := sec.Options[optKey]
			if !exists {
				sec.Options[optKey] = value
				continue
			}
			sec.Options[optKey] = mergeUCIValues(existing, value)
		}
	}

	result := make([]UCISection, 0, len(order))
	for _, id := range order {
		result = append(result, *sectionMap[id])
	}
	return result
}

func parseUCIValue(raw string) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '\'' {
		return strings.Trim(raw, "'")
	}

	values := make([]string, 0, 1)
	for raw != "" {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw[0] != '\'' {
			break
		}
		raw = raw[1:]
		var value strings.Builder
		for raw != "" {
			if strings.HasPrefix(raw, "'\\''") {
				value.WriteByte('\'')
				raw = raw[4:]
				continue
			}
			if raw[0] == '\'' {
				raw = raw[1:]
				break
			}
			value.WriteByte(raw[0])
			raw = raw[1:]
		}
		values = append(values, value.String())
	}
	if len(values) == 1 {
		return values[0]
	}
	return values
}

func mergeUCIValues(existing, next interface{}) interface{} {
	values := make([]string, 0, 2)
	for _, value := range []interface{}{existing, next} {
		switch value := value.(type) {
		case string:
			values = append(values, value)
		case []string:
			values = append(values, value...)
		}
	}
	if len(values) == 1 {
		return values[0]
	}
	return values
}
