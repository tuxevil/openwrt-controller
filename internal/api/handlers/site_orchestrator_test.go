package handlers

import (
	"strings"
	"testing"

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

func TestFleetPlanHashIsStable(t *testing.T) {
	first := fleetPlanHash([]services.RenderResult{{
		DeviceID: "device-a",
		Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "router-a"}},
	}})
	second := fleetPlanHash([]services.RenderResult{{
		DeviceID: "device-a",
		Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "router-a"}},
	}})
	if first == "" || first != second {
		t.Fatalf("plan hash is not stable: %q != %q", first, second)
	}
	changed := []services.RenderResult{{
		DeviceID: "device-a",
		Commands: []services.UciCommand{{Action: "set", Config: "system", Section: "@system[0]", Option: "hostname", Value: "router-b"}},
	}}
	if first == fleetPlanHash(changed) {
		t.Fatal("different plans must have different hashes")
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
		t.Fatal("successful and no-op rollout results should both advance generation")
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
