package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

type TailscaleDeviceStatus struct {
	DeviceID     string   `json:"device_id"`
	DeviceName   string   `json:"device_name"`
	Model        string   `json:"model"`
	Installed    bool     `json:"installed"`
	BackendState string   `json:"backend_state"`
	TailscaleIPs []string `json:"tailscale_ips"`
	DNSName      string   `json:"dns_name"`
	Online       bool     `json:"online"`
	ExitNode     bool     `json:"exit_node"`
	RxBytes      int64    `json:"rx_bytes"`
	TxBytes      int64    `json:"tx_bytes"`
	LastSeenAt   string   `json:"last_seen_at"`
}

type TailscalePlanRequest struct {
	AuthKey         string   `json:"auth_key"`
	AdvertiseRoutes []string `json:"advertise_routes"`
	IsExitNode      bool     `json:"is_exit_node"`
	AcceptDNS       bool     `json:"accept_dns"`
	WatchdogTTL     int      `json:"watchdog_ttl"`
}

type TailscalePlanResponse struct {
	DeviceID        string   `json:"device_id"`
	PlanID          string   `json:"plan_id"`
	WatchdogTTL     int      `json:"watchdog_ttl"`
	AdvertiseRoutes []string `json:"advertise_routes"`
	IsExitNode      bool     `json:"is_exit_node"`
	UCICommands     []string `json:"uci_commands"`
	TailscaleCmd    string   `json:"tailscale_cmd"`
	SafetyNotice    string   `json:"safety_notice"`
}

// GetSiteTailscaleHandler aggregates Tailscale status from device telemetry
func GetSiteTailscaleHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"missing site_id"}`, http.StatusBadRequest)
		return
	}

	rows, err := database.Tx(r.Context()).Query(
		`SELECT id, COALESCE(name, id), COALESCE(model, 'Unknown'), state_json, COALESCE(last_seen_at::text, '') 
		 FROM devices WHERE site_id = $1 ORDER BY name ASC`,
		siteID,
	)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []TailscaleDeviceStatus
	for rows.Next() {
		var id, name, model, lastSeen string
		var stateJSON sql.NullString
		if err := rows.Scan(&id, &name, &model, &stateJSON, &lastSeen); err != nil {
			continue
		}

		status := TailscaleDeviceStatus{
			DeviceID:     id,
			DeviceName:   name,
			Model:        model,
			Installed:    false,
			BackendState: "Not Installed",
			TailscaleIPs: []string{},
			DNSName:      "",
			Online:       false,
			ExitNode:     false,
			RxBytes:      0,
			TxBytes:      0,
			LastSeenAt:   lastSeen,
		}

		if stateJSON.Valid && stateJSON.String != "" {
			var state map[string]interface{}
			if err := json.Unmarshal([]byte(stateJSON.String), &state); err == nil {
				// Check installed packages in capabilities
				if caps, ok := state["capabilities"].(map[string]interface{}); ok {
					if pkgs, ok := caps["packages"].([]interface{}); ok {
						for _, p := range pkgs {
							if pStr, ok := p.(string); ok && pStr == "tailscale" {
								status.Installed = true
								status.BackendState = "Stopped"
								break
							}
						}
					}
				}

				// Check telemetry tailscale block
				if ts, ok := state["tailscale"].(map[string]interface{}); ok && len(ts) > 0 {
					status.Installed = true
					if bs, ok := ts["backend_state"].(string); ok && bs != "" {
						status.BackendState = bs
					}
					if dns, ok := ts["dns_name"].(string); ok {
						status.DNSName = dns
					}
					if onl, ok := ts["online"].(bool); ok {
						status.Online = onl
					}
					if en, ok := ts["exit_node"].(bool); ok {
						status.ExitNode = en
					}
					if rx, ok := ts["rx_bytes"].(float64); ok {
						status.RxBytes = int64(rx)
					}
					if tx, ok := ts["tx_bytes"].(float64); ok {
						status.TxBytes = int64(tx)
					}
					if ips, ok := ts["tailscale_ips"].([]interface{}); ok {
						for _, ip := range ips {
							if s, ok := ip.(string); ok {
								status.TailscaleIPs = append(status.TailscaleIPs, s)
							}
						}
					}
				}
			}
		}

		results = append(results, status)
	}

	if results == nil {
		results = []TailscaleDeviceStatus{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

var safeAuthKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.:]{0,128}$`)

