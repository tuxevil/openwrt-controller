package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
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
		device_token VARCHAR(255),
		last_config_pulled_at TIMESTAMP WITH TIME ZONE,
		last_seen_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- Migrate existing tables safely (idempotent)
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS device_token VARCHAR(255);
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_config_pulled_at TIMESTAMP WITH TIME ZONE;

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

	CREATE TABLE IF NOT EXISTS site_settings (
		site_id UUID PRIMARY KEY REFERENCES sites(id),
		dns_servers VARCHAR(255) DEFAULT '9.9.9.9,1.1.1.1',
		dhcp_server BOOLEAN DEFAULT true,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		username VARCHAR(100) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		role VARCHAR(50) DEFAULT 'viewer',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return seedAdminUser()
}

func seedAdminUser() error {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil || count > 0 {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("REPLACE_WITH_BOOTSTRAP_PASSWORD"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash bootstrap password: %w", err)
	}
	_, err = DB.Exec(
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'admin')",
		"admin", string(hash),
	)
	if err != nil {
		return fmt.Errorf("failed to seed admin user: %w", err)
	}
	log.Println("Bootstrap admin user created (username: admin)")
	return nil
}

func UpsertDeviceState(deviceID string, stateJSON []byte, model string) error {
	query := `
		INSERT INTO devices (id, state_json, model, last_seen_at, updated_at) 
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) 
		DO UPDATE SET 
			state_json = EXCLUDED.state_json, 
			model = EXCLUDED.model,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at;
	`
	_, err := DB.Exec(query, deviceID, string(stateJSON), model)
	return err
}
