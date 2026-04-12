package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var DB *sql.DB

func InitPostgres() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/openwrthub"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open pgx connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping postgres: %w", err)
	}

	DB = db
	log.Println("PostgreSQL initialized successfully")

	return createTables()
}

func createTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS controllers (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		mac VARCHAR(50) UNIQUE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sites (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		controller_id UUID REFERENCES controllers(id),
		name VARCHAR(255) NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS devices (
		id VARCHAR(50) PRIMARY KEY, -- MAC Address
		site_id UUID REFERENCES sites(id),
		name VARCHAR(255),
		model VARCHAR(255),
		status VARCHAR(50),
		state_json JSONB,
		last_seen_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS wlans (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES sites(id),
		ssid VARCHAR(255) NOT NULL,
		security VARCHAR(50) NOT NULL,
		password VARCHAR(255),
		enabled BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

func UpsertDeviceState(deviceID string, stateJSON []byte) error {
	query := `
		INSERT INTO devices (id, state_json, last_seen_at, updated_at) 
		VALUES ($1, $2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) 
		DO UPDATE SET 
			state_json = EXCLUDED.state_json, 
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at;
	`
	_, err := DB.Exec(query, deviceID, string(stateJSON))
	return err
}
