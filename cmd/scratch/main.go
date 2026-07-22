package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

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

	schemaRows, err := db.Query(`
		SELECT nspname
		FROM pg_namespace
		WHERE nspname = 'public' OR nspname LIKE 'tenant_%'
		ORDER BY nspname
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer schemaRows.Close()

	for schemaRows.Next() {
		var schema string
		if err := schemaRows.Scan(&schema); err != nil {
			log.Fatal(err)
		}

		sitesTable := quoteIdentifier(schema) + ".sites"
		versionsTable := quoteIdentifier(schema) + ".agent_versions"
		siteRows, err := db.Query("SELECT id::text FROM " + sitesTable + " ORDER BY id")
		if err != nil {
			log.Printf("Schema %s skipped: cannot read sites: %v", schema, err)
			continue
		}

		for siteRows.Next() {
			var siteID string
			if err := siteRows.Scan(&siteID); err != nil {
				log.Printf("Schema %s skipped site: %v", schema, err)
				continue
			}

			siteContent := make([]byte, 0, len(content)+len(siteID)+8)
			siteContent = append(siteContent, content...)
			siteContent = append(siteContent, []byte("\n# SITE: "+siteID)...)
			siteHash := sha256.Sum256(siteContent)
			siteHashStr := hex.EncodeToString(siteHash[:])

			if _, err := db.Exec("UPDATE "+versionsTable+" SET is_active = false WHERE site_id = $1", siteID); err != nil {
				log.Printf("Schema %s site %s deactivate warning/error: %v", schema, siteID, err)
				continue
			}

			_, err = db.Exec(fmt.Sprintf(`
				INSERT INTO %s (version_hash, script_content, is_active, site_id)
				VALUES ($1, $2, true, $3)
				ON CONFLICT (version_hash) DO UPDATE
				SET script_content = EXCLUDED.script_content,
				    is_active = true,
				    site_id = EXCLUDED.site_id
			`, versionsTable), siteHashStr, string(siteContent), siteID)
			if err != nil {
				log.Printf("Schema %s deploy warning/error: %v\n", schema, err)
				continue
			}
			log.Printf("Deployed to schema %s site %s: %s", schema, siteID, siteHashStr)
		}
		if err := siteRows.Err(); err != nil {
			log.Printf("Schema %s site enumeration error: %v", schema, err)
		}
		siteRows.Close()
	}
	if err := schemaRows.Err(); err != nil {
		log.Fatal(err)
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
