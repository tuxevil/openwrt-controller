package handlers

import (
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
