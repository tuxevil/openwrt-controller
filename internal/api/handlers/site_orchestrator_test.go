package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func TestSortedConfigNames(t *testing.T) {
	got := sortedConfigNames(map[string][]services.UciCommand{
		"wireless": nil,
		"network":  nil,
		"dhcp":     nil,
	})
	want := []string{"dhcp", "network", "wireless"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFleetRolloutPhasesUsesFirstDeviceAsCanary(t *testing.T) {
	got := fleetRolloutPhases(10)
	want := [][]int{{0}, {1, 2, 3, 4}, {5, 6, 7, 8}, {9}}
	if len(got) != len(want) {
		t.Fatalf("got phases %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("got phases %v, want %v", got, want)
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("got phases %v, want %v", got, want)
			}
		}
	}
}

func TestFleetRolloutPhasesHandlesEmptyFleet(t *testing.T) {
	if got := fleetRolloutPhases(0); got != nil {
		t.Fatalf("got phases %v, want nil", got)
	}
}

func TestBoundedRolloutDiagnostic(t *testing.T) {
	short := "script execution failed at line 4"
	if got := boundedRolloutDiagnostic(short); got != short {
		t.Fatalf("short diagnostic changed: %q", got)
	}
	long := strings.Repeat("x", maxRolloutDiagnosticBytes+100)
	got := boundedRolloutDiagnostic(long)
	if len(got) <= maxRolloutDiagnosticBytes || !strings.HasSuffix(got, "[diagnostic output truncated]") {
		t.Fatalf("long diagnostic was not bounded: length=%d", len(got))
	}
}

func TestSequentialFleetRolloutPhasesPutGatewayLast(t *testing.T) {
	results := []services.RenderResult{
		{DeviceID: "gateway", Role: "Gateway"},
		{DeviceID: "ap-b", Role: "AP"},
		{DeviceID: "ap-a", Role: "AP"},
	}
	got := sequentialFleetRolloutPhases(results)
	want := [][]int{{2}, {1}, {0}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != 1 || got[i][0] != want[i][0] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSelectRolloutDevicesDefaultsToFullSite(t *testing.T) {
	devs := []services.DeviceRoleInfo{{DeviceID: "ap-a"}, {DeviceID: "gateway"}}
	got, err := selectRolloutDevices(devs, "")
	if err != nil {
		t.Fatalf("selectRolloutDevices returned error: %v", err)
	}
	if len(got) != len(devs) || got[0].DeviceID != "ap-a" || got[1].DeviceID != "gateway" {
		t.Fatalf("got %#v, want original device list", got)
	}
}

func TestSelectRolloutDevicesCanTargetOneDevice(t *testing.T) {
	devs := []services.DeviceRoleInfo{{DeviceID: "ap-a"}, {DeviceID: "gateway"}}
	got, err := selectRolloutDevices(devs, "gateway")
	if err != nil {
		t.Fatalf("selectRolloutDevices returned error: %v", err)
	}
	if len(got) != 1 || got[0].DeviceID != "gateway" {
		t.Fatalf("got %#v, want gateway only", got)
	}
}

func TestSelectRolloutDevicesRejectsUnknownTarget(t *testing.T) {
	_, err := selectRolloutDevices([]services.DeviceRoleInfo{{DeviceID: "gateway"}}, "missing")
	if err == nil {
		t.Fatal("expected unknown target to be rejected")
	}
}

func TestRolloutHealthTargetsPersistsEffectiveDefault(t *testing.T) {
	targets, err := rolloutHealthTargets(services.SiteConfig{HealthChecks: []byte(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != "1.1.1.1" {
		t.Fatalf("health targets = %#v, want effective default", targets)
	}
}

func TestFilterUCICommandsByObservedStateSkipsSatisfiedCommands(t *testing.T) {
	commands := []services.UciCommand{
		{Action: "set", Config: "network", Section: "lan", Option: "ipaddr", Value: "10.128.128.1"},
		{Action: "set", Config: "network", Section: "lan", Option: "netmask", Value: "255.255.255.0"},
		{Action: "set", Config: "network", Section: "lan", Option: "ipaddr", Value: "192.0.2.1"},
	}
	sections := parseUciShow(`network.lan=interface
network.lan.ipaddr='10.128.128.1'
network.lan.netmask='255.255.255.0'
`, "network")

	got := filterUCICommandsByObservedState(commands, sections)
	if len(got) != 1 || got[0].Value != "192.0.2.1" {
		t.Fatalf("got %#v, want only the unsatisfied command", got)
	}
}

func TestRolloutResultSuccessIncludesSkipped(t *testing.T) {
	if !rolloutResultSuccess("SUCCESS") || !rolloutResultSuccess("SKIPPED") {
		t.Fatal("successful and no-op rollout results should both count as successful")
	}
	if rolloutResultSuccess("FAILED") || rolloutResultSuccess("ABORTED") {
		t.Fatal("failed and aborted rollout results must not advance generation")
	}
}

func TestRejectUnsafeNetworkMutations(t *testing.T) {
	results := []services.RenderResult{{
		DeviceID: "gateway",
		Commands: []services.UciCommand{{
			Action:  "set",
			Config:  "network",
			Section: "wan",
			Option:  "metric",
			Value:   "10",
		}},
	}}
	if err := rejectUnsafeNetworkMutations(results); err == nil {
		t.Fatal("network mutations must be rejected until they have a transport-safe executor")
	}

	if err := rejectUnsafeNetworkMutations([]services.RenderResult{{
		DeviceID: "ap",
		Commands: []services.UciCommand{{Config: "wireless", Action: "set"}},
	}}); err != nil {
		t.Fatalf("non-network mutations should remain allowed: %v", err)
	}
}

func TestBuildRolloutDraftCapturesCommandsAndObservedState(t *testing.T) {
	results := []services.RenderResult{{
		DeviceID: "device-a",
		Hostname: "ap-a",
		Role:     "AP",
		LastIP:   "192.0.2.10",
		Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "ap-a"}},
	}}
	observed := map[string]map[string]string{
		"device-a": {"system": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}

	draft, err := buildRolloutDraft("site-1", "", []string{"1.1.1.1"}, results, observed)
	if err != nil {
		t.Fatal(err)
	}
	if draft.SiteID != "site-1" || len(draft.Devices) != 1 {
		t.Fatalf("draft = %#v", draft)
	}
	if draft.Devices[0].Commands[0].Value != "ap-a" || len(draft.Devices[0].PreviewCommands) != 1 || draft.Devices[0].Scripts["system"] == "" || draft.Devices[0].ObservedState["system"] == "" {
		t.Fatalf("draft lost exact command, script, or observed state: %#v", draft.Devices[0])
	}
	if rolloutPlanHash(draft) == "" {
		t.Fatal("draft hash is empty")
	}
	plan, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRolloutDraft(database.RolloutDraftRecord{SiteID: draft.SiteID, PlanHash: rolloutPlanHash(draft), Plan: plan}); err != nil {
		t.Fatalf("serialized draft failed identity validation: %v", err)
	}
}

func TestFilterRolloutResultsByNamespaceKeepsOnlySystemCommands(t *testing.T) {
	results := []services.RenderResult{{
		DeviceID: "device-a",
		Commands: []services.UciCommand{
			{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "router-a"},
			{Action: "set", Config: "dropbear", Section: "global", Option: "Port", Value: "22"},
		},
	}}

	filtered, err := filterRolloutResultsByNamespace(results, "system")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || len(filtered[0].Commands) != 1 || filtered[0].Commands[0].Config != "system" {
		t.Fatalf("filtered results = %#v", filtered)
	}
	if len(results[0].Commands) != 2 {
		t.Fatal("namespace filtering mutated the rendered results")
	}
}

func TestFilterRolloutResultsByNamespaceRejectsUnsupportedNamespace(t *testing.T) {
	if _, err := filterRolloutResultsByNamespace(nil, "network"); err == nil {
		t.Fatal("unsupported namespace was accepted")
	}
}

func TestRolloutPlanHashStableForSameDraft(t *testing.T) {
	first, err := buildRolloutDraft("site-1", "", nil, []services.RenderResult{{
		DeviceID: "device-a",
		Hostname: "ap-a",
		Role:     "AP",
		Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "ap-a"}},
	}}, map[string]map[string]string{"device-a": {"system": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}})
	if err != nil {
		t.Fatal(err)
	}
	second := first
	if rolloutPlanHash(first) != rolloutPlanHash(second) {
		t.Fatal("equivalent immutable plans should have the same hash")
	}
}

func TestRolloutPlanHashIncludesExecutionOrder(t *testing.T) {
	first := rolloutDraft{
		SiteID: "site-1",
		Devices: []rolloutDraftDevice{
			{DeviceID: "device-a", Commands: []services.UciCommand{{Action: "set", Config: "system", Option: "hostname", Value: "a"}}},
			{DeviceID: "device-b", Commands: []services.UciCommand{{Action: "set", Config: "system", Option: "hostname", Value: "b"}}},
		},
	}
	second := first
	second.Devices = []rolloutDraftDevice{first.Devices[1], first.Devices[0]}
	if rolloutPlanHash(first) == rolloutPlanHash(second) {
		t.Fatal("changing execution order must change the immutable plan hash")
	}
}

func TestBuildRolloutDraftOrderingDoesNotDependOnDeviceIP(t *testing.T) {
	results := []services.RenderResult{
		{DeviceID: "device-b", Role: "AP", LastIP: "192.0.2.2", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "b"}}},
		{DeviceID: "device-a", Role: "AP", LastIP: "192.0.2.1", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "a"}}},
	}
	observed := map[string]map[string]string{
		"device-a": {"system": strings.Repeat("a", 64)},
		"device-b": {"system": strings.Repeat("b", 64)},
	}
	first, err := buildRolloutDraft("site-1", "", nil, results, observed)
	if err != nil {
		t.Fatal(err)
	}
	results[0].LastIP, results[1].LastIP = "192.0.2.200", "192.0.2.3"
	second, err := buildRolloutDraft("site-1", "", nil, results, observed)
	if err != nil {
		t.Fatal(err)
	}
	if rolloutPlanHash(first) != rolloutPlanHash(second) {
		t.Fatal("device IP changes must not change immutable rollout ordering")
	}
	if first.Devices[0].DeviceID != "device-a" || first.Devices[1].DeviceID != "device-b" {
		t.Fatalf("draft order = %#v, want deterministic device ID order", first.Devices)
	}
}

