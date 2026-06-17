package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		panic("DATABASE_URL is required (e.g. postgres://user:pass@host:5432/db)")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("cannot reach database: %v", err))
	}
	defer db.Close()

	siteID := "2dc5179f-290b-4997-9528-213b75f8087d"
	rows, err := db.Query("SELECT id, name, state_json FROM devices WHERE site_id = $1", siteID)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		var stateJSON []byte
		rows.Scan(&id, &name, &stateJSON)

		fmt.Printf("\n====================\nDEVICE: %s (%s)\n", id, name)

		var dev map[string]interface{}
		if err := json.Unmarshal(stateJSON, &dev); err != nil {
			fmt.Println("Error unmarshaling json:", err)
			continue
		}

		if wStations, ok := dev["wireless_stations"].(map[string]interface{}); ok {
			for iface, stationsObj := range wStations {
				if stations, ok := stationsObj.([]interface{}); ok {
					for _, stObj := range stations {
						if st, ok := stObj.(map[string]interface{}); ok {
							fmt.Printf("  Wireless: %s -> %s\n", iface, st["mac"])
						}
					}
				}
			}
		}
	}
}
