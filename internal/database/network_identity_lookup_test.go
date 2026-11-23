package database

import "testing"

func TestSiteClientHostnameLookupResolvesMACAndIPCaseInsensitive(t *testing.T) {
	lookup := NewSiteClientHostnameLookup()
	lookup.ByMAC[NormalizeMAC("3c:28:6d:0f:85:db")] = "pixel3"
	lookup.ByMAC[NormalizeMAC("bc:24:11:0b:ea:8f")] = "dev"

	state := map[string]interface{}{
		"board": map[string]interface{}{"hostname": "mr8300"},
		"dhcp": map[string]interface{}{
			"leases": []interface{}{
				map[string]interface{}{"mac": "3c:28:6d:0f:85:db", "ip": "10.128.128.161", "hostname": "Pixel-3"},
				map[string]interface{}{"mac": "90:39:5f:2f:c9:7f", "ip": "10.128.128.89", "hostname": "firetv"},
				map[string]interface{}{"mac": "aa:bb:cc:dd:ee:ff", "ip": "10.128.128.200", "hostname": "*"},
			},
		},
		"neighbor_stats": map[string]interface{}{
			"arp_table": []interface{}{
				map[string]interface{}{"mac": "bc:24:11:0b:ea:8f", "ip": "10.128.128.6"},
			},
		},
	}

	lookup.MergeDeviceState("e8:9f:80:14:69:c5", "", "10.128.128.1", state)

	// Custom hostname takes precedence over DHCP lease hostname and is case-insensitive.
	if got := lookup.ResolveMAC("3c:28:6d:0f:85:db"); got != "pixel3" {
		t.Fatalf("ResolveMAC(pixel3) = %q, want pixel3", got)
	}
	if got := lookup.ResolveIP("10.128.128.161"); got != "pixel3" {
		t.Fatalf("ResolveIP(.161) = %q, want pixel3", got)
	}
	// DHCP lease hostname fills in clients not in client_hostnames.
	if got := lookup.Resolve("90:39:5F:2F:C9:7F"); got != "firetv" {
		t.Fatalf("Resolve(firetv MAC) = %q, want firetv", got)
	}
	if got := lookup.Resolve("10.128.128.89"); got != "firetv" {
		t.Fatalf("Resolve(firetv IP) = %q, want firetv", got)
	}
	// ARP table resolves static-IP hosts via client_hostnames.
	if got := lookup.ResolveIP("10.128.128.6"); got != "dev" {
		t.Fatalf("ResolveIP(.6) = %q, want dev", got)
	}
	// Wildcard DHCP hostnames are ignored.
	if got := lookup.ResolveMAC("aa:bb:cc:dd:ee:ff"); got != "" {
		t.Fatalf("ResolveMAC(wildcard) = %q, want empty", got)
	}
	// Device itself resolves by MAC and last_ip.
	if got := DeviceDisplayName("e8:9f:80:14:69:c5", nil, state, lookup); got != "mr8300" {
		t.Fatalf("DeviceDisplayName(mr8300) = %q, want mr8300", got)
	}
	if got := lookup.ResolveIP("10.128.128.1"); got != "mr8300" {
		t.Fatalf("ResolveIP(.1) = %q, want mr8300", got)
	}
}