func TestDecodeRolloutDraftRejectsTamperedPlan(t *testing.T) {
	record := database.RolloutDraftRecord{
		SiteID:   "site-1",
		PlanHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Plan:     []byte(`{"site_id":"site-1","devices":[]}`),
	}
	if _, err := decodeRolloutDraft(record); err == nil {
		t.Fatal("tampered rollout plan was accepted")
	}
}

func TestRolloutRecordPreviewRejectsTamperedDraft(t *testing.T) {
	record := rolloutRecord{
		SiteID:   "site-1",
		PlanHash: strings.Repeat("a", 64),
		Plan:     json.RawMessage(`{"site_id":"site-1","devices":[]}`),
	}
	if previews := rolloutDevicePreviewsFromRecord(record); previews != nil {
		t.Fatalf("tampered rollout record exposed preview: %#v", previews)
	}
}

func TestBuildSingleDeviceChangeSetUsesImmutableDraftState(t *testing.T) {
	draft := rolloutDraft{
		Namespace:    "system",
		HealthChecks: []string{"1.1.1.1"},
		Devices: []rolloutDraftDevice{{
			DeviceID:      "device-1",
			Commands:      []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
			ObservedState: map[string]string{"system": strings.Repeat("a", 64)},
		}},
	}

	changeSet, err := buildSingleDeviceChangeSet("rollout-1", draft)
	if err != nil {
		t.Fatal(err)
	}
	if changeSet.DeviceID != "device-1" || changeSet.Generation != 0 || changeSet.Operations[0].Config != "system" {
		t.Fatalf("changeset = %#v", changeSet)
	}
	if changeSet.Operations[0].ObservedStateHash != strings.Repeat("a", 64) || changeSet.PlanHash == "" {
		t.Fatalf("changeset lost immutable state: %#v", changeSet)
	}
}

