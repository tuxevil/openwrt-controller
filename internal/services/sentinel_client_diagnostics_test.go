package services

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func TestExtractSentinelMACFromQueryAndToolPlan(t *testing.T) {
	if got := extractSentinelMACFromQuery("por que este cliente no se esta pudiendo conectar? 3C:28:6D:0F:85:DB"); got != "3c:28:6d:0f:85:db" {
		t.Fatalf("extractSentinelMACFromQuery() = %q, want 3c:28:6d:0f:85:db", got)
	}
	if got := extractSentinelMACFromQuery("How many clients are connected?"); got != "" {
		t.Fatalf("expected empty MAC, got %q", got)
	}

	planWithMAC := sentinelContextToolPlan("por que este cliente no se esta pudiendo conectar? 3c:28:6d:0f:85:db", SentinelCase{SiteID: "site-1"})
	if len(planWithMAC) == 0 || planWithMAC[0] != "search_logs" {
		t.Fatalf("planWithMAC = %v, want search_logs first", planWithMAC)
	}
	hasClients := false
	for _, tool := range planWithMAC {
		if tool == "get_site_clients" {
			hasClients = true
		}
	}
	if !hasClients {
		t.Fatalf("planWithMAC = %v, expected get_site_clients", planWithMAC)
	}

	planWithoutMAC := sentinelContextToolPlan("por que este cliente no se puede conectar por dhcp?", SentinelCase{SiteID: "site-1"})
	hasLogs, hasSiteClients := false, false
	for _, tool := range planWithoutMAC {
		if tool == "search_logs" {
			hasLogs = true
		}
		if tool == "get_site_clients" {
			hasSiteClients = true
		}
	}
	if !hasLogs || !hasSiteClients {
		t.Fatalf("planWithoutMAC = %v, want both search_logs and get_site_clients", planWithoutMAC)
	}
}

func TestSentinelClientClassificationDistinguishesActiveWirelessFromCaches(t *testing.T) {
	clients := map[string]map[string]bool{}
	collectSentinelMACList(clients, []interface{}{
		map[string]interface{}{"mac": "aa:bb:cc:dd:ee:01"},
	}, "wireless_station")
	collectSentinelMACList(clients, []interface{}{
		map[string]interface{}{"mac": "3c:28:6d:0f:85:db", "ip": "10.128.128.161"},
		map[string]interface{}{"mac": "aa:bb:cc:dd:ee:01", "ip": "10.128.128.10"},
	}, "arp")
	collectSentinelMACList(clients, []interface{}{
		map[string]interface{}{"mac": "e8:9f:80:14:69:c5", "port": "1", "is_local": "yes"},
		map[string]interface{}{"mac": "bc:24:11:0b:ea:8f", "port": "2", "is_local": "no"},
	}, "bridge")
	collectSentinelMACList(clients, []interface{}{
		map[string]interface{}{"mac": "11:22:33:44:55:66", "ip": "10.128.128.99"},
	}, "dhcp_lease")

	if _, exists := clients["E8:9F:80:14:69:C5"]; exists {
		t.Fatal("local bridge port MAC must be excluded from client list")
	}
	if got := classifySentinelClientConnection(clients["AA:BB:CC:DD:EE:01"]); got != "wireless" {
		t.Fatalf("active wireless station classified as %q, want wireless", got)
	}
	if got := classifySentinelClientConnection(clients["3C:28:6D:0F:85:DB"]); got != "wired_or_cached" {
		t.Fatalf("ARP-only MAC classified as %q, want wired_or_cached", got)
	}
	if got := classifySentinelClientConnection(clients["BC:24:11:0B:EA:8F"]); got != "wired_or_cached" {
		t.Fatalf("bridge-only MAC classified as %q, want wired_or_cached", got)
	}
	if got := classifySentinelClientConnection(clients["11:22:33:44:55:66"]); got != "dhcp_lease_only" {
		t.Fatalf("DHCP-lease-only MAC classified as %q, want dhcp_lease_only", got)
	}
}

func TestCreateSentinelProposalRejectsUnknownManagedDevice(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previous
		_ = db.Close()
	})

	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM tenant_demo.devices WHERE id = $1)")).
		WithArgs("3c:28:6d:0f:85:db").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	draft := &SentinelProposalDraft{
		DeviceID:      "3c:28:6d:0f:85:db",
		Config:        "system",
		Summary:       "Bogus client proposal",
		BlockedReason: "proposal targets a device outside the Case site scope",
	}
	_, err = CreateSentinelProposal("tenant_demo", "", "", "tuxevil", draft)
	if !errors.Is(err, ErrSentinelProposalUnknownDevice) {
		t.Fatalf("CreateSentinelProposal() err = %v, want ErrSentinelProposalUnknownDevice", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSentinelPromptGuidesClientLogTroubleshootingAndLanguage(t *testing.T) {
	for _, needle := range []string{
		"Answer in the same language as the operator's question",
		"ALWAYS inspect search_logs",
		"DHCPDISCOVER/DHCPOFFER/DHCPREQUEST/DHCPACK",
		"never put a client MAC",
	} {
		if !strings.Contains(sentinelAgentSystemPrompt, needle) {
			t.Fatalf("sentinelAgentSystemPrompt missing guidance %q", needle)
		}
	}
}
