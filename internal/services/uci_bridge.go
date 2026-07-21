package services

import (
	"fmt"
	"regexp"
	"strings"
)

// ─── UCI Bridge ──────────────────────────────────────────────────────────────
// Translation engine for OpenWrt UCI commands.
// Based on: docs/uci/uci.md — Unified Configuration Interface reference.
// Generates shell-safe UCI batch scripts for atomic SSH execution.

// UciCommand represents a single UCI mutation.
type UciCommand struct {
	Action  string `json:"action"`  // "set", "delete", "add_list", "del_list", "add", "rename", "reorder"
	Config  string `json:"config"`  // namespace: "network", "wireless", "firewall", etc.
	Section string `json:"section"` // section name or @type[N] anonymous ref
	Option  string `json:"option"`  // option key (empty for section-level ops)
	Value   string `json:"value"`   // value to set (empty for delete)
}

var (
	uciNamePattern    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	uciSectionPattern = regexp.MustCompile(`^(?:[A-Za-z0-9_-]+|@[A-Za-z0-9_-]+(?:\[-?[0-9]+\])?)$`)
)

func validUCIName(value string) bool {
	return uciNamePattern.MatchString(value)
}

func validUCISection(value string) bool {
	return uciSectionPattern.MatchString(value)
}

func validUCIPath(path, config string, minParts, maxParts int) bool {
	parts := strings.Split(path, ".")
	if len(parts) < minParts || len(parts) > maxParts || parts[0] != config {
		return false
	}
	for _, part := range parts[1:] {
		if !validUCISection(part) {
			return false
		}
	}
	return true
}

func validUCIValueToken(value string) bool {
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return !strings.ContainsAny(value[1:len(value)-1], "'\r\n")
	}
	return validUCIName(value)
}

// ValidRawUCICommand accepts only the small command grammar emitted by the
// legacy UCI editor. It deliberately rejects shell syntax instead of trying
// to parse arbitrary shell input.
func ValidRawUCICommand(command, config string) bool {
	if !validUCIName(config) || strings.TrimSpace(command) != command ||
		strings.ContainsAny(command, "\r\n;|&$`()<>\\") {
		return false
	}

	for _, prefix := range []string{"uci set ", "uci add_list ", "uci del_list "} {
		if strings.HasPrefix(command, prefix) {
			body := strings.TrimPrefix(command, prefix)
			path, value, ok := strings.Cut(body, "=")
			return ok && validUCIPath(path, config, 2, 3) && validUCIValueToken(value)
		}
	}

	for _, prefix := range []string{"uci delete ", "uci -q delete "} {
		if strings.HasPrefix(command, prefix) {
			return validUCIPath(strings.TrimPrefix(command, prefix), config, 2, 3)
		}
	}

	if strings.HasPrefix(command, "uci add ") {
		fields := strings.Fields(strings.TrimPrefix(command, "uci add "))
		return len(fields) == 2 && fields[0] == config && validUCIName(fields[1])
	}

	return false
}

// ServiceRestartMap is defined in uci_restart_map.go to keep a single
// source of truth shared with api/handlers/uci_ops.go.

// SetOption generates: uci set <config>.<section>.<option>='<value>'
// If option is empty, creates/types a section: uci set <config>.<section>=<value>
func SetOption(config, section, option, value string) string {
	if !validUCIName(config) || !validUCISection(section) || (option != "" && !validUCIName(option)) {
		return ""
	}
	if option == "" {
		// Section-level: set type — ref: uci.md "Creating a named section"
		// Example: uci set playapp.myname=mysectiontype
		return fmt.Sprintf("uci set %s.%s='%s'", config, section, escapeVal(value))
	}
	return fmt.Sprintf("uci set %s.%s.%s='%s'", config, section, option, escapeVal(value))
}

// AddList generates: uci add_list <config>.<section>.<option>='<value>'
// Ref: uci.md — "append an entry to a list"
func AddList(config, section, option, value string) string {
	if !validUCIName(config) || !validUCISection(section) || !validUCIName(option) {
		return ""
	}
	return fmt.Sprintf("uci add_list %s.%s.%s='%s'", config, section, option, escapeVal(value))
}

// DelList generates: uci del_list <config>.<section>.<option>='<value>'
func DelList(config, section, option, value string) string {
	if !validUCIName(config) || !validUCISection(section) || !validUCIName(option) {
		return ""
	}
	return fmt.Sprintf("uci del_list %s.%s.%s='%s'", config, section, option, escapeVal(value))
}

// Delete generates: uci delete <config>.<section>[.<option>]
// Ref: uci.md — "Delete the given section or option"
func Delete(config, section string) string {
	if !validUCIName(config) || !validUCISection(section) {
		return ""
	}
	return fmt.Sprintf("uci -q delete %s.%s", config, section)
}

// DeleteOption generates: uci delete <config>.<section>.<option>
func DeleteOption(config, section, option string) string {
	if !validUCIName(config) || !validUCISection(section) || !validUCIName(option) {
		return ""
	}
	return fmt.Sprintf("uci -q delete %s.%s.%s", config, section, option)
}

