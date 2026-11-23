package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSentinelAdGuardStatsAndQueryLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "root" || pass != "secret-pass" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/control/status":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"version":            "0.107.79",
				"running":            true,
				"protection_enabled": true,
				"dns_port":           53,
			})
		case "/control/stats":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"time_units":                "days",
				"num_dns_queries":           1000,
				"num_blocked_filtering":     250,
				"num_replaced_safebrowsing": 2,
				"num_replaced_parental":     0,
				"avg_processing_time":       0.0345,
				"top_clients": []map[string]int64{
					{"10.128.128.22": 500},
					{"10.128.128.89": 200},
				},
				"top_queried_domains": []map[string]int64{
					{"ha.tuxevil.com": 300},
					{"example.com": 100},
				},
				"top_blocked_domains": []map[string]int64{
					{"ads.example.net": 120},
					{"tracker.example.net": 80},
				},
				"top_upstreams_responses": []map[string]int64{
					{"https://cloudflare-dns.com:443/dns-query": 800},
				},
				"top_upstreams_avg_time": []map[string]float64{
					{"https://cloudflare-dns.com:443/dns-query": 0.144},
				},
			})
		case "/control/filtering/status":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"enabled":    true,
				"user_rules": []string{"||bad.test^"},
				"filters": []map[string]interface{}{
					{"name": "AdGuard DNS filter", "enabled": true, "rules_count": 194101, "last_updated": "2026-11-23T16:15:19Z"},
				},
			})
		case "/control/querylog":
			if got := r.URL.Query().Get("response_status"); got != "blocked" {
				t.Errorf("response_status = %q, want blocked", got)
			}
			if got := r.URL.Query().Get("search"); got != "10.128.128.89" {
				t.Errorf("search = %q, want 10.128.128.89", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"time":      "2026-11-23T17:32:40Z",
						"client":    "10.128.128.89",
						"question":  map[string]string{"name": "ads.example.net", "type": "A"},
						"status":    "NOERROR",
						"reason":    "FilteredBlackList",
						"elapsedMs": "0.43",
						"cached":    false,
						"rules":     []map[string]interface{}{{"text": "||ads.example.net^", "filter_list_id": 1}},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("SENTINEL_ADGUARD_URL", srv.URL)
	t.Setenv("SENTINEL_ADGUARD_USERNAME", "root")
	t.Setenv("SENTINEL_ADGUARD_PASSWORD", "secret-pass")

	statsRes, err := sentinelToolRegistry.Execute(context.Background(), SentinelToolCall{
		Name:      "get_adguard_stats",
		Arguments: SentinelToolArgumentsWithSite(json.RawMessage(`{"top_limit":1}`), "tenant_demo", "site-1"),
	})
	if err != nil {
		t.Fatalf("get_adguard_stats failed: %v", err)
	}
	statsMap := statsRes.(map[string]interface{})
	if statsMap["blocked_percent"] != 25.0 {
		t.Fatalf("blocked_percent = %v, want 25.0", statsMap["blocked_percent"])
	}
	if statsMap["avg_processing_time_ms"] != 34.5 {
		t.Fatalf("avg_processing_time_ms = %v, want 34.5", statsMap["avg_processing_time_ms"])
	}
	topClients := statsMap["top_clients"].([]map[string]interface{})
	if len(topClients) != 1 || topClients[0]["client_ip"] != "10.128.128.22" {
		t.Fatalf("unexpected top_clients: %#v", topClients)
	}

	logRes, err := sentinelToolRegistry.Execute(context.Background(), SentinelToolCall{
		Name:      "get_adguard_querylog",
		Arguments: SentinelToolArgumentsWithSite(json.RawMessage(`{"search":"10.128.128.89","response_status":"blocked","limit":10}`), "tenant_demo", "site-1"),
	})
	if err != nil {
		t.Fatalf("get_adguard_querylog failed: %v", err)
	}
	logMap := logRes.(map[string]interface{})
	if logMap["count"] != 1 {
		t.Fatalf("querylog count = %v, want 1", logMap["count"])
	}
	queries := logMap["queries"].([]map[string]interface{})
	if queries[0]["blocked"] != true || queries[0]["rule"] != "||ads.example.net^" {
		t.Fatalf("unexpected query entry: %#v", queries[0])
	}

	// Invalid response_status must be rejected before calling AdGuardHome.
	if _, err := sentinelToolRegistry.Execute(context.Background(), SentinelToolCall{
		Name:      "get_adguard_querylog",
		Arguments: SentinelToolArgumentsWithSite(json.RawMessage(`{"response_status":"drop_tables"}`), "tenant_demo", "site-1"),
	}); err == nil {
		t.Fatal("expected invalid response_status to fail")
	}

	// Context tool plan should include AdGuard tools when configured.
	plan := sentinelContextToolPlan("que dominios bloqueados tiene adguard hoy?", SentinelCase{SiteID: "site-1"})
	joined := strings.Join(plan, ",")
	if !strings.Contains(joined, "get_adguard_stats") || !strings.Contains(joined, "get_adguard_querylog") {
		t.Fatalf("expected adguard tools in plan, got %v", plan)
	}
}

func TestSentinelAdGuardSecurityGuards(t *testing.T) {
	// Public IP must be rejected.
	t.Setenv("SENTINEL_ADGUARD_URL", "http://8.8.8.8:4000")
	if _, err := loadSentinelAdGuardConfig(); err == nil {
		t.Fatal("expected public IP SENTINEL_ADGUARD_URL to be rejected")
	}

	// Non-allowlisted path must be rejected even with valid private URL.
	t.Setenv("SENTINEL_ADGUARD_URL", "http://127.0.0.1:4000")
	var out map[string]interface{}
	if err := sentinelAdGuardGetJSON(context.Background(), "/control/protection", nil, &out); err == nil {
		t.Fatal("expected non-allowlisted AdGuardHome path to be rejected")
	}

	// Redirects must not be followed (prevents leaking Basic Auth).
	redirSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:9999/steal", http.StatusFound)
	}))
	defer redirSrv.Close()
	t.Setenv("SENTINEL_ADGUARD_URL", redirSrv.URL)
	if err := sentinelAdGuardGetJSON(context.Background(), "/control/status", nil, &out); err == nil {
		t.Fatal("expected HTTP redirect to fail without following")
	}
}
