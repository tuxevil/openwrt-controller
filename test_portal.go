package main

import (
	"fmt"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func main() {
	if err := database.InitPostgres(); err != nil {
		fmt.Printf("Init error: %v\n", err)
	}
	
	s := services.PortalSettings{
		SiteID: "00000000-0000-0000-0000-000000000000", // existing site from logs
		Enabled: true,
		WelcomeText: "Hello",
		TermsText: "Test",
		BgColor: "#0a0a0a",
		LogoURL: "",
		RedirectURL: "",
	}
	err := services.UpsertPortalSettings(s)
	if err != nil {
		fmt.Printf("DB Error: %v\n", err)
	} else {
		fmt.Println("Success")
	}
}
