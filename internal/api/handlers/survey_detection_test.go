package handlers

import (
	"net/netip"
	"testing"
)

func TestDetectSurveyMAC(t *testing.T) {
	const arp = `{"neighbor_stats":{"arp_table":[{"ip":"192.168.1.45","mac":"06:11:22:33:44:55"}]}}`
	const stations = `{"wireless_stations":{"phy0-ap0":[{"mac":"06:11:22:33:44:55"}],"phy1-ap0":[{"mac":"02:aa:bb:cc:dd:ee"}]}}`
	const conflict = `{"arp_table":[{"ip":"192.168.1.45","mac":"02:AA:BB:CC:DD:EE"}]}`
	for _, tc := range []struct {
		name   string
		states []string
		want   string
	}{
		{"gateway and AP", []string{arp, stations}, "06:11:22:33:44:55"},
		{"same client on two APs", []string{arp, arp, stations, stations}, "06:11:22:33:44:55"},
		{"wired only", []string{arp}, ""},
		{"no IP evidence", []string{stations}, ""},
		{"ambiguous IP", []string{arp, stations, conflict}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var states [][]byte
			for _, state := range tc.states {
				states = append(states, []byte(state))
			}
			if got := detectSurveyMAC(netip.MustParseAddr("192.168.1.45"), states); got != tc.want {
				t.Fatalf("MAC = %q, want %q", got, tc.want)
			}
		})
	}
}