func TestBuildSingleDeviceChangeSetRejectsUnsupportedNamespace(t *testing.T) {
	draft := rolloutDraft{
		Devices: []rolloutDraftDevice{{
			DeviceID:      "device-1",
			Commands:      []services.UciCommand{{Action: "set", Config: "dhcp", Section: "lan", Option: "start", Value: "100"}},
			ObservedState: map[string]string{"dhcp": strings.Repeat("a", 64)},
		}},
	}
	if _, err := buildSingleDeviceChangeSet("rollout-1", draft); err == nil {
		t.Fatal("unsupported namespace was accepted")
	}
}

func TestBuildSingleDeviceChangeSetRequiresExplicitNamespace(t *testing.T) {
	draft := rolloutDraft{
		Devices: []rolloutDraftDevice{{
			DeviceID:      "device-1",
			Commands:      []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"}},
			ObservedState: map[string]string{"system": strings.Repeat("a", 64)},
		}},
	}
	if _, err := buildSingleDeviceChangeSet("rollout-1", draft); err == nil {
		t.Fatal("unscoped rollout draft entered the changeset slice")
	}
}

func TestBuildFleetDeviceChangeSetsUsesStablePerDeviceIdentities(t *testing.T) {
	draft := rolloutDraft{
		Namespace: "system",
		Devices: []rolloutDraftDevice{
			{DeviceID: "device-a", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "a"}}, ObservedState: map[string]string{"system": strings.Repeat("a", 64)}},
			{DeviceID: "device-b", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "b"}}, ObservedState: map[string]string{"system": strings.Repeat("b", 64)}},
		},
	}

	changeSets, err := buildFleetDeviceChangeSets("rollout-1", draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(changeSets) != 2 || changeSets[0].DeviceID != "device-a" || changeSets[1].DeviceID != "device-b" {
		t.Fatalf("changesets = %#v", changeSets)
	}
	if changeSets[0].ChangeSetID == changeSets[1].ChangeSetID || changeSets[0].Operations[0].OperationID == changeSets[1].Operations[0].OperationID {
		t.Fatalf("device identities were not separated: %#v", changeSets)
	}
	if changeSetsAgain, err := buildFleetDeviceChangeSets("rollout-1", draft); err != nil || changeSetsAgain[0].ChangeSetID != changeSets[0].ChangeSetID || changeSetsAgain[1].ChangeSetID != changeSets[1].ChangeSetID {
		t.Fatalf("changeset identities were not stable: %#v, %v", changeSetsAgain, err)
	}
}

