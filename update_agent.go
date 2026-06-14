package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"crypto/sha256"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		dbUrl = "postgres://postgres:postgres@localhost:5432/openwrthub"
	}
	pool, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	contentBytes, err := os.ReadFile("devices/agent.sh")
	if err != nil {
		log.Fatal(err)
	}
	content := string(contentBytes)
	hash := sha256.Sum256(contentBytes)
	hashHex := hex.EncodeToString(hash[:])

	_, err = pool.Exec(context.Background(), "INSERT INTO tenant_example.agent_versions (version_hash, script_content, is_active) VALUES ($1, $2, true)", hashHex, content)
	if err != nil {
		log.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), "UPDATE tenant_example.agent_versions SET is_active = false WHERE version_hash != $1", hashHex)
	
	fmt.Println("Agent updated in database")
}
