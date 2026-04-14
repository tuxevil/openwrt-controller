package main

import (
	"fmt"
	"os"

	"openwrt-controller/internal/services"
)

func main() {
	b, err := os.ReadFile("docs/omada-backups/omada_dhcp.json")
	if err != nil {
		panic(err)
	}
	dhcp, fw, err := services.ParseOmadaExport(b)
	fmt.Printf("DHCP: %d, FW: %d, ERR: %v\n", len(dhcp), len(fw), err)

	b2, err := os.ReadFile("docs/omada-backups/omada_ports.json")
	if err != nil {
		panic(err)
	}
	dhcp2, fw2, err := services.ParseOmadaExport(b2)
	fmt.Printf("DHCP: %d, FW: %d, ERR: %v\n", len(dhcp2), len(fw2), err)
}
