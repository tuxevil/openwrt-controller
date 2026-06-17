package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required (e.g. postgres://user:pass@host:5432/db?sslmode=disable)")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("cannot reach database: %v", err)
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

		// Disable active versions and insert/update for all schemas:
		schemas := []string{"public", "tenant_example"}
		for _, schema := range schemas {
			db.Exec(fmt.Sprintf("UPDATE %s.agent_versions SET is_active = false WHERE site_id = $1", schema), siteID)
			_, err = db.Exec(fmt.Sprintf(`
				INSERT INTO %s.agent_versions (version_hash, script_content, is_active, site_id) 
				VALUES ($1, $2, true, $3)
				ON CONFLICT (version_hash) DO UPDATE SET is_active = true
			`, schema), siteHashStr, string(siteContent), siteID)
			if err != nil {
				log.Printf("Schema %s deploy warning/error: %v\n", schema, err)
			}
		}
		log.Printf("Deployed to site: %s\n", siteID)
	}
}
