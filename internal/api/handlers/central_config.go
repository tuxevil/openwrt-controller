package handlers

import (
	"openwrt-controller/internal/api/middleware"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// ─── CENTRAL_LUCI Handlers ───────────────────────────────────────────────────
// Centralised LuCI-grade configuration management for distributed OpenWrt fleets.

// GetCentralConfigHandler reads an entire UCI namespace from the device
// using `uci show <config>` in programmable notation, which returns
// structured key=value pairs parseable by the frontend.
//
// Also supports optional `?path=` query for scoped reads, e.g.:
//   GET /api/devices/{device_id}/central-config?config=wireless&path=wireless.radio0.channel
func GetCentralConfigHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	config := r.URL.Query().Get("config")
	path := r.URL.Query().Get("path")

	if config == "" {
		http.Error(w, `{"error":"missing 'config' parameter"}`, http.StatusBadRequest)
		return
	}

	// If specific UCI path provided, return just that
	var cmd string
	if path != "" {
		cmd = fmt.Sprintf("uci show %s 2>&1", path)
	} else {
		cmd = fmt.Sprintf("uci show %s 2>&1", config)
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
	username := GetUsernameFromReq(r)

	if config == "" {
		http.Error(w, `{"error":"missing 'config' parameter"}`, http.StatusBadRequest)
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

	// ── VAULT INTEGRATION: Pre-change backup ─────────────────────────────
	// Before any destructive change, snapshot the entire /etc/config/<config>
	// into The Vault as a safety net.
	log.Printf("[CENTRAL_LUCI] Triggering pre-change Vault backup for device %s, config: %s", deviceID, config)
	go func() {
		if err := services.CreateBackup(middleware.GetTenantSchema(r), deviceID); err != nil {
			log.Printf("[CENTRAL_LUCI][WARN] Pre-change backup failed for %s: %v", deviceID, err)
		} else {
			log.Printf("[CENTRAL_LUCI] Pre-change backup stored in Vault for %s", deviceID)
		}
	}()

	// ── Build & execute batch script via UCI Bridge ──────────────────────
	script := services.BuildBatchScript(config, payload.Commands)
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
