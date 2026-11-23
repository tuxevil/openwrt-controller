package services

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"openwrt-controller/internal/database"
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

func TestParseWANLiveStatusParsesWireGuardAndMwan3AndResolvesVPS(t *testing.T) {
	now := time.Unix(1795456262, 0).UTC()
	raw := "WG_IFACE\twg0\t51820\toff\n" +
		"WG_PEER\twg0\tZ6to1qQJ1234\t18.226.129.187:31337\t10.128.129.0/24,192.168.1.0/24\t1795456202\t104857600\t209715200\t25\n" +
		"---MWAN3---\n" +
		"Interface status:\n" +
		" interface wan is online and tracking is active (online 49h:35m:41s, uptime 49h:36m:01s)\n" +
		" interface wanb is unknown and tracking is down (31)\n\n" +
		"Current ipv4 policies:\n" +
		"balanced:\n" +
		" wan (100%)\n" +
		"wanb_only:\n" +
		" unreachable\n" +
		"---STARLINK_DISH---\n" +
		"2 packets transmitted, 2 packets received, 0% packet loss\n" +
		"round-trip min/avg/max = 1.333/1.356/1.380 ms\n" +
		"---WAN_POP---\n" +
		"2 packets transmitted, 2 packets received, 0% packet loss\n" +
		"round-trip min/avg/max = 25.427/27.862/30.298 ms\n"

	lookup := database.NewSiteClientHostnameLookup()
	lookup.ByIP["10.128.129.1"] = "vps"

	wg, mwan, sl := parseWANLiveStatus(raw, now, lookup)
	if sl["available"] != true || sl["health_rating"] != "OPTIMAL" || sl["dish_rtt_ms"] != 1.36 || sl["pop_rtt_ms"] != 27.86 {
		t.Fatalf("unexpected live starlink probe: %#v", sl)
	}
	if wg["peer_count"] != 1 {
		t.Fatalf("wg peer_count = %#v", wg)
	}
	peers := wg["peers"].([]map[string]interface{})
	if peers[0]["tunnel_alive"] != true || peers[0]["handshake_age_seconds"] != int64(60) {
		t.Fatalf("unexpected peer handshake state: %#v", peers[0])
	}
	if peers[0]["rx_mb"] != 100.0 || peers[0]["tx_mb"] != 200.0 {
		t.Fatalf("unexpected peer transfer mb: %#v", peers[0])
	}
	hosts, _ := peers[0]["known_remote_hosts"].([]string)
	if len(hosts) != 1 || hosts[0] != "vps (10.128.129.1)" {
		t.Fatalf("expected vps host resolution, got %#v", hosts)
	}
	if mwan["available"] != true {
		t.Fatalf("expected mwan3 available, got %#v", mwan)
	}
	policies := mwan["ipv4_policies"].(map[string]string)
	if policies["balanced"] != "wan (100%)" || policies["wanb_only"] != "unreachable" {
		t.Fatalf("unexpected mwan3 policies: %#v", policies)
	}
}

func TestParseUCIFirewallSummaryResolvesHostnamesAndSanitizes(t *testing.T) {
	raw := `firewall.@defaults[0]=defaults
firewall.@defaults[0].input='REJECT'
firewall.@defaults[0].output='ACCEPT'
firewall.@defaults[0].forward='REJECT'
firewall.@defaults[0].synflood_protect='1'
firewall.@zone[0]=zone
firewall.@zone[0].name='wan'
firewall.@zone[0].input='REJECT'
firewall.@zone[0].output='ACCEPT'
firewall.@zone[0].forward='REJECT'
firewall.@zone[0].masq='1'
firewall.@forwarding[0]=forwarding
firewall.@forwarding[0].src='lan'
firewall.@forwarding[0].dest='wan'
firewall.@redirect[0]=redirect
firewall.@redirect[0].name='Allow-HTTPS-Prod'
firewall.@redirect[0].src='wan'
firewall.@redirect[0].src_dport='443'
firewall.@redirect[0].dest='lan'
firewall.@redirect[0].dest_ip='10.128.128.22'
firewall.@redirect[0].dest_port='443'
firewall.@redirect[0].proto='tcp'
firewall.@redirect[0].target='DNAT'
firewall.@rule[0]=rule
firewall.@rule[0].name='Allow-Ping'
firewall.@rule[0].src='wan'
firewall.@rule[0].proto='icmp'
firewall.@rule[0].target='ACCEPT'
firewall.@rule[0].secret_key='should-not-appear'`

	lookup := database.NewSiteClientHostnameLookup()
	lookup.ByIP["10.128.128.22"] = "prod"

	summary := parseUCIFirewallSummary(raw, lookup)
	if summary["port_forward_count"] != 1 || summary["rule_count"] != 1 {
		t.Fatalf("unexpected counts: %#v", summary)
	}
	forwards := summary["port_forwards"].([]map[string]interface{})
	if forwards[0]["dest_hostname"] != "prod" || forwards[0]["src_dport"] != "443" || forwards[0]["enabled"] != true {
		t.Fatalf("unexpected port forward: %#v", forwards[0])
	}
	encoded, _ := json.Marshal(summary)
	if strings.Contains(string(encoded), "should-not-appear") {
		t.Fatalf("secret leaked into firewall summary: %s", string(encoded))
	}
}

