package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"openwrt-controller/internal/database"
)

const (
	sentinelAdGuardMaxResponseBytes = 4 << 20 // 4 MiB
	sentinelAdGuardDefaultTopLimit  = 15
	sentinelAdGuardDefaultLogLimit  = 30
)

var sentinelAdGuardAllowedPaths = map[string]bool{
	"/control/status":           true,
	"/control/stats":            true,
	"/control/filtering/status": true,
	"/control/querylog":         true,
}

var sentinelAdGuardAllowedResponseStatuses = map[string]bool{
	"all":                  true,
	"filtered":             true,
	"blocked":              true,
	"blocked_safebrowsing": true,
	"blocked_parental":     true,
	"whitelisted":          true,
	"rewritten":            true,
	"safe_search":          true,
	"processed":            true,
}

type sentinelAdGuardConfig struct {
	BaseURL  string
	Username string
	Password string
}

func loadSentinelAdGuardConfig() (sentinelAdGuardConfig, error) {
	rawURL := strings.TrimSpace(os.Getenv("SENTINEL_ADGUARD_URL"))
	if rawURL == "" {
		return sentinelAdGuardConfig{}, fmt.Errorf("AdGuardHome API is not configured (set SENTINEL_ADGUARD_URL)")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return sentinelAdGuardConfig{}, fmt.Errorf("invalid SENTINEL_ADGUARD_URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return sentinelAdGuardConfig{}, fmt.Errorf("SENTINEL_ADGUARD_URL must use http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return sentinelAdGuardConfig{}, fmt.Errorf("SENTINEL_ADGUARD_URL is missing host")
	}
	if strings.ToLower(host) != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !isPrivateOrLocalIP(ip) {
			return sentinelAdGuardConfig{}, fmt.Errorf("SENTINEL_ADGUARD_URL must target a private LAN or loopback IP")
		}
	}
	base := strings.TrimRight(parsed.Scheme+"://"+parsed.Host+parsed.Path, "/")
	return sentinelAdGuardConfig{
		BaseURL:  base,
		Username: strings.TrimSpace(os.Getenv("SENTINEL_ADGUARD_USERNAME")),
		Password: os.Getenv("SENTINEL_ADGUARD_PASSWORD"),
	}, nil
}

func isSentinelAdGuardConfigured() bool {
	_, err := loadSentinelAdGuardConfig()
	return err == nil
}