func TestBuildFleetDeviceChangeSetsRejectsDuplicateDevices(t *testing.T) {
	draft := rolloutDraft{Namespace: "system", Devices: []rolloutDraftDevice{
		{DeviceID: "device-a", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "a"}}, ObservedState: map[string]string{"system": strings.Repeat("a", 64)}},
		{DeviceID: "DEVICE-A", Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "b"}}, ObservedState: map[string]string{"system": strings.Repeat("b", 64)}},
	}}
	if _, err := buildFleetDeviceChangeSets("rollout-1", draft); err == nil {
		t.Fatal("duplicate device target was accepted")
	}
}

func TestBuildDeviceChangeSetGroupsSelectedSafeNamespaces(t *testing.T) {
	draft := rolloutDraft{Namespace: "system,dhcp", Devices: []rolloutDraftDevice{{
		DeviceID: "device-a",
		Commands: []services.UciCommand{
			{Action: "set", Config: "dhcp", Section: "lan", Option: "start", Value: "100"},
			{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "lab-router"},
		},
		ObservedState: map[string]string{"system": strings.Repeat("a", 64), "dhcp": strings.Repeat("b", 64)},
	}}}

	changeSet, err := buildSingleDeviceChangeSet("rollout-1", draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(changeSet.Operations) != 2 || changeSet.Operations[0].Config != "system" || changeSet.Operations[1].Config != "dhcp" {
		t.Fatalf("operations = %#v, want deterministic system then dhcp order", changeSet.Operations)
	}
}

func TestVerifyRolloutDraftTargetsRejectsRoleChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(device_role, 'AP'), pending_operation, pending_change_set FROM "tenant_demo".devices WHERE id = $1 AND site_id = $2 AND COALESCE(last_operation->>'state', '') <> 'RECOVERY_REQUIRED' AND COALESCE(last_change_set->>'state', '') <> 'RECOVERY_REQUIRED'`)).
		WithArgs("device-a", "site-1").
		WillReturnRows(sqlmock.NewRows([]string{"device_role", "pending_operation", "pending_change_set"}).AddRow("Gateway", nil, nil))
	draft := rolloutDraft{
		SiteID:  "site-1",
		Devices: []rolloutDraftDevice{{DeviceID: "device-a", Role: "AP"}},
	}
	if err := verifyRolloutDraftTargets(t.Context(), "tenant_demo", "site-1", draft); !errors.Is(err, errRolloutDraftStale) {
		t.Fatalf("role change error = %v, want stale error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestedRolloutIDAcceptsBodyAndQuery(t *testing.T) {
	bodyRequest := httptest.NewRequest("POST", "/?", bytes.NewBufferString(`{"rollout_id":"00000000-0000-0000-0000-000000000001"}`))
	if got, err := requestedRolloutID(bodyRequest); err != nil || got != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("body rollout ID = %q, %v", got, err)
	}

	queryRequest := httptest.NewRequest("POST", "/?rollout_id=00000000-0000-0000-0000-000000000002", nil)
	if got, err := requestedRolloutID(queryRequest); err != nil || got != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("query rollout ID = %q, %v", got, err)
	}
	uppercaseRequest := httptest.NewRequest("POST", "/?rollout_id=00000000-0000-0000-0000-0000000000AB", nil)
	if got, err := requestedRolloutID(uppercaseRequest); err != nil || got != "00000000-0000-0000-0000-0000000000ab" {
		t.Fatalf("uppercase rollout ID = %q, %v", got, err)
	}
}

func TestRequestedRolloutIDRequiresAnID(t *testing.T) {
	request := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{}`))
	if _, err := requestedRolloutID(request); err == nil {
		t.Fatal("missing rollout ID was accepted")
	}
	invalid := httptest.NewRequest("POST", "/?rollout_id=not-a-uuid", nil)
	if _, err := requestedRolloutID(invalid); err == nil {
		t.Fatal("invalid rollout ID was accepted")
	}
}

