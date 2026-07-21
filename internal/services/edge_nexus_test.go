package services

import "testing"

func TestValidateEdgeUCIIdentifiers(t *testing.T) {
	if err := ValidateNetworkInterfaces([]NetworkInterface{{Name: "lan; reboot", Proto: "dhcp"}}); err == nil {
		t.Fatal("accepted a shell fragment as a network interface name")
	}
	if err := ValidateDHCPInterfaces([]DHCPInterface{{Interface: "lan$(id)"}}); err == nil {
		t.Fatal("accepted a shell fragment as a DHCP interface name")
	}
	if err := ValidateNetworkInterfaces([]NetworkInterface{{Name: "lan", Proto: "dhcp"}}); err != nil {
		t.Fatalf("rejected valid network interface: %v", err)
	}
}
