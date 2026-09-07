package services

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var healthTargetPattern = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)

// ValidateHealthTargets accepts IPv4/IPv6 addresses and DNS-like hostnames.
// Shell quoting is still applied by BuildSafeBatchScript at the execution seam.
func ValidateHealthTargets(targets []string) ([]string, error) {
	valid := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target == "" || (net.ParseIP(target) == nil && !healthTargetPattern.MatchString(target)) {
			return nil, fmt.Errorf("invalid health check target")
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		valid = append(valid, target)
	}
	return valid, nil
}

// ─── UCI Bridge ──────────────────────────────────────────────────────────────
// Translation engine for OpenWrt UCI commands.
// Based on: docs/uci/uci.md — Unified Configuration Interface reference.
// Generates shell-safe UCI batch scripts for atomic SSH execution.

// UciCommand represents a single UCI mutation.
type UciCommand struct {
	Action  string `json:"action"`  // "set", "delete", "add_list", "del_list", "add", "ensure_host", "rename", "reorder"
	Config  string `json:"config"`  // namespace: "network", "wireless", "firewall", etc.
	Section string `json:"section"` // section name or @type[N] anonymous ref
	Option  string `json:"option"`  // option key (empty for section-level ops)
	Value   string `json:"value"`   // value to set (empty for delete)
}

var (
	uciNamePattern       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	uciSectionPattern    = regexp.MustCompile(`^(?:[A-Za-z0-9_-]+|@[A-Za-z0-9_-]+(?:\[-?[0-9]+\])?)$`)
	uciMACAddressPattern = regexp.MustCompile(`(?i)(?:[0-9a-f]{2}:){5}[0-9a-f]{2}`)
)

// ParseMACList accepts a single MAC or the UCI list representation stored by
// the importer, for example "AA:...:01' 'AA:...:02".
func ParseMACList(raw string) ([]string, error) {
	macs := uciMACAddressPattern.FindAllString(raw, -1)
	if len(macs) == 0 {
		return nil, fmt.Errorf("invalid MAC address list")
	}
	remainder := uciMACAddressPattern.ReplaceAllString(raw, "")
	remainder = strings.NewReplacer("'", "", `"`, "", " ", "", "\t", "", "\r", "", "\n", "").Replace(remainder)
	if remainder != "" {
		return nil, fmt.Errorf("invalid MAC address list")
	}
	result := make([]string, 0, len(macs))
	seen := make(map[string]struct{}, len(macs))
	for _, mac := range macs {
		mac = strings.ToUpper(mac)
		if _, ok := seen[mac]; ok {
			continue
		}
		seen[mac] = struct{}{}
		result = append(result, mac)
	}
	return result, nil
}

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

// Delete generates an idempotent delete for a section. OpenWrt's -q flag
// suppresses the missing-entry message but still returns a non-zero status.
// Ref: uci.md — "Delete the given section or option"
func Delete(config, section string) string {
	if !validUCIName(config) || !validUCISection(section) {
		return ""
	}
	return fmt.Sprintf("uci -q delete %s.%s || true", config, section)
}

