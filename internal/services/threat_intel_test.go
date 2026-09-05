package services

import "testing"

func TestSafeThreatCIDRRejectsNonPublicRanges(t *testing.T) {
	for _, cidr := range []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"not-an-ip/24",
	} {
		if safeThreatCIDR(cidr) {
			t.Errorf("safeThreatCIDR(%q) = true, want false", cidr)
		}
	}
}

func TestSafeThreatCIDRAcceptsPublicRanges(t *testing.T) {
	for _, cidr := range []string{"1.10.16.0/20", "8.8.8.8/32", "185.220.101.0/24"} {
		if !safeThreatCIDR(cidr) {
			t.Errorf("safeThreatCIDR(%q) = false, want true", cidr)
		}
	}
}

func TestSafeThreatCIDRRejectsRangesCrossingExcludedNetwork(t *testing.T) {
	for _, cidr := range []string{"9.0.0.0/6", "192.0.0.0/8"} {
		if safeThreatCIDR(cidr) {
			t.Errorf("safeThreatCIDR(%q) = true, want false", cidr)
		}
	}
}
