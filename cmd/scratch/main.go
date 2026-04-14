package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgres://postgres:postgres@localhost:5432/openwrthub?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	content, err := os.ReadFile("devices/agent.sh")
	if err != nil {
		log.Fatal(err)
	}

	rows, err := db.Query("SELECT id FROM sites")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var siteID string
		if err := rows.Scan(&siteID); err != nil {
			log.Fatal(err)
		}

		siteContent := append(content, []byte("\n# SITE: "+siteID)...)
		siteHash := sha256.Sum256(siteContent)
		siteHashStr := hex.EncodeToString(siteHash[:])

		// Disable active versions
		db.Exec("UPDATE agent_versions SET is_active = false WHERE site_id = $1", siteID)

		// Insert new version
		_, err = db.Exec(`
			INSERT INTO agent_versions (version_hash, script_content, is_active, site_id) 
			VALUES ($1, $2, true, $3)
			ON CONFLICT (version_hash) DO UPDATE SET is_active = true
		`, siteHashStr, string(siteContent), siteID)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Deployed to site: %s\n", siteID)
	}
}
