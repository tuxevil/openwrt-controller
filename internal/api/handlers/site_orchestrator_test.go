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
