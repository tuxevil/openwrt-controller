package main

import (
	"fmt"
	"log"
	
	"openwrt-controller/internal/services"
	"openwrt-controller/internal/database"
)

func main() {
	if err := database.InitPostgres(); err != nil {
		log.Fatal(err)
	}

	devs, err := services.GetSiteDevicesWithRoles("00000000-0000-0000-0000-000000000000")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("found %d devices\n", len(devs))
	for _, d := range devs {
		fmt.Printf(" -> %+v\n", d)
	}
}