// GenerateTailscalePlanHandler validates arguments and renders a commit-confirmed plan
func GenerateTailscalePlanHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error":"missing device_id"}`, http.StatusBadRequest)
		return
	}

	var req TailscalePlanRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	req.AuthKey = strings.TrimSpace(req.AuthKey)
	if req.AuthKey != "" && !safeAuthKeyPattern.MatchString(req.AuthKey) {
		http.Error(w, `{"error":"auth_key contains invalid characters"}`, http.StatusBadRequest)
		return
	}

	// Validate advertised routes as CIDRs
	var validRoutes []string
	for _, route := range req.AdvertiseRoutes {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}
		_, _, err := net.ParseCIDR(route)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid CIDR route: %s"}`, route), http.StatusBadRequest)
			return
		}
		validRoutes = append(validRoutes, route)
	}

	ttl := req.WatchdogTTL
	if ttl < 30 {
		ttl = 120 // default safe window
	} else if ttl > 1800 {
		ttl = 1800
	}

	planID := fmt.Sprintf("ts-%d", time.Now().Unix())

	// Build UCI network and firewall commands
	uciCommands := []string{
		"uci set network.tailscale=interface",
		"uci set network.tailscale.proto=none",
		"uci set network.tailscale.device=tailscale0",
		"uci commit network",
		"uci set firewall.tailscale=zone",
		"uci set firewall.tailscale.name=tailscale",
		"uci add_list firewall.tailscale.network=tailscale",
		"uci set firewall.tailscale.input=ACCEPT",
		"uci set firewall.tailscale.output=ACCEPT",
		"uci set firewall.tailscale.forward=ACCEPT",
		"uci set firewall.tailscale.masq=0",
		"uci add firewall forwarding",
		"uci set firewall.@forwarding[-1].src=tailscale",
		"uci set firewall.@forwarding[-1].dest=lan",
		"uci add firewall forwarding",
		"uci set firewall.@forwarding[-1].src=lan",
		"uci set firewall.@forwarding[-1].dest=tailscale",
		"uci commit firewall",
	}

	// Build tailscale up command
	cmdParts := []string{"tailscale up"}
	if req.AuthKey != "" {
		cmdParts = append(cmdParts, fmt.Sprintf(`--authkey="%s"`, req.AuthKey))
	}
	if len(validRoutes) > 0 {
		cmdParts = append(cmdParts, fmt.Sprintf(`--advertise-routes=%s`, strings.Join(validRoutes, ",")))
	}
	if !req.AcceptDNS {
		cmdParts = append(cmdParts, "--accept-dns=false")
	}
	if req.IsExitNode {
		cmdParts = append(cmdParts, "--advertise-exit-node")
	}

	tailscaleCmd := strings.Join(cmdParts, " ")

	resp := TailscalePlanResponse{
		DeviceID:        deviceID,
		PlanID:          planID,
		WatchdogTTL:     ttl,
		AdvertiseRoutes: validRoutes,
		IsExitNode:      req.IsExitNode,
		UCICommands:     uciCommands,
		TailscaleCmd:    tailscaleCmd,
		SafetyNotice:    fmt.Sprintf("Plan armed with %d second commit-confirmed watchdog. If confirmation is not received or health-check pings fail, all changes automatically revert.", ttl),
	}

	// Insert audit log
	remoteIP, _, splitErr := net.SplitHostPort(r.RemoteAddr)
	if splitErr != nil {
		remoteIP = r.RemoteAddr
	}
	database.InsertAuditLog(
		GetUsernameFromReq(r),
		"TAILSCALE_PLAN_GENERATED",
		"DEVICE",
		deviceID,
		fmt.Sprintf("Generated plan %s with TTL %ds and %d routes", planID, ttl, len(validRoutes)),
		remoteIP,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