func TestBuildGuardedRolloutScriptChecksObservedStateBeforeMutation(t *testing.T) {
	command := services.UciCommand{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "router-a"}
	script, err := buildGuardedRolloutScript("system", []services.UciCommand{command}, nil, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "observed_rollout_hash=$(uci show system") || !strings.Contains(script, "exit 75") {
		t.Fatalf("script has no stale-state guard: %s", script)
	}
	if strings.Index(script, "observed_rollout_hash=") > strings.Index(script, "uci set") {
		t.Fatal("stale-state guard must precede UCI mutation")
	}
	if strings.Index(script, "observed_rollout_hash=") > strings.Index(script, "# Phase 2: Apply UCI mutations") {
		t.Fatal("stale-state guard must run before the mutation phase")
	}
	if strings.Index(script, `mkdir "$uci_lock_dir"`) > strings.Index(script, "observed_rollout_hash=") {
		t.Fatal("UCI lock must protect the observed-state check")
	}
}

func TestSyncFleetRequiresAnImmutableDraft(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/sites/site-1/orchestrator/sync", bytes.NewBufferString(`{}`))
	request.SetPathValue("site_id", "site-1")
	recorder := httptest.NewRecorder()
	SyncFleetHandler(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestPreviewSyncRejectsUnsafeNamespaceBeforeDatabaseAccess(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/sites/site-1/orchestrator/preview?namespace=network", nil)
	request.SetPathValue("site_id", "site-1")
	recorder := httptest.NewRecorder()
	PreviewSyncHandler(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "not supported") {
		t.Fatalf("status=%d body=%q, want early unsupported namespace rejection", recorder.Code, recorder.Body.String())
	}
}

func TestSyncFleetReturnsNotFoundForMissingImmutableDraft(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id::text, site_id::text, generation, status, claim_token::text, plan_hash, requested_by, target_device_ids, plan, results FROM public.rollout_runs WHERE id = $1 AND site_id = $2")).
		WithArgs("00000000-0000-0000-0000-000000000001", "site-1").
		WillReturnError(sql.ErrNoRows)

	request := httptest.NewRequest("POST", "/api/sites/site-1/orchestrator/sync?rollout_id=00000000-0000-0000-0000-000000000001", nil)
	request.SetPathValue("site_id", "site-1")
	recorder := httptest.NewRecorder()
	SyncFleetHandler(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%q, want missing draft", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMarkRolloutDevicesRunningDoesNotUseRolloutSequence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM public.rollout_runs WHERE id = $1 AND claim_token::text = $2 FOR UPDATE")).
		WithArgs("rollout-1", "claim-token").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("RUNNING"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE public.devices SET last_rollout_status = 'RUNNING'")).
		WithArgs("site-1", "device-a", "AP").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	request := httptest.NewRequest("POST", "/api/sites/site-1/orchestrator/sync", nil)
	if err := markRolloutDevicesRunning(request, "site-1", "rollout-1", "claim-token", []rolloutDraftDevice{{DeviceID: "device-a", Role: "AP"}}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistRolloutResultsDoesNotAdvanceDeviceGeneration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previousDB := database.DB
	database.DB = db
	defer func() { database.DB = previousDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM public.rollout_runs WHERE id = $1 AND claim_token::text = $2 FOR UPDATE")).
		WithArgs("rollout-1", "claim-token").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("RUNNING"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE public.devices SET last_rollout_status = $1, last_rollout_at = CURRENT_TIMESTAMP WHERE site_id = $2 AND id = $3 AND last_rollout_status = 'RUNNING'")).
		WithArgs("SUCCESS", "site-1", "device-a").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE public.rollout_runs SET status = $1, results = $2, claim_token = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $3 AND status = 'RUNNING' AND claim_token::text = $4")).
		WithArgs("completed", sqlmock.AnyArg(), "rollout-1", "claim-token").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	request := httptest.NewRequest("POST", "/api/sites/site-1/orchestrator/sync", nil)
	if err := persistRolloutResults(request, "site-1", "rollout-1", "claim-token", "completed", []fleetSyncResult{{DeviceID: "device-a", Status: "SUCCESS"}}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