// DeleteOption generates an idempotent delete for an option.
func DeleteOption(config, section, option string) string {
	if !validUCIName(config) || !validUCISection(section) || !validUCIName(option) {
		return ""
	}
	return fmt.Sprintf("uci -q delete %s.%s.%s || true", config, section, option)
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
	if !validUCIName(config) || !validUCISection(section) || (option != "" && !validUCIName(option)) || !validUCIName(newName) {
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
//  1. Snapshots the raw config file for rollback
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

	script := fmt.Sprintf(`#!/bin/sh
set -e

# ──────────────────────────────────────────────────────────────
# CENTRAL_LUCI — Atomic UCI batch push (Nerve Center)
# Config namespace: %s
# ──────────────────────────────────────────────────────────────

logger -t central_luci "CENTRAL_LUCI: starting batch push for '%s'"

# Phase 1: Snapshot current state for rollback
backup_exists=0
if [ -f /etc/config/%s ]; then
  cp /etc/config/%s /tmp/central_luci_bak_%s.conf
  backup_exists=1
else
  : > /tmp/central_luci_bak_%s.conf
fi

	rollback() {
  logger -t central_luci "CENTRAL_LUCI: ROLLBACK — restoring '%s' from snapshot"
  uci revert %s 2>/dev/null || true
  if [ "$backup_exists" -eq 1 ]; then
    cp /tmp/central_luci_bak_%s.conf /etc/config/%s
  else
    rm -f /etc/config/%s
  fi
  uci revert %s 2>/dev/null || true
  %s || true
  rm -f /tmp/central_luci_bak_%s.conf
  exit 1
}

trap 'status=$?; echo "CENTRAL_LUCI: command failed at line $LINENO with status $status" >&2; rollback' ERR

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
`, config, config, config, config, config, config,
		config, config, config, config, config, config,
		restartCmd, config,
		sb.String(), config, config, config,
		restartCmd, config, config)
	return withUciMutationLock(script)
}

// withUciMutationLock serializes controller-generated UCI batches on a device.
// mkdir is available in the base OpenWrt shell and is atomic across SSH
// sessions, so the observed-state check and mutation phase share one lock.
func withUciMutationLock(script string) string {
	const marker = "set -e\n"
	const lock = `
uci_lock_dir=/tmp/central_luci_uci.lock
uci_lock_acquired=0
release_uci_lock() {
  if [ "$uci_lock_acquired" -eq 1 ]; then
    rmdir "$uci_lock_dir" 2>/dev/null || true
    uci_lock_acquired=0
  fi
}
trap release_uci_lock EXIT

uci_lock_attempt=0
while ! mkdir "$uci_lock_dir" 2>/dev/null; do
  uci_lock_mtime=$(stat -c %Y "$uci_lock_dir" 2>/dev/null || printf '0')
  uci_lock_now=$(date +%s)
  if [ "$uci_lock_mtime" -gt 0 ] && [ "$uci_lock_now" -gt "$uci_lock_mtime" ] && [ "$((uci_lock_now - uci_lock_mtime))" -ge 1800 ]; then
    rmdir "$uci_lock_dir" 2>/dev/null || true
    continue
  fi
  uci_lock_attempt=$((uci_lock_attempt + 1))
  if [ "$uci_lock_attempt" -ge 60 ]; then
    printf 'CENTRAL_LUCI: timed out waiting for UCI lock\n' >&2
    exit 1
  fi
  sleep 1
done
uci_lock_acquired=1
`
	index := strings.Index(script, marker)
	if index == -1 {
		return script
	}
	index += len(marker)
	return script[:index] + lock + script[index:]
}

// BuildSafeBatchScript adds a post-apply connectivity check to the normal
// rollback-protected batch before the backup is removed.
func BuildSafeBatchScript(config string, commands []UciCommand, healthTargets []string) string {
	script := BuildBatchScript(config, commands)
	if script == "" {
		return ""
	}
	if len(healthTargets) == 0 {
		healthTargets = []string{"1.1.1.1"}
	}
	var healthCheck strings.Builder
	healthCheck.WriteString("\n")
	for _, target := range healthTargets {
		healthCheck.WriteString(fmt.Sprintf("health_target=%s\n", shellQuote(target)))
		healthCheck.WriteString("health_ok=0\n")
		healthCheck.WriteString("for health_attempt in 1 2 3; do\n")
		healthCheck.WriteString(fmt.Sprintf("  if ping -c 1 -W 2 %s >/dev/null 2>&1; then\n", shellQuote(target)))
		healthCheck.WriteString("    health_ok=1\n")
		healthCheck.WriteString("    break\n")
		healthCheck.WriteString("  fi\n")
		healthCheck.WriteString("  sleep 1\n")
		healthCheck.WriteString("done\n")
		healthCheck.WriteString("if [ \"$health_ok\" -ne 1 ]; then\n")
		healthCheck.WriteString("  printf 'CENTRAL_LUCI: health check failed for target %s\\n' \"$health_target\" >&2\n")
		healthCheck.WriteString("  rollback\n")
		healthCheck.WriteString("fi\n")
	}
	healthCheck.WriteString("\n")
	const marker = "# Phase 5: Service restart\n"
	markerStart := strings.Index(script, marker)
	if markerStart == -1 {
		return script
	}
	restartStart := markerStart + len(marker)
	restartEnd := strings.IndexByte(script[restartStart:], '\n')
	if restartEnd == -1 {
		return script
	}
	restartEnd += restartStart
	return script[:restartEnd+1] + healthCheck.String() + script[restartEnd+1:]
}

// BuildDryRunScript validates the batch without committing or restarting a
// service. It is intended for a preview/validation pass before a rollout.
func BuildDryRunScript(config string, commands []UciCommand) string {
	if len(commands) == 0 || !validUCIName(config) {
		return ""
	}
	var sb strings.Builder
	for _, cmd := range commands {
		if cmd.Config != config {
			return ""
		}
		line := translateCommand(cmd)
		if line == "" {
			return ""
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
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
	case "ensure_host":
		return ensureDHCPHost(cmd)
	case "delete_all":
		if !validUCIName(cmd.Config) || !validUCIName(cmd.Section) || cmd.Option != "" || cmd.Value != "" {
			return ""
		}
		return fmt.Sprintf("while uci -q delete %s.@%s[0]; do :; done", cmd.Config, cmd.Section)
	case "rename":
		return Rename(cmd.Config, cmd.Section, cmd.Option, cmd.Value)
	default:
		return ""
	}
}

// ensureDHCPHost renders an idempotent static lease mutation. Section is the
// friendly host name, Option is the MAC address, and Value is the IP address.
// Existing leases are updated by MAC (or IP as a fallback); only missing leases
// create a new anonymous host section.
func ensureDHCPHost(cmd UciCommand) string {
	parsedIP := net.ParseIP(cmd.Value)
	if cmd.Config != "dhcp" || cmd.Section == "" || len(cmd.Section) > 128 ||
		strings.ContainsAny(cmd.Section, "\r\n") || cmd.Option == "" || parsedIP == nil || parsedIP.To4() == nil {
		return ""
	}
	macs, err := ParseMACList(cmd.Option)
	if err != nil {
		return ""
	}
	name := shellQuote(cmd.Section)
	macList := shellQuote(strings.Join(macs, " "))
	ip := shellQuote(cmd.Value)
	return fmt.Sprintf(`host_ip=%s
host_name=%s
host_macs=%s
host_mac_count=%d
host_ref=
for host_mac in $host_macs; do
  host_ref=$(uci show dhcp | grep -i -F "$host_mac" | grep -F ".mac=" | cut -d= -f1 | cut -d. -f2 | head -n 1)
  if [ -n "$host_ref" ]; then
    break
  fi
done
if [ -z "$host_ref" ]; then
  host_ref=$(
    uci show dhcp | grep -F ".ip=" | while IFS= read -r host_line; do
      host_path=${host_line%%=*}
      host_value=${host_line#*=}
      host_value=$(printf '%%s' "$host_value" | sed "s/^'//; s/'$//")
      if [ "$host_value" = "$host_ip" ]; then
        host_path=${host_path#dhcp.}
        host_path=${host_path%%.ip}
        printf '%%s\n' "$host_path"
        break
      fi
    done
  )
fi
if [ -z "$host_ref" ]; then
  uci add dhcp host
  host_ref=@host[-1]
fi
uci set "dhcp.$host_ref.name=$host_name"
uci -q delete "dhcp.$host_ref.mac" || true
if [ "$host_mac_count" -eq 1 ]; then
  uci set "dhcp.$host_ref.mac=$host_macs"
else
  for host_mac in $host_macs; do
    uci add_list "dhcp.$host_ref.mac=$host_mac"
  done
fi
uci set "dhcp.$host_ref.ip=$host_ip"`, ip, name, macList, len(macs))
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