// AddAnonymousSection generates: uci add <config> <section-type>
// Returns the generated CFGID to stdout. Ref: uci.md "Add an anonymous section"
func AddAnonymousSection(config, sectionType string) string {
	if !validUCIName(config) || !validUCIName(sectionType) {
		return ""
	}
	return fmt.Sprintf("uci add %s %s", config, sectionType)
}

// Rename generates: uci rename <config>.<section>[.<option>]=<name>
func Rename(config, section, option, newName string) string {
	if !validUCIName(config) || !validUCISection(section) || (option != "" && !validUCIName(option)) {
		return ""
	}
	if option == "" {
		return fmt.Sprintf("uci rename %s.%s='%s'", config, section, escapeVal(newName))
	}
	return fmt.Sprintf("uci rename %s.%s.%s='%s'", config, section, option, escapeVal(newName))
}

// Reorder generates: uci reorder <config>.<section>=<position>
func Reorder(config, section string, position int) string {
	if !validUCIName(config) || !validUCISection(section) {
		return ""
	}
	return fmt.Sprintf("uci reorder %s.%s=%d", config, section, position)
}

// ─── Batch Builder ───────────────────────────────────────────────────────────

// BuildBatchScript takes a list of UciCommand structs and produces a single
// atomic shell script that:
//  1. Snapshots current config via `uci export`
//  2. Applies all mutations inside a trap-guarded block
//  3. Runs `uci commit`
//  4. Validates with `uci show`
//  5. Restarts the affected service
//  6. Rolls back on ANY failure
//
// This matches the "batch execution" paradigm from uci.md.
func BuildBatchScript(config string, commands []UciCommand) string {
	if !validUCIName(config) {
		return ""
	}

	var sb strings.Builder

	// Translate each UciCommand into a shell line
	for _, cmd := range commands {
		if cmd.Config != config {
			return ""
		}
		line := translateCommand(cmd)
		if line == "" {
			return ""
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	restartCmd := ""
	if svc, ok := ServiceRestartMap[config]; ok {
		restartCmd = svc + " && logger -t central_luci '" + config + " service restarted'"
	}

	return fmt.Sprintf(`#!/bin/sh
set -e

# ──────────────────────────────────────────────────────────────
# CENTRAL_LUCI — Atomic UCI batch push (Nerve Center)
# Config namespace: %s
# ──────────────────────────────────────────────────────────────

logger -t central_luci "CENTRAL_LUCI: starting batch push for '%s'"

# Phase 1: Snapshot current state for rollback
uci export %s > /tmp/central_luci_bak_%s.conf 2>/dev/null || true

rollback() {
  logger -t central_luci "CENTRAL_LUCI: ROLLBACK — restoring '%s' from snapshot"
  uci import %s < /tmp/central_luci_bak_%s.conf 2>/dev/null || true
  uci commit %s
  exit 1
}
trap rollback ERR

# Phase 2: Apply UCI mutations
%s
# Phase 3: Commit to flash
uci commit %s

# Phase 4: Syntax validation
uci show %s > /dev/null 2>&1 || {
  logger -t central_luci "CENTRAL_LUCI: VALIDATION FAILED for '%s'"
  rollback
}

# Phase 5: Service restart
%s

logger -t central_luci "CENTRAL_LUCI: batch push complete for '%s'"
rm -f /tmp/central_luci_bak_%s.conf
exit 0
`, config, config, config, config, config, config, config, config,
		sb.String(), config, config, config, restartCmd, config, config)
}

// translateCommand converts a UciCommand struct into its shell-safe UCI string.
func translateCommand(cmd UciCommand) string {
	switch cmd.Action {
	case "set":
		return SetOption(cmd.Config, cmd.Section, cmd.Option, cmd.Value)
	case "delete":
		if cmd.Option != "" {
			return DeleteOption(cmd.Config, cmd.Section, cmd.Option)
		}
		return Delete(cmd.Config, cmd.Section)
	case "add_list":
		return AddList(cmd.Config, cmd.Section, cmd.Option, cmd.Value)
	case "del_list":
		return DelList(cmd.Config, cmd.Section, cmd.Option, cmd.Value)
	case "add":
		return AddAnonymousSection(cmd.Config, cmd.Value) // value = section-type
	case "rename":
		return Rename(cmd.Config, cmd.Section, cmd.Option, cmd.Value)
	default:
		return ""
	}
}

// escapeVal prevents single-quote injection in UCI values.
func escapeVal(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// shellQuote returns one POSIX shell word containing s. Values that cross the
// SSH boundary must use this helper; fmt.Sprintf alone is not a shell escape.
func shellQuote(s string) string {
	return "'" + escapeVal(s) + "'"
}

// PreviewCommands returns the list of shell-safe UCI command strings
// WITHOUT the batch wrapper — for the UI "command preview" feature.
func PreviewCommands(commands []UciCommand) []string {
	var result []string
	for _, cmd := range commands {
		line := translateCommand(cmd)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
