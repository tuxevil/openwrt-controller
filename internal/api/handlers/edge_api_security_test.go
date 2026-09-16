package handlers

import (
	"context"
	"io"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
)

func TestGetDeviceIPForSiteScopesDeviceToSite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_ip FROM "tenant_demo".devices WHERE id = $1 AND site_id = $2`)).
		WithArgs("device-1", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"last_ip"}).AddRow("192.0.2.10"))

	got, err := getDeviceIPForSite(context.Background(), "tenant_demo", "site-1", "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "192.0.2.10" {
		t.Fatalf("device IP = %q, want 192.0.2.10", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSSHReadScriptRejectsShellInjection(t *testing.T) {
	for _, command := range []string{
		"uci show wireless; id",
		"uci show wireless && reboot",
		"ubus call uci get '{\"config\": \"wireless\"}'; cat /etc/shadow",
		"echo controller-compromised",
	} {
		if _, err := buildSSHReadScript(command); err == nil {
			t.Errorf("accepted unsafe SSH read command %q", command)
		}
	}
}

func TestBuildSSHReadScriptAcceptsAllowlistedReads(t *testing.T) {
	commands := []string{
		"uci show wireless.radio0.channel 2>&1",
		"ubus call uci get '{\"config\": \"wireless\"}'",
		"uci show wireless; echo \"===SECTION_BREAK===\"; uci show network; echo \"===SECTION_BREAK===\"; uci show dhcp; echo \"===SECTION_BREAK===\"; uci show firewall; echo \"===SECTION_BREAK===\"; uci show system; echo \"===SECTION_BREAK===\"; uci show dropbear; echo \"===SECTION_BREAK===\"; uci show usteer",
	}

	for _, command := range commands {
		script, err := buildSSHReadScript(command)
		if err != nil {
			t.Fatalf("rejected allowlisted SSH read command %q: %v", command, err)
		}
		if got, readErr := io.ReadAll(script); readErr != nil || string(got) != command+"\n" {
			t.Fatalf("read script = %q, %v; want %q", got, readErr, command+"\n")
		}
	}
}

func TestSSHTransportUsesFixedShellEntrypoint(t *testing.T) {
	if sshShellEntrypoint != "sh -s" {
		t.Fatalf("SSH shell entrypoint = %q, want fixed sh -s", sshShellEntrypoint)
	}
}

func TestGetDeviceIPForTenantRejectsMissingIP(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_ip FROM "tenant_demo".devices WHERE id = $1`)).
		WithArgs("device-1").
		WillReturnRows(sqlmock.NewRows([]string{"last_ip"}).AddRow(nil))

	if _, err := getDeviceIPForTenant(context.Background(), "tenant_demo", "device-1"); err == nil {
		t.Fatal("missing device IP was accepted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
