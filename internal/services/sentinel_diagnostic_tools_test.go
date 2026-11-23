package services

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestSentinelProbeValidationRejectsInjectionAndPublicTCP(t *testing.T) {
	badTargets := []string{
		"127.0.0.1; id",
		"$(whoami)",
		"-c 100 127.0.0.1",
		"example.com && rm -rf /",
		"10.128.128.1\nid",
		"",
	}
	for _, bad := range badTargets {
		if isValidSentinelProbeHost(bad) {
			t.Fatalf("isValidSentinelProbeHost(%q) = true, want false", bad)
		}
	}
	for _, good := range []string{"10.128.128.1", "1.1.1.1", "mr8300.lan", "google.com"} {
		if !isValidSentinelProbeHost(good) {
			t.Fatalf("isValidSentinelProbeHost(%q) = false, want true", good)
		}
	}

	// probe_tcp_port must reject public IPs
	pubCall := SentinelToolCall{
		Name:      "probe_tcp_port",
		Arguments: SentinelToolArgumentsWithSite(json.RawMessage(`{"host":"1.1.1.1","port":443}`), "tenant_demo", "site-1"),
	}
	if _, err := executeSentinelToolContext(context.Background(), pubCall); err == nil || !strings.Contains(err.Error(), "restricted to private LAN") {
		t.Fatalf("probe_tcp_port on public IP should fail closed, got err=%v", err)
	}

	// ping_host must reject shell metacharacters
	badPing := SentinelToolCall{
		Name:      "ping_host",
		Arguments: SentinelToolArgumentsWithSite(json.RawMessage(`{"target":"10.128.128.1;id"}`), "tenant_demo", "site-1"),
	}
	if _, err := executeSentinelToolContext(context.Background(), badPing); err == nil {
		t.Fatal("ping_host accepted shell injection target")
	}

	// dns_lookup must reject raw IPs and invalid servers
	badDNS := SentinelToolCall{
		Name:      "dns_lookup",
		Arguments: SentinelToolArgumentsWithSite(json.RawMessage(`{"domain":"example.com","server":"10.128.128.1;id"}`), "tenant_demo", "site-1"),
	}
	if _, err := executeSentinelToolContext(context.Background(), badDNS); err == nil {
		t.Fatal("dns_lookup accepted invalid server")
	}
}

func TestSentinelProbeTCPPortLocalLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	rawArgs, _ := json.Marshal(map[string]interface{}{"host": "127.0.0.1", "port": port, "timeout_ms": 1000})
	call := SentinelToolCall{
		Name:      "probe_tcp_port",
		Arguments: SentinelToolArgumentsWithSite(rawArgs, "tenant_demo", "site-1"),
	}
	res, err := executeSentinelToolContext(context.Background(), call)
	if err != nil {
		t.Fatalf("probe_tcp_port failed: %v", err)
	}
	m, ok := res.(map[string]interface{})
	if !ok || m["reachable"] != true {
		t.Fatalf("probe_tcp_port = %#v, want reachable=true", res)
	}
}

func TestSanitizeWirelessUCIConfigRedactsSecrets(t *testing.T) {
	raw := `
wireless.radio0.channel='149'
wireless.radio0.htmode='HT40'
wireless.cfg_radio0_2.ssid='tuxcave'
wireless.cfg_radio0_2.key='super-secret-pass'
wireless.cfg_radio0_2.auth_secret='radsec'
wireless.cfg_radio0_2.r0kh='00:11:22:33:44:55,nas,deadbeef'
wireless.cfg_radio2_2.macfilter='deny'
wireless.cfg_radio2_2.maclist='3c:28:6d:0f:85:db'
`
	clean := sanitizeWirelessUCIConfig(raw)
	for _, secret := range []string{"super-secret-pass", "radsec", "deadbeef"} {
		if strings.Contains(clean, secret) {
			t.Fatalf("sanitizeWirelessUCIConfig leaked secret %q: %s", secret, clean)
		}
	}
	for _, keep := range []string{"htmode='HT40'", "channel='149'", "macfilter='deny'", "3c:28:6d:0f:85:db"} {
		if !strings.Contains(clean, keep) {
			t.Fatalf("sanitizeWirelessUCIConfig dropped %q: %s", keep, clean)
		}
	}
}

func TestParsePingAndTracerouteOutputs(t *testing.T) {
	pingOut := `PING 10.128.128.1 (10.128.128.1): 56 data bytes
64 bytes from 10.128.128.1: seq=0 ttl=64 time=0.512 ms
--- 10.128.128.1 ping statistics ---
2 packets transmitted, 2 packets received, 0% packet loss
round-trip min/avg/max = 0.512/0.620/0.728 ms`
	parsed := parsePingSummary(pingOut)
	if parsed["reachable"] != true || parsed["received"] != 2 || parsed["packet_loss_pct"] != 0.0 || parsed["avg_ms"] != 0.62 {
		t.Fatalf("parsePingSummary = %#v", parsed)
	}

	traceOut := `traceroute to 1.1.1.1 (1.1.1.1), 12 hops max
 1  10.128.128.1  0.412 ms
 2  100.64.0.1  21.340 ms
 3  *`
	hops := parseTracerouteHops(traceOut)
	if len(hops) != 3 || hops[0]["ip"] != "10.128.128.1" || hops[1]["rtt_ms"] != 21.34 || hops[2]["ip"] != "*" {
		t.Fatalf("parseTracerouteHops = %#v", hops)
	}
}
