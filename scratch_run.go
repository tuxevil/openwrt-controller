package main

import (
	"fmt"
	"log"
	"os"
	"openwrt-controller/internal/database"
)

func main() {
	os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/openwrthub")
	err := database.InitPostgres()
	if err != nil {
		log.Fatal(err)
	}

	siteKey := "a29c9e4cb27c6a350b7d0d9dbf70ac03"

	schema, err := database.GetTenantSchemaForSiteKey(siteKey)
	if err != nil {
		fmt.Printf("GetTenantSchemaForSiteKey err: %v\n", err)
		return
	}
	fmt.Printf("GetTenantSchemaForSiteKey: %s\n", schema)

	var siteID string
	err = database.DB.QueryRow(
		"SELECT id FROM public.sites WHERE api_key = $1", siteKey,
	).Scan(&siteID)
	if err != nil {
		fmt.Printf("QueryRow public.sites err: %v\n", err)
		return
	}
	fmt.Printf("siteID: %s\n", siteID)

	var id, versionHash, scriptContent string
	var isActive bool
	var createdAt interface{}
	err = database.DB.QueryRow(fmt.Sprintf(`
		SELECT id, version_hash, script_content, is_active, created_at 
		FROM %s.agent_versions 
		WHERE is_active = true AND site_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, schema), siteID).Scan(&id, &versionHash, &scriptContent, &isActive, &createdAt)
	if err != nil {
		fmt.Printf("agent_versions query err: %v\n", err)
		return
	}
	fmt.Printf("id: %s, hash: %s, active: %t, created: %v\n", id, versionHash, isActive, createdAt)
}
