package main

import (
	"log"
	"net/http"

	"openwrt-controller/internal/api"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func main() {
	// Initialize PostgreSQL
	if err := database.InitPostgres(); err != nil {
		log.Printf("Warning: Postgres init failed: %v\n", err)
	}

	// Initialize InfluxDB
	if err := database.InitInflux(); err != nil {
		log.Printf("Warning: Influx config/init failed: %v\n", err)
	}
	defer database.CloseInflux()

	services.StartAlertEngine()

	// Setup routes using the dedicated routes file
	mux := api.SetupRoutes()

	port := ":3000"
	log.Printf("Starting openwrt-controller on port %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
