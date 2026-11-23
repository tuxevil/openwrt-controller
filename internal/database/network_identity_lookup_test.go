package database

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

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

func TestLoadSiteClientHostnameLookupSupportsIPAliases(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	origDB := DB
	DB = db
	t.Cleanup(func() {
		DB = origDB
		_ = db.Close()
	})

	mock.ExpectQuery(`SELECT mac, hostname FROM client_hostnames WHERE site_id = \$1`).
		WithArgs("site-1").
		WillReturnRows(sqlmock.NewRows([]string{"mac", "hostname"}).
			AddRow("10.128.129.1", "vps").
			AddRow("3C:28:6D:0F:85:DB", "pixel3"))

	lookup := LoadSiteClientHostnameLookup(context.Background(), "site-1")
	if got := lookup.ResolveIP("10.128.129.1"); got != "vps" {
		t.Fatalf("ResolveIP(10.128.129.1) = %q, want vps", got)
	}
	if got := lookup.Resolve("10.128.129.1"); got != "vps" {
		t.Fatalf("Resolve(10.128.129.1) = %q, want vps", got)
	}
	if got := lookup.ResolveMAC("3c:28:6d:0f:85:db"); got != "pixel3" {
		t.Fatalf("ResolveMAC(pixel3) = %q, want pixel3", got)
	}
}

func TestExtractAndSyncDHCPHostnamesDeduplicates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	origDB := DB
	DB = db
	dhcpHostnameSyncMu.Lock()
	dhcpHostnameSyncSeen = map[string]string{}
	dhcpHostnameSyncMu.Unlock()
	t.Cleanup(func() {
		DB = origDB
		dhcpHostnameSyncMu.Lock()
		dhcpHostnameSyncSeen = map[string]string{}
		dhcpHostnameSyncMu.Unlock()
		_ = db.Close()
	})

	state := map[string]interface{}{
		"dhcp": map[string]interface{}{
			"leases": []interface{}{
				map[string]interface{}{"mac": "90:39:5f:2f:c9:7f", "ip": "10.128.128.89", "hostname": "firetv"},
				map[string]interface{}{"mac": "aa:bb:cc:dd:ee:ff", "ip": "10.128.128.200", "hostname": "*"},
				map[string]interface{}{"mac": "not-a-mac", "ip": "10.128.128.201", "hostname": "ignored"},
			},
		},
	}

	got := ExtractDHCPHostnameMap(state)
	if len(got) != 1 || got["90:39:5F:2F:C9:7F"] != "firetv" {
		t.Fatalf("ExtractDHCPHostnameMap = %#v, want single firetv entry", got)
	}

	mock.ExpectExec(`INSERT INTO tenant_demo\.client_hostnames`).
		WithArgs("90:39:5F:2F:C9:7F", "site-1", "firetv").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// First call writes to DB; second identical call is deduplicated in memory.
	SyncDHCPHostnamesFromState("tenant_demo", "site-1", state)
	SyncDHCPHostnamesFromState("tenant_demo", "site-1", state)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}