func TestSentinelContextToolPlanSelectsFirewallAndWAN(t *testing.T) {
	fwPlan := sentinelContextToolPlan("que puertos tengo expuestos en el firewall?", SentinelCase{})
	foundFW := false
	for _, tool := range fwPlan {
		if tool == "get_firewall_summary" {
			foundFW = true
		}
	}
	if !foundFW {
		t.Fatalf("expected get_firewall_summary in %v", fwPlan)
	}

	wanPlan := sentinelContextToolPlan("esta vivo el tunel wireguard hacia el vps?", SentinelCase{})
	foundWAN := false
	for _, tool := range wanPlan {
		if tool == "get_wan_health" {
			foundWAN = true
		}
	}
	if !foundWAN {
		t.Fatalf("expected get_wan_health in %v", wanPlan)
	}

	wifiPlan := sentinelContextToolPlan("Revisa la configuración Wi-Fi actual de mr8300: canales, ancho de banda (htmode) y macfilter.", SentinelCase{})
	foundWiFi, foundBW := false, false
	for _, tool := range wifiPlan {
		if tool == "get_wifi_config_summary" {
			foundWiFi = true
		}
		if tool == "get_bandwidth_stats" {
			foundBW = true
		}
	}
	if !foundWiFi || foundBW {
		t.Fatalf("expected get_wifi_config_summary without get_bandwidth_stats, got %v", wifiPlan)
	}

	pingPlan := sentinelContextToolPlan("Haz ping a 10.128.128.3 desde mr8300", SentinelCase{SiteID: "site-1"})
	if len(pingPlan) != 1 || pingPlan[0] != "get_device_status" {
		t.Fatalf("expected lean get_device_status for active probe, got %v", pingPlan)
	}
}

func TestCompactSentinelDeviceStateStripsBloatAndPreservesRadios(t *testing.T) {
	raw := []byte(`{"board":{"hostname":"mr8300"},"logs":"very huge log blob","arp_table":[{"ip":"10.0.0.1"}],"bridge_table":[{"mac":"aa:bb"}],"dhcp":{"leases":[{"ip":"10.0.0.2"}]},"wireless_stations":{"phy2-ap0":[{"mac":"3c:28:6d:0f:85:db","signal":-50,"tx_rate":"144.4","tx_failed":0,"extra_noise":"drop"}]}}`)
	compacted := compactSentinelDeviceState(raw, map[string]database.NetworkIdentity{
		"3C:28:6D:0F:85:DB": {Label: "pixel3"},
	})
	encoded, _ := json.Marshal(compacted)
	s := string(encoded)
	for _, forbidden := range []string{"very huge log blob", "arp_table", "bridge_table", "extra_noise"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("expected %q to be stripped from compact device state: %s", forbidden, s)
		}
	}
	for _, expected := range []string{"mr8300", "lease_count", "phy2-ap0", "pixel3"} {
		if !strings.Contains(s, expected) {
			t.Fatalf("expected %q in compact device state: %s", expected, s)
		}
	}
}

func TestSentinelLooksLikeRefusal(t *testing.T) {
	if sentinelLooksLikeRefusal("Se ha ejecutado una prueba iperf3 contra wndr3800: 65 Mbps.") {
		t.Fatal("fabricated probe answer must not look like a refusal")
	}
	if !sentinelLooksLikeRefusal("No puedo ejecutar comandos del sistema operativo como cat /etc/passwd.") {
		t.Fatal("refusal must be detected so probe nudge does not fire")
	}
}
