package main

import (
	"log"
	"openwrt-controller/internal/database"
)

func main() {
	if err := database.InitPostgres(); err != nil {
		log.Fatal(err)
	}

	_, err := database.DB.Exec(`
		INSERT INTO ai_insights (correlation_id, diagnosis, severity, involved_devices)
		VALUES ($1, $2, $3, $4)
	`, "AI-CORR-99999", "Simulated SOC Alert: Multiple failed SSH login attempts detected from internal subnet (10.0.0.144). The Sentinel pipeline has successfully intercepted the threat and deployed the Preventive Sniper Shaping module to contain lateral movement.", "Critical", `["AP-Lobby", "AP-Office"]`)

	if err != nil {
		log.Fatal(err)
	}
	log.Println("Inserted simulation insight!")
}