// sentinelAdGuardGetJSON performs a strictly read-only HTTP GET against an
// allowlisted AdGuardHome control endpoint without following redirects.
func sentinelAdGuardGetJSON(ctx context.Context, path string, query url.Values, out interface{}) error {
	if !sentinelAdGuardAllowedPaths[path] {
		return fmt.Errorf("AdGuardHome path %q is not allowlisted", path)
	}
	cfg, err := loadSentinelAdGuardConfig()
	if err != nil {
		return err
	}
	endpoint := cfg.BaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("AdGuardHome request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, sentinelAdGuardMaxResponseBytes))
	if err != nil {
		return fmt.Errorf("AdGuardHome response read failed: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("AdGuardHome authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AdGuardHome returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("AdGuardHome response decode failed: %w", err)
	}
	return nil
}

type adguardStatusResponse struct {
	Version           string   `json:"version"`
	Running           bool     `json:"running"`
	ProtectionEnabled bool     `json:"protection_enabled"`
	DNSPort           int      `json:"dns_port"`
	DNSAddresses      []string `json:"dns_addresses"`
}

type adguardStatsResponse struct {
	TimeUnits               string               `json:"time_units"`
	NumDNSQueries           int64                `json:"num_dns_queries"`
	NumBlockedFiltering     int64                `json:"num_blocked_filtering"`
	NumReplacedSafebrowsing int64                `json:"num_replaced_safebrowsing"`
	NumReplacedParental     int64                `json:"num_replaced_parental"`
	AvgProcessingTime       float64              `json:"avg_processing_time"`
	TopQueriedDomains       []map[string]int64   `json:"top_queried_domains"`
	TopBlockedDomains       []map[string]int64   `json:"top_blocked_domains"`
	TopClients              []map[string]int64   `json:"top_clients"`
	TopUpstreamsResponses   []map[string]int64   `json:"top_upstreams_responses"`
	TopUpstreamsAvgTime     []map[string]float64 `json:"top_upstreams_avg_time"`
}

type adguardFilteringStatusResponse struct {
	Enabled   bool `json:"enabled"`
	UserRules []string `json:"user_rules"`
	Filters   []struct {
		Name        string `json:"name"`
		Enabled     bool   `json:"enabled"`
		RulesCount  int    `json:"rules_count"`
		LastUpdated string `json:"last_updated"`
	} `json:"filters"`
}

type adguardQueryLogResponse struct {
	Data []struct {
		Time       string `json:"time"`
		Client     string `json:"client"`
		ClientInfo struct {
			Name string `json:"name"`
		} `json:"client_info"`
		Question struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"question"`
		Status    string `json:"status"`
		Reason    string `json:"reason"`
		ElapsedMs string `json:"elapsedMs"`
		Upstream  string `json:"upstream"`
		Cached    bool   `json:"cached"`
		Rules     []struct {
			Text         string `json:"text"`
			FilterListID int64  `json:"filter_list_id"`
		} `json:"rules"`
	} `json:"data"`
}

func loadSentinelAdGuardHostnames(ctx context.Context, schema, siteID string) (map[string]string, map[string]string) {
	ipToHost := map[string]string{}
	hostToIP := map[string]string{}
	if database.DB == nil || strings.TrimSpace(schema) == "" || strings.TrimSpace(siteID) == "" {
		return ipToHost, hostToIP
	}
	if _, lookup, err := loadSentinelSiteDevices(ctx, schema, siteID); err == nil {
		for ip, host := range lookup.ByIP {
			if ip != "" && host != "" {
				ipToHost[ip] = host
				hostToIP[strings.ToLower(host)] = ip
			}
		}
	}
	if dir, err := database.LoadNetworkIdentityDirectory(schema, siteID); err == nil {
		for _, identity := range dir {
			ip := strings.TrimSpace(identity.IP)
			if ip == "" {
				continue
			}
			label := strings.TrimSpace(identity.Label)
			if label == "" {
				label = strings.TrimSpace(identity.Hostname)
			}
			if label == "" {
				continue
			}
			if _, exists := ipToHost[ip]; !exists {
				ipToHost[ip] = label
			}
			if identity.Label != "" {
				hostToIP[strings.ToLower(strings.TrimSpace(identity.Label))] = ip
			}
			if identity.Hostname != "" {
				hostToIP[strings.ToLower(strings.TrimSpace(identity.Hostname))] = ip
			}
		}
	}
	return ipToHost, hostToIP
}

func flattenAdGuardSinglePairInt(items []map[string]int64, keyField, valField string, limit int) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, entry := range items {
		if len(out) >= limit {
			break
		}
		for k, v := range entry {
			out = append(out, map[string]interface{}{
				keyField: k,
				valField: v,
			})
			break
		}
	}
	return out
}

func sentinelGetAdGuardStatsTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	topLimit := sentinelAdGuardDefaultTopLimit
	if raw, ok := invocation.Args["top_limit"].(float64); ok && raw >= 1 && raw <= 50 {
		topLimit = int(raw)
	}

	var status adguardStatusResponse
	if err := sentinelAdGuardGetJSON(ctx, "/control/status", nil, &status); err != nil {
		return nil, err
	}
	var stats adguardStatsResponse
	if err := sentinelAdGuardGetJSON(ctx, "/control/stats", nil, &stats); err != nil {
		return nil, err
	}
	var filtering adguardFilteringStatusResponse
	_ = sentinelAdGuardGetJSON(ctx, "/control/filtering/status", nil, &filtering)

	ipToHost, _ := loadSentinelAdGuardHostnames(ctx, invocation.Schema, siteID)

	topClients := make([]map[string]interface{}, 0, topLimit)
	for _, entry := range stats.TopClients {
		if len(topClients) >= topLimit {
			break
		}
		for clientIP, count := range entry {
			row := map[string]interface{}{
				"client_ip": clientIP,
				"queries":   count,
			}
			if host := ipToHost[clientIP]; host != "" {
				row["hostname"] = host
			}
			topClients = append(topClients, row)
			break
		}
	}

	upstreamAvg := map[string]float64{}
	for _, entry := range stats.TopUpstreamsAvgTime {
		for k, v := range entry {
			upstreamAvg[k] = mathRound2(v * 1000)
		}
	}
	topUpstreams := make([]map[string]interface{}, 0, len(stats.TopUpstreamsResponses))
	for _, entry := range stats.TopUpstreamsResponses {
		for upstream, count := range entry {
			row := map[string]interface{}{
				"upstream":  upstream,
				"responses": count,
			}
			if ms, ok := upstreamAvg[upstream]; ok {
				row["avg_ms"] = ms
			}
			topUpstreams = append(topUpstreams, row)
			break
		}
	}

	filterLists := make([]map[string]interface{}, 0, len(filtering.Filters))
	for _, f := range filtering.Filters {
		filterLists = append(filterLists, map[string]interface{}{
			"name":         f.Name,
			"enabled":      f.Enabled,
			"rules_count":  f.RulesCount,
			"last_updated": f.LastUpdated,
		})
	}

	blockedPct := 0.0
	if stats.NumDNSQueries > 0 {
		blockedPct = mathRound2(float64(stats.NumBlockedFiltering) * 100.0 / float64(stats.NumDNSQueries))
	}

	return map[string]interface{}{
		"version":                   status.Version,
		"running":                   status.Running,
		"protection_enabled":        status.ProtectionEnabled,
		"dns_port":                  status.DNSPort,
		"filtering_enabled":         filtering.Enabled,
		"filter_lists":              filterLists,
		"user_rules_count":          len(filtering.UserRules),
		"time_units":                stats.TimeUnits,
		"num_dns_queries":           stats.NumDNSQueries,
		"num_blocked_filtering":     stats.NumBlockedFiltering,
		"blocked_percent":           blockedPct,
		"num_replaced_safebrowsing": stats.NumReplacedSafebrowsing,
		"num_replaced_parental":     stats.NumReplacedParental,
		"avg_processing_time_ms":    mathRound2(stats.AvgProcessingTime * 1000),
		"top_clients":               topClients,
		"top_queried_domains":       flattenAdGuardSinglePairInt(stats.TopQueriedDomains, "domain", "queries", topLimit),
		"top_blocked_domains":       flattenAdGuardSinglePairInt(stats.TopBlockedDomains, "domain", "blocked_count", topLimit),
		"top_upstreams":             topUpstreams,
	}, nil
}

func sentinelGetAdGuardQueryLogTool(ctx context.Context, invocation sentinelToolInvocation) (interface{}, error) {
	siteID, _ := invocation.Args["site_id"].(string)
	searchRaw := strings.TrimSpace(stringField(invocation.Args, "search"))
	responseStatus := strings.ToLower(strings.TrimSpace(stringField(invocation.Args, "response_status")))
	if responseStatus == "" {
		responseStatus = "all"
	}
	if !sentinelAdGuardAllowedResponseStatuses[responseStatus] {
		return nil, fmt.Errorf("invalid response_status %q (use all, blocked, filtered, whitelisted, rewritten, or processed)", responseStatus)
	}
	limit := sentinelAdGuardDefaultLogLimit
	if raw, ok := invocation.Args["limit"].(float64); ok && raw >= 1 && raw <= 100 {
		limit = int(raw)
	}

	ipToHost, hostToIP := loadSentinelAdGuardHostnames(ctx, invocation.Schema, siteID)

	effectiveSearch := searchRaw
	resolvedSearchIP := ""
	if searchRaw != "" && net.ParseIP(searchRaw) == nil {
		if ip := hostToIP[strings.ToLower(searchRaw)]; ip != "" {
			effectiveSearch = ip
			resolvedSearchIP = ip
		}
	}

	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))
	params.Set("response_status", responseStatus)
	if effectiveSearch != "" {
		params.Set("search", effectiveSearch)
	}

	var rawLog adguardQueryLogResponse
	if err := sentinelAdGuardGetJSON(ctx, "/control/querylog", params, &rawLog); err != nil {
		return nil, err
	}

	entries := make([]map[string]interface{}, 0, len(rawLog.Data))
	for _, item := range rawLog.Data {
		clientHost := ipToHost[item.Client]
		if clientHost == "" {
			clientHost = strings.TrimSpace(item.ClientInfo.Name)
		}
		blocked := strings.HasPrefix(item.Reason, "Filtered")
		row := map[string]interface{}{
			"time":       item.Time,
			"client_ip":  item.Client,
			"domain":     item.Question.Name,
			"query_type": item.Question.Type,
			"status":     item.Status,
			"reason":     item.Reason,
			"blocked":    blocked,
			"cached":     item.Cached,
		}
		if clientHost != "" {
			row["client_hostname"] = clientHost
		}
		if item.Upstream != "" {
			row["upstream"] = item.Upstream
		}
		if ms, err := strconv.ParseFloat(item.ElapsedMs, 64); err == nil {
			row["elapsed_ms"] = mathRound2(ms)
		}
		if len(item.Rules) > 0 && item.Rules[0].Text != "" {
			row["rule"] = item.Rules[0].Text
		}
		entries = append(entries, row)
	}

	out := map[string]interface{}{
		"count":           len(entries),
		"response_status": responseStatus,
		"queries":         entries,
	}
	if searchRaw != "" {
		out["search"] = searchRaw
	}
	if resolvedSearchIP != "" {
		out["resolved_search_ip"] = resolvedSearchIP
	}
	return out, nil
}
