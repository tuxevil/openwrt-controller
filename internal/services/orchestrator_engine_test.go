package services

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func TestLogicalNetworkSectionUsesExplicitMapping(t *testing.T) {
	capabilities := DeviceCapabilities{LogicalNetworks: map[string]string{"lan": "home"}}
	if got := logicalNetworkSection(capabilities, "lan"); got != "home" {
		t.Fatalf("got %q, want home", got)
	}
	if got := logicalNetworkSection(DeviceCapabilities{}, "lan"); got != "lan" {
		t.Fatalf("got %q, want legacy lan", got)
	}
}

func TestRenderSiteConfigDoesNotEmitUnsupportedDPIOption(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{DPIEnabled: true, GlobalSSID: "test"}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "firewall" && command.Option == "dpi_enabled" {
			t.Fatal("renderer emitted unsupported firewall dpi_enabled option")
		}
	}
}

func TestRenderGatewayRemovesLegacyDPIOptionWhenDisabled(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{DPIEnabled: false}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "firewall" && command.Section == "@defaults[0]" && command.Option == "dpi_enabled" {
			if command.Action != "delete" {
				t.Fatalf("legacy DPI option must be deleted, got action %q", command.Action)
			}
			return
		}
	}
	t.Fatal("renderer did not remove the legacy firewall dpi_enabled option")
}

func TestRenderGatewaySkipsSQMWhenDisabled(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{SQMCakeEnabled: false}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "sqm" {
			t.Fatalf("disabled SQM must not mutate optional sqm config: %#v", command)
		}
	}
}

func TestRenderGatewayEnsuresDHCPReservationsWithoutUnconditionalAdds(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{
		DHCPReservations: []byte(`[{"name":"test-host","mac":"AA:BB:CC:DD:EE:FF","ip":"192.0.2.10"}]`),
	}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	found := false
	for _, command := range results[0].Commands {
		if command.Config != "dhcp" || command.Option != "AA:BB:CC:DD:EE:FF" {
			continue
		}
		if command.Action != "ensure_host" || command.Section != "test-host" || command.Value != "192.0.2.10" {
			t.Fatalf("unexpected DHCP reservation command: %#v", command)
		}
		found = true
	}
	if !found {
		t.Fatal("renderer did not emit an idempotent DHCP reservation command")
	}
}

func TestRenderGatewayUsesConfiguredSQMSectionAndWANInterface(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{SQMCakeEnabled: true, SQMSection: "eth1", SQMInterface: "wan", SqmDownload: 115000, SqmUpload: 18000}, []DeviceRoleInfo{{
		DeviceID: "gateway",
		Role:     "Gateway",
		Capabilities: DeviceCapabilities{
			SQMCandidates: []string{"eth0", "eth1"},
		},
	}})
	foundEnabled, foundInterface := false, false
	for _, command := range results[0].Commands {
		if command.Config != "sqm" {
			continue
		}
		if command.Section == "@queue[0]" || command.Section == "@sqm[0]" {
			t.Fatalf("SQM must target a named section, got %#v", command)
		}
		if command.Section != "eth1" {
			t.Fatalf("SQM targeted %q instead of the configured queue", command.Section)
		}
		if command.Option == "enabled" && command.Value == "1" {
			foundEnabled = true
		}
		if command.Option == "interface" && command.Value == "wan" {
			foundInterface = true
		}
	}
	if !foundEnabled || !foundInterface {
		t.Fatal("renderer did not target the configured SQM queue and WAN interface")
	}
}

func TestSQMRequiresExplicitSafeTargetAndRates(t *testing.T) {
	cases := []SiteConfig{
		{SQMCakeEnabled: true, SQMSection: "", SQMInterface: "wan", SqmDownload: 115000, SqmUpload: 18000},
		{SQMCakeEnabled: true, SQMSection: "eth1", SQMInterface: "", SqmDownload: 115000, SqmUpload: 18000},
		{SQMCakeEnabled: true, SQMSection: "@queue[0]", SQMInterface: "wan", SqmDownload: 115000, SqmUpload: 18000},
		{SQMCakeEnabled: true, SQMSection: "eth1", SQMInterface: "wan", SqmDownload: 0, SqmUpload: 18000},
	}
	for _, cfg := range cases {
		if err := cfg.ValidateSQM(); err == nil {
			t.Fatalf("accepted unsafe SQM configuration: %#v", cfg)
		}
	}
	if err := (SiteConfig{SQMCakeEnabled: true, SQMSection: "eth1", SQMInterface: "wan", SqmDownload: 115000, SqmUpload: 18000}).ValidateSQM(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderAPNeverEnablesGatewaySQM(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{SQMCakeEnabled: true, SQMSection: "eth1", SQMInterface: "wan", SqmDownload: 115000, SqmUpload: 18000}, []DeviceRoleInfo{{DeviceID: "ap", Role: "AP"}})
	for _, command := range results[0].Commands {
		if command.Config == "sqm" {
			t.Fatalf("AP received gateway SQM command: %#v", command)
		}
	}
}

func TestRenderGatewayDisablesHardwareOffloadForCAKE(t *testing.T) {
	results := RenderSiteConfig(SiteConfig{SQMCakeEnabled: true, SQMSection: "eth1", SQMInterface: "wan", SqmDownload: 115000, SqmUpload: 18000}, []DeviceRoleInfo{{DeviceID: "gateway", Role: "Gateway"}})
	for _, command := range results[0].Commands {
		if command.Config == "firewall" && command.Section == "@defaults[0]" && command.Option == "flow_offloading_hw" && command.Value == "0" {
			return
		}
	}
	t.Fatal("gateway CAKE must disable hardware flow offloading")
}

func TestUpdateDeviceRoleForTenantRejectsRunningRollout(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_demo".devices SET device_role = $1 WHERE id = $2 AND COALESCE(last_rollout_status, '') <> 'RUNNING'`)).
		WithArgs("AP", "device-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := UpdateDeviceRoleForTenant(t.Context(), "tenant_demo", "device-1", "AP"); err != ErrDeviceRoleUpdateConflict {
		t.Fatalf("role update error = %v, want ErrDeviceRoleUpdateConflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
