package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

var uciPathSegmentPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_-]+|@[A-Za-z0-9_-]+(?:\[-?[0-9]+\])?)$`)

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

// PutCentralConfigHandler receives UciCommand structs, creates a Vault backup
// of the target config BEFORE applying, then executes the batch script
// with rollback protection.
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
		Commands []services.UciCommand `json:"commands"`
	}
	if !readBody(w, r, &payload) {
		return
	}

	if len(payload.Commands) == 0 {
		http.Error(w, `{"error":"empty command list"}`, http.StatusBadRequest)
		return
	}
	if dryRun {
		script := services.BuildDryRunScript(config, payload.Commands)
		if script == "" {
			http.Error(w, `{"error":"invalid UCI command or config"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "dry_run",
			"config":   config,
			"commands": services.PreviewCommands(payload.Commands),
		})
		return
	}

	// ── VAULT INTEGRATION: Pre-change backup ─────────────────────────────
	// Before any destructive change, snapshot the entire /etc/config/<config>
	// into The Vault as a safety net.
	log.Printf("[CENTRAL_LUCI] Triggering pre-change Vault backup for device %s, config: %s", deviceID, config)
	if err := services.CreateBackup(context.Background(), middleware.GetTenantSchema(r), deviceID); err != nil {
		log.Printf("[CENTRAL_LUCI][WARN] Pre-change backup failed for %s: %v", deviceID, err)
		http.Error(w, `{"error":"pre-change backup failed; configuration was not applied"}`, http.StatusServiceUnavailable)
		return
	}

	// ── Build & execute batch script via UCI Bridge ──────────────────────
	script := services.BuildBatchScript(config, payload.Commands)
	if script == "" {
		http.Error(w, `{"error":"invalid UCI command or config"}`, http.StatusBadRequest)
		return
	}
	out, err := runSSHScript(deviceID, script)

	if err != nil {
		// Report the exact UCI error from the remote binary
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		database.InsertAuditLog(username, "CENTRAL_LUCI_PUSH_FAILED", "DEVICE", deviceID,
			fmt.Sprintf("FAILED push %d commands to '%s': %s", len(payload.Commands), config, err.Error()), r.RemoteAddr)

		json.NewEncoder(w).Encode(map[string]string{
			"error":  err.Error(),
			"output": out,
		})
		return
	}

	database.InsertAuditLog(username, "CENTRAL_LUCI_PUSH", "DEVICE", deviceID,
		fmt.Sprintf("Pushed %d UCI commands to namespace: %s", len(payload.Commands), config), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"output": out,
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
		Commands []services.UciCommand `json:"commands"`
		Confirm  bool                  `json:"confirm"`
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

	plan := services.PreviewCommands(payload.Commands)
	if !payload.Confirm {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "preview",
			"device_id": deviceID,
			"config":    config,
			"commands":  plan,
			"next_step": "repeat with confirm=true to apply to this device",
		})
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
	_, _ = database.Tx(r.Context()).Exec("UPDATE "+schema+".devices SET last_rollout_status = 'RUNNING', last_rollout_at = CURRENT_TIMESTAMP WHERE id = $1", deviceID)

	script := services.BuildSafeBatchScript(config, payload.Commands, nil)
	output, err := runSSHScript(deviceID, script)
	if err != nil {
		_, _ = database.Tx(r.Context()).Exec("UPDATE "+schema+".devices SET last_rollout_status = 'FAILED', last_health_check_at = CURRENT_TIMESTAMP WHERE id = $1", deviceID)
		database.InsertAuditLog(GetUsernameFromReq(r), "SAFE_ROLLOUT_FAILED", "DEVICE", deviceID, err.Error(), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"status": "rolled_back_or_failed", "output": output, "error": err.Error()})
		return
	}

	database.InsertAuditLog(GetUsernameFromReq(r), "SAFE_ROLLOUT_APPLIED", "DEVICE", deviceID, fmt.Sprintf("Applied %d commands to %s", len(payload.Commands), config), r.RemoteAddr)
	_, _ = database.Tx(r.Context()).Exec("UPDATE "+schema+".devices SET last_rollout_status = 'SUCCESS', last_health_check_at = CURRENT_TIMESTAMP WHERE id = $1", deviceID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "output": output})
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
	if err := database.Tx(r.Context()).QueryRow("SELECT site_id, COALESCE(name, model, id), COALESCE(device_role, 'AP') FROM "+schema+".devices WHERE id = $1", deviceID).Scan(&siteID, &name, &role); err != nil {
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
			for _, command := range commands {
				if command.Option != "" && !uciCommandMatchesObserved(command, sections) {
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
	for _, section := range sections {
		value, ok := section.Options[command.Option]
		if !ok {
			continue
		}
		if valueString, ok := value.(string); ok && valueString == command.Value {
			return true
		}
		if values, ok := value.([]string); ok {
			for _, item := range values {
				if item == command.Value {
					return true
				}
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
		valPart := strings.Trim(rest[eqIdx+1:], "'")

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
				sectionMap[sectionID] = &UCISection{
					ID:      sectionID,
					Type:    valPart,
					Name:    name,
					IsAnon:  isAnon,
					Options: map[string]interface{}{},
				}
				order = append(order, sectionID)
			} else {
				sectionMap[sectionID].Type = valPart
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
			if exists {
				// Convert to list
				switch v := existing.(type) {
				case string:
					sec.Options[optKey] = []string{v, valPart}
				case []string:
					sec.Options[optKey] = append(v, valPart)
				}
			} else {
				sec.Options[optKey] = valPart
			}
		}
	}

	result := make([]UCISection, 0, len(order))
	for _, id := range order {
		result = append(result, *sectionMap[id])
	}
	return result
}
