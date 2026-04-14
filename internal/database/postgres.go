package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_ip VARCHAR(50);
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS agent_version VARCHAR(64);
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS wg_pubkey VARCHAR(255);
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS wg_privkey VARCHAR(255);
	ALTER TABLE devices ADD COLUMN IF NOT EXISTS wg_ip VARCHAR(50);

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
		role VARCHAR(50) DEFAULT 'VIEWER',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS incidents (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES sites(id),
		device_id VARCHAR(50) REFERENCES devices(id),
		incident_type VARCHAR(50) NOT NULL,
		severity VARCHAR(20) NOT NULL,
		status VARCHAR(20) DEFAULT 'OPEN',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		resolved_at TIMESTAMP WITH TIME ZONE
	);

	CREATE TABLE IF NOT EXISTS profiles (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		description TEXT,
		config_json JSONB DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS backups (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		device_id VARCHAR(50) REFERENCES devices(id),
		checksum VARCHAR(64) NOT NULL,
		content BYTEA,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS firmwares (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		filename VARCHAR(255) NOT NULL,
		version VARCHAR(50),
		model_compatibility VARCHAR(50),
		data BYTEA,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_versions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		version_hash VARCHAR(64) UNIQUE NOT NULL,
		script_content TEXT NOT NULL,
		is_active BOOLEAN DEFAULT false,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS system_logs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		device_id VARCHAR(50) REFERENCES devices(id) ON DELETE CASCADE,
		log_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
		severity VARCHAR(20) NOT NULL,
		message TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- In Postgres, GIN index on text usually requires pg_trgm
	CREATE EXTENSION IF NOT EXISTS pg_trgm;
	CREATE INDEX IF NOT EXISTS trgm_idx_system_logs_message ON system_logs USING gin (message gin_trgm_ops);

	CREATE TABLE IF NOT EXISTS client_hostnames (
		mac VARCHAR(50) PRIMARY KEY,
		site_id UUID REFERENCES sites(id),
		hostname VARCHAR(255) NOT NULL,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- Migrate existing tables safely (idempotent)
	ALTER TABLE sites ADD COLUMN IF NOT EXISTS profile_id UUID REFERENCES profiles(id);
	ALTER TABLE sites ADD COLUMN IF NOT EXISTS wg_endpoint VARCHAR(255);
	ALTER TABLE sites ADD COLUMN IF NOT EXISTS wg_pubkey VARCHAR(255);
	ALTER TABLE sites ADD COLUMN IF NOT EXISTS wg_privkey VARCHAR(255);

	CREATE TABLE IF NOT EXISTS ai_insights (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		correlation_id VARCHAR(100),
		diagnosis TEXT,
		severity VARCHAR(20),
		involved_devices JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS platform_settings (
		id INT PRIMARY KEY DEFAULT 1,
		ollama_host VARCHAR(255) DEFAULT '127.0.0.1:11434',
		ollama_model VARCHAR(255) DEFAULT 'llama3',
		sentinel_prompt TEXT DEFAULT 'You are a Fleet Security Analyst. Analyze this cross-device log stream. Look for coordinated attacks, lateral movements, or cascading hardware failures. If Device A shows a login failure and Device B shows a login success from the same IP, flag it as CRITICAL SUSPICION. Be technical, concise, and provide a ''Recommended Action''. The output must look like a high-level SOC report. No fluff.\n\nEnd your report with these two exact lines at the bottom for parsing:\nSEVERITY: [Critical, High, Medium, Low]\nDEVICES: [Device_Name_1, Device_Name_2]',
		telegram_bot_token VARCHAR(255) DEFAULT '',
		telegram_chat_id VARCHAR(255) DEFAULT '',
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		CHECK (id = 1)
	);

	CREATE TABLE IF NOT EXISTS shaping_rules (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		device_id VARCHAR(50) REFERENCES devices(id) ON DELETE CASCADE,
		mac VARCHAR(50) NOT NULL,
		rate_mbytes INT NOT NULL,
		expires_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(device_id, mac)
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		username VARCHAR(100) NOT NULL,
		action VARCHAR(255) NOT NULL,
		resource_type VARCHAR(100),
		resource_id VARCHAR(255),
		payload TEXT,
		ip_addr VARCHAR(50),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	INSERT INTO platform_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

	CREATE TABLE IF NOT EXISTS threat_intel_meta (
		id SERIAL PRIMARY KEY,
		fetched_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		ip_count INTEGER NOT NULL DEFAULT 0,
		sources_count INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS site_configs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES sites(id) ON DELETE CASCADE UNIQUE,
		global_ssid VARCHAR(255) DEFAULT '',
		global_wpa_key VARCHAR(255) DEFAULT '',
		global_encryption VARCHAR(50) DEFAULT 'psk2',
		lan_ipaddr VARCHAR(50) DEFAULT '192.168.1.1',
		lan_netmask VARCHAR(50) DEFAULT '255.255.255.0',
		dhcp_start INT DEFAULT 100,
		dhcp_limit INT DEFAULT 150,
		dhcp_leasetime VARCHAR(20) DEFAULT '12h',
		dns_primary VARCHAR(50) DEFAULT '9.9.9.9',
		dns_secondary VARCHAR(50) DEFAULT '1.1.1.1',
		timezone VARCHAR(100) DEFAULT 'UTC',
		hostname_prefix VARCHAR(100) DEFAULT 'nerve',
		firewall_syn_flood BOOLEAN DEFAULT true,
		firewall_drop_invalid BOOLEAN DEFAULT true,
		dropbear_port INT DEFAULT 22,
		dropbear_password_auth BOOLEAN DEFAULT true,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Idempotent migrations
	migrations := []string{
		"ALTER TABLE sites ADD COLUMN IF NOT EXISTS api_key TEXT UNIQUE",
		"ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS site_id UUID REFERENCES sites(id)",
		"UPDATE users SET role = UPPER(role)",
		"ALTER TABLE ai_insights ADD COLUMN IF NOT EXISTS llm_model VARCHAR(255)",
		"ALTER TABLE ai_insights ADD COLUMN IF NOT EXISTS tokens_used INT DEFAULT 0",
		"ALTER TABLE sites ADD COLUMN IF NOT EXISTS auto_adopt BOOLEAN DEFAULT false",
		"ALTER TABLE sites ADD COLUMN IF NOT EXISTS threat_shield_enabled BOOLEAN DEFAULT false",
		"ALTER TABLE devices ADD COLUMN IF NOT EXISTS threat_shield_drops BIGINT DEFAULT 0",
		"ALTER TABLE devices ADD COLUMN IF NOT EXISTS device_role VARCHAR(50) DEFAULT 'AP'",
		"ALTER TABLE wlans ADD COLUMN IF NOT EXISTS roaming_enabled BOOLEAN DEFAULT false",
	}
	for _, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			return fmt.Errorf("migration failed (%s): %w", m, err)
		}
	}

	if err := seedAdminUser(); err != nil {
		return err
	}
	return seedSiteAPIKeys()
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
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'ADMIN')",
		"admin", string(hash),
	)
	if err != nil {
		return fmt.Errorf("failed to seed admin user: %w", err)
	}
	log.Println("Bootstrap admin user created (username: admin)")
	return nil
}

func seedSiteAPIKeys() error {
	rows, err := DB.Query("SELECT id, name FROM sites WHERE api_key IS NULL OR api_key = ''")
	if err != nil {
		return err
	}
	defer rows.Close()

	var updates []struct{ id, name, key string }
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		b := make([]byte, 16)
		rand.Read(b)
		key := hex.EncodeToString(b)
		updates = append(updates, struct{ id, name, key string }{id, name, key})
	}

	for _, u := range updates {
		_, err := DB.Exec("UPDATE sites SET api_key = $1 WHERE id = $2", u.key, u.id)
		if err != nil {
			return err
		}
		fmt.Printf("SITIO: [%s] | API_KEY: [%s]\n", u.name, u.key)
	}
	return nil
}

func UpsertDeviceState(deviceID string, stateJSON []byte, model string, lastIP string, agentVersion string) error {
	query := `
		INSERT INTO devices (id, state_json, model, last_ip, agent_version, last_seen_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET 
			state_json = EXCLUDED.state_json,
			model = EXCLUDED.model,
			last_ip = EXCLUDED.last_ip,
			agent_version = EXCLUDED.agent_version,
			last_seen_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := DB.Exec(query, deviceID, stateJSON, model, lastIP, agentVersion)
	return err
}

type LogEntry struct {
	Timestamp  string `json:"timestamp"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

func InsertDeviceLogs(deviceID string, logs []LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		WITH input AS (
			SELECT CAST($1 AS VARCHAR) as device_id,
			       CAST($2 AS TIMESTAMP WITH TIME ZONE) as log_timestamp,
			       CAST($3 AS VARCHAR) as severity,
			       CAST($4 AS TEXT) as message
		)
		INSERT INTO system_logs (device_id, log_timestamp, severity, message)
		SELECT device_id, log_timestamp, severity, message FROM input
		WHERE NOT EXISTS (
			SELECT 1 FROM system_logs 
			WHERE device_id = input.device_id 
			  AND log_timestamp = input.log_timestamp 
			  AND message = input.message
		)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, logLine := range logs {
		_, err := stmt.Exec(deviceID, logLine.Timestamp, logLine.Level, logLine.Message)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
