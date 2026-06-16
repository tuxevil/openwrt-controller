package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

func InitPostgres() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required (no default credentials allowed)")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open pgx connection: %w", err)
	}

	// Pool tuning. Without explicit limits a chatty dashboard can exhaust
	// the connection pool and lock out background workers (alerts, vault
	// cron, threat-intel fetcher). The defaults below are conservative for
	// a single-tenant deployment; multi-tenant / large fleets should tune
	// via PG_MAX_OPEN_CONNS / PG_MAX_IDLE_CONNS.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// Retry loop for ping
	for i := 0; i < 10; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		log.Printf("Waiting for postgres... (%d/10)", i+1)
		time.Sleep(2 * time.Second)
	}

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping postgres after retries: %w", err)
	}

	DB = db
	log.Println("PostgreSQL initialized successfully")

	if err := createLandlordTables(); err != nil {
		return fmt.Errorf("failed to create landlord tables: %w", err)
	}

	if err := runMigrationsForAllTenants(); err != nil {
		log.Printf("Warning: tenant migrations failed: %v", err)
	}

	return nil
}

// ─── LANDLORD SCHEMA (public) ────────────────────────────────────────────────
// Global tables that live in the public schema: users, tenants, platform_settings, audit_logs

func createLandlordTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS tenants (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		schema_alias VARCHAR(100) UNIQUE NOT NULL,
		is_active BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		username VARCHAR(100) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		role VARCHAR(50) DEFAULT 'VIEWER',
		tenant_id UUID REFERENCES tenants(id),
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
	`
	_, err := DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create landlord tables: %w", err)
	}

	// Idempotent landlord migrations
	landlordMigrations := []string{
		"ALTER TABLE users ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id)",
		"UPDATE users SET role = UPPER(role)",
	}
	for _, m := range landlordMigrations {
		if _, err := DB.Exec(m); err != nil {
			return fmt.Errorf("landlord migration failed (%s): %w", m, err)
		}
	}

	if err := seedSuperAdminUser(); err != nil {
		return err
	}

	return nil
}

// ─── TENANT SCHEMA (isolated) ────────────────────────────────────────────────
// All operational tables are created inside the tenant-specific schema.

// RunTenantMigrations creates all operational tables inside the given schema.
// schemaAlias should be the full schema name (e.g., "tenant_example").
func RunTenantMigrations(schemaAlias string) error {
	if !isValidSchemaName(schemaAlias) {
		return fmt.Errorf("invalid schema alias: %s", schemaAlias)
	}

	// Create the schema
	_, err := DB.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaAlias))
	if err != nil {
		return fmt.Errorf("failed to create schema %s: %w", schemaAlias, err)
	}

	return createTenantTables(schemaAlias)
}

func createTenantTables(schema string) error {
	// Prefix all table names with the schema
	s := schema

	query := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %[1]s.controllers (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		mac VARCHAR(50) UNIQUE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.sites (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		controller_id UUID REFERENCES %[1]s.controllers(id),
		name VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.devices (
		id VARCHAR(50) PRIMARY KEY,
		site_id UUID REFERENCES %[1]s.sites(id),
		name VARCHAR(255),
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		model VARCHAR(255),
		status VARCHAR(50),
		state_json JSONB,
		device_token VARCHAR(255),
		last_config_pulled_at TIMESTAMP WITH TIME ZONE,
		last_seen_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.wlans (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES %[1]s.sites(id),
		ssid VARCHAR(255) NOT NULL,
		security VARCHAR(50) NOT NULL,
		password VARCHAR(255),
		enabled BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		ieee80211w VARCHAR(10) DEFAULT '0',
		auth_server VARCHAR(50),
		auth_secret VARCHAR(255),
		dynamic_vlan VARCHAR(10) DEFAULT '0',
		band VARCHAR(50) DEFAULT 'both',
		target_mode VARCHAR(50) DEFAULT 'all',
		roaming_enabled BOOLEAN DEFAULT false,
		ieee80211k BOOLEAN DEFAULT false,
		ieee80211v BOOLEAN DEFAULT false
	);

	CREATE TABLE IF NOT EXISTS %[1]s.device_wlans (

		wlan_id UUID REFERENCES %[1]s.wlans(id) ON DELETE CASCADE,

		device_id VARCHAR(50) REFERENCES %[1]s.devices(id) ON DELETE CASCADE,

		PRIMARY KEY (wlan_id, device_id)

	);

	CREATE TABLE IF NOT EXISTS %[1]s.site_settings (
		site_id UUID PRIMARY KEY REFERENCES %[1]s.sites(id),
		dns_servers VARCHAR(255) DEFAULT '9.9.9.9,1.1.1.1',
		dhcp_server BOOLEAN DEFAULT true,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.incidents (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES %[1]s.sites(id),
		device_id VARCHAR(50) REFERENCES %[1]s.devices(id),
		incident_type VARCHAR(50) NOT NULL,
		severity VARCHAR(20) NOT NULL,
		status VARCHAR(20) DEFAULT 'OPEN',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		resolved_at TIMESTAMP WITH TIME ZONE
	);

	CREATE TABLE IF NOT EXISTS %[1]s.vpn_meshes (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		topology VARCHAR(50) DEFAULT 'hub_and_spoke',
		hub_device_id VARCHAR(50) REFERENCES %[1]s.devices(id),
		subnet VARCHAR(50) DEFAULT '10.9.0.0/24',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.vpn_mesh_nodes (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		mesh_id UUID REFERENCES %[1]s.vpn_meshes(id) ON DELETE CASCADE,
		device_id VARCHAR(50) REFERENCES %[1]s.devices(id),
		role VARCHAR(50) DEFAULT 'spoke',
		private_key VARCHAR(255) NOT NULL,
		public_key VARCHAR(255) NOT NULL,
		listen_port INT DEFAULT 51821,
		internal_ip VARCHAR(50) NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (mesh_id, device_id),
		UNIQUE (mesh_id, internal_ip)
	);

	CREATE TABLE IF NOT EXISTS %[1]s.profiles (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		description TEXT,
		config_json JSONB DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.backups (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		device_id VARCHAR(50) REFERENCES %[1]s.devices(id),
		checksum VARCHAR(64) NOT NULL,
		content BYTEA,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.firmwares (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		filename VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		version VARCHAR(50),
		model_compatibility VARCHAR(50),
		data BYTEA,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.agent_versions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		version_hash VARCHAR(64) UNIQUE NOT NULL,
		script_content TEXT NOT NULL,
		is_active BOOLEAN DEFAULT false,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.system_logs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		device_id VARCHAR(50) REFERENCES %[1]s.devices(id) ON DELETE CASCADE,
		log_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
		severity VARCHAR(20) NOT NULL,
		message TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE EXTENSION IF NOT EXISTS pg_trgm;
	CREATE INDEX IF NOT EXISTS trgm_idx_%[1]s_system_logs_message ON %[1]s.system_logs USING gin (message gin_trgm_ops);

	CREATE TABLE IF NOT EXISTS %[1]s.client_hostnames (
		mac VARCHAR(50) PRIMARY KEY,
		site_id UUID REFERENCES %[1]s.sites(id),
		hostname VARCHAR(255) NOT NULL,
		latitude NUMERIC(10, 6),
		longitude NUMERIC(10, 6),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.ai_insights (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		correlation_id VARCHAR(100),
		diagnosis TEXT,
		severity VARCHAR(20),
		involved_devices JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.shaping_rules (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		device_id VARCHAR(50) REFERENCES %[1]s.devices(id) ON DELETE CASCADE,
		mac VARCHAR(50) NOT NULL,
		rate_mbytes INT NOT NULL,
		expires_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(device_id, mac)
	);

	CREATE TABLE IF NOT EXISTS %[1]s.threat_intel_meta (
		id SERIAL PRIMARY KEY,
		fetched_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		ip_count INTEGER NOT NULL DEFAULT 0,
		sources_count INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS %[1]s.site_configs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES %[1]s.sites(id) ON DELETE CASCADE UNIQUE,
		enable_global_ssid BOOLEAN DEFAULT true,
		sqm_cake_enabled BOOLEAN DEFAULT false,
		sqm_download INTEGER DEFAULT 0,
		sqm_upload INTEGER DEFAULT 0,
		dpi_enabled BOOLEAN DEFAULT false,
		secure_tunnel_enabled BOOLEAN DEFAULT true,
		tailscale_enabled BOOLEAN DEFAULT false,
		tailscale_auth_key VARCHAR(255) DEFAULT '',
		global_ssid VARCHAR(255) DEFAULT '',
		global_wpa_key VARCHAR(255) DEFAULT '',
		global_encryption VARCHAR(50) DEFAULT 'psk2',
		lan_ipaddr VARCHAR(50) DEFAULT '192.168.1.1',
		sqm_cake_enabled BOOLEAN DEFAULT false,
		dpi_enabled BOOLEAN DEFAULT false,
		secure_tunnel_enabled BOOLEAN DEFAULT true,
		tailscale_enabled BOOLEAN DEFAULT false,
		tailscale_auth_key VARCHAR(255) DEFAULT '',
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
		dhcp_reservations JSONB DEFAULT '[]',
		port_forwarding_rules JSONB DEFAULT '[]',
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS %[1]s.guest_vouchers (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		site_id UUID REFERENCES %[1]s.sites(id),
		code VARCHAR(10) UNIQUE NOT NULL,
		duration_minutes INT NOT NULL,
		quota_mb INT,
		is_used BOOLEAN DEFAULT false,
		used_by_mac VARCHAR(50),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP WITH TIME ZONE,
		used_at TIMESTAMP WITH TIME ZONE
	);

	CREATE TABLE IF NOT EXISTS %[1]s.portal_settings (
		site_id UUID PRIMARY KEY REFERENCES %[1]s.sites(id),
		enabled BOOLEAN DEFAULT false,
		welcome_text TEXT,
		terms_text TEXT,
		bg_color VARCHAR(20) DEFAULT '#0a0a0a',
		logo_url TEXT,
		redirect_url TEXT,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`, s)

	_, err := DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tenant tables in %s: %w", schema, err)
	}

	// Idempotent tenant-schema migrations
	migrations := []string{
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS device_token VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS last_config_pulled_at TIMESTAMP WITH TIME ZONE", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS last_ip VARCHAR(50)", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS agent_version VARCHAR(64)", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS wg_pubkey VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS wg_privkey VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS wg_ip VARCHAR(50)", s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS profile_id UUID REFERENCES %s.profiles(id)", s, s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS wg_endpoint VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS wg_pubkey VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS wg_privkey VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS api_key TEXT UNIQUE", s),
		fmt.Sprintf("ALTER TABLE %s.agent_versions ADD COLUMN IF NOT EXISTS site_id UUID REFERENCES %s.sites(id)", s, s),
		fmt.Sprintf("ALTER TABLE %s.ai_insights ADD COLUMN IF NOT EXISTS llm_model VARCHAR(255)", s),
		fmt.Sprintf("ALTER TABLE %s.ai_insights ADD COLUMN IF NOT EXISTS tokens_used INT DEFAULT 0", s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS auto_adopt BOOLEAN DEFAULT false", s),
		fmt.Sprintf("ALTER TABLE %s.sites ADD COLUMN IF NOT EXISTS threat_shield_enabled BOOLEAN DEFAULT false", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS threat_shield_drops BIGINT DEFAULT 0", s),
		fmt.Sprintf("ALTER TABLE %s.devices ADD COLUMN IF NOT EXISTS device_role VARCHAR(50) DEFAULT 'AP'", s),
		fmt.Sprintf("ALTER TABLE %s.wlans ADD COLUMN IF NOT EXISTS roaming_enabled BOOLEAN DEFAULT false", s),
		fmt.Sprintf("ALTER TABLE %s.wlans ADD COLUMN IF NOT EXISTS ieee80211k BOOLEAN DEFAULT false", s),
		fmt.Sprintf("ALTER TABLE %s.wlans ADD COLUMN IF NOT EXISTS ieee80211v BOOLEAN DEFAULT false", s),
		fmt.Sprintf("ALTER TABLE %s.site_configs ADD COLUMN IF NOT EXISTS dhcp_reservations JSONB DEFAULT '[]'", s),
		fmt.Sprintf("ALTER TABLE %s.site_configs ADD COLUMN IF NOT EXISTS port_forwarding_rules JSONB DEFAULT '[]'", s),
		fmt.Sprintf("ALTER TABLE %s.site_configs ADD COLUMN IF NOT EXISTS threat_shield_enabled BOOLEAN DEFAULT false", s),
		fmt.Sprintf("ALTER TABLE %s.site_configs ADD COLUMN IF NOT EXISTS guest_portal_enabled BOOLEAN DEFAULT false, sqm_enabled BOOLEAN DEFAULT false, sqm_download INTEGER DEFAULT 0, sqm_upload INTEGER DEFAULT 0", s),
		fmt.Sprintf("ALTER TABLE %s.site_configs ADD COLUMN IF NOT EXISTS wan_interfaces JSONB DEFAULT '[]'", s),
	}
	for _, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			log.Printf("tenant migration warning (%s): %v", m, err)
		}
	}

	// Seed API keys for sites without one
	seedTenantSiteAPIKeys(s)

	return nil
}

// ─── DATA MIGRATION ──────────────────────────────────────────────────────────

func runMigrationsForAllTenants() error {
	rows, err := DB.Query("SELECT schema_alias FROM tenants WHERE is_active = true")
	if err != nil {
		return fmt.Errorf("failed to query active tenants for migration: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			log.Printf("Warning: failed to scan tenant schema alias: %v", err)
			continue
		}
		schemaAlias := "tenant_" + alias
		log.Printf("[LANDLORD] Running migrations for tenant: %s", schemaAlias)
		if err := RunTenantMigrations(schemaAlias); err != nil {
			log.Printf("Error running migrations for tenant %s: %v", schemaAlias, err)
		}
	}
	return nil
}

func seedSuperAdminUser() error {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Fresh install — create SUPERADMIN
		adminPass := os.Getenv("SUPERADMIN_DEFAULT_PASSWORD")
		if adminPass == "" {
			b := make([]byte, 12)
			rand.Read(b)
			adminPass = hex.EncodeToString(b)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash bootstrap password: %w", err)
		}
		_, err = DB.Exec(
			"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'SUPERADMIN')",
			"admin", string(hash),
		)
		if err != nil {
			return fmt.Errorf("failed to seed superadmin user: %w", err)
		}
		log.Println("=========================================================")
		log.Println("Bootstrap SUPERADMIN user created!")
		log.Println("Username: admin")
		log.Printf("Password: %s\n", adminPass)
		log.Println("Please change this password immediately after login.")
		log.Println("=========================================================")
	} else {
		// Upgrade existing admin to SUPERADMIN if no SUPERADMIN exists
		var saCount int
		DB.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'SUPERADMIN'").Scan(&saCount)
		if saCount == 0 {
			_, err := DB.Exec("UPDATE users SET role = 'SUPERADMIN' WHERE username = 'admin' AND role = 'ADMIN'")
			if err != nil {
				log.Printf("Warning: failed to upgrade admin to SUPERADMIN: %v", err)
			} else {
				log.Println("[LANDLORD] Upgraded 'admin' user to SUPERADMIN role")
			}
		}
	}
	return nil
}

func seedTenantSiteAPIKeys(schema string) {
	rows, err := DB.Query(fmt.Sprintf("SELECT id, name FROM %s.sites WHERE api_key IS NULL OR api_key = ''", schema))
	if err != nil {
		return
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
		_, err := DB.Exec(fmt.Sprintf("UPDATE %s.sites SET api_key = $1 WHERE id = $2", schema), u.key, u.id)
		if err != nil {
			continue
		}
		fmt.Printf("SITIO [%s]: [%s] | API_KEY: [%s...]\n", schema, u.name, maskAPIKey(u.key))
	}
}

// ─── TENANT CONTEXT HELPERS ──────────────────────────────────────────────────

// SetTenantSearchPath sets the search_path for the current connection to include
// the tenant schema first, then public. This allows unqualified queries to resolve
// to the tenant schema while still accessing public (landlord) tables.
func SetTenantSearchPath(tx *sql.Tx, schemaAlias string) error {
	fullSchema := "tenant_" + schemaAlias
	if !isValidSchemaName(fullSchema) {
		return fmt.Errorf("invalid schema alias: %s", schemaAlias)
	}
	_, err := tx.Exec(fmt.Sprintf("SET search_path TO %s, public", fullSchema))
	return err
}

// ─── VALIDATION ──────────────────────────────────────────────────────────────

// validSchemaRegex matches a *fully qualified* PostgreSQL identifier
// (i.e., it may already include the "tenant_" prefix). Identifiers are capped
// at 63 bytes per the SQL standard, so the prefix + alias must fit.
var validSchemaRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

// maskAPIKey returns the first 8 characters of an API key followed by an
// ellipsis. The full key is only ever printed once on bootstrap; operators
// who need to recover it can `SELECT api_key FROM sites WHERE id=...`.
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "********"
	}
	return key[:8] + "..." + "(" + itoa(len(key)) + " chars)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	if neg {
		return "-" + digits
	}
	return digits
}

func isValidSchemaName(name string) bool {
	return validSchemaRegex.MatchString(name)
}

// SafeTenantSchema validates a tenant alias and returns the fully qualified
// schema name ("tenant_<alias>"). The alias portion is capped so the resulting
// identifier fits within PostgreSQL's 63-byte identifier limit.
//
// This is the single entry point for composing "tenant_<x>" identifiers. All
// SQL building code that needs to interpolate a schema name MUST go through
// this helper (or SafeSchemaIdent) to prevent SQL injection via crafted
// tenant aliases.
func SafeTenantSchema(alias string) (string, error) {
	// 8 ("tenant_") + N alias <= 63 ⇒ N <= 55.
	const maxAliasLen = 55
	if alias == "" {
		return "", fmt.Errorf("empty tenant alias")
	}
	if len(alias) > maxAliasLen {
		return "", fmt.Errorf("tenant alias too long: %d > %d", len(alias), maxAliasLen)
	}
	if !validSchemaRegex.MatchString(alias) {
		return "", fmt.Errorf("invalid tenant alias %q", alias)
	}
	return "tenant_" + alias, nil
}

// SafeSchemaIdent validates a fully qualified schema name (e.g. "tenant_x"
// or "public"). Returns the input unchanged on success, error on failure.
func SafeSchemaIdent(schema string) (string, error) {
	if !isValidSchemaName(schema) {
		return "", fmt.Errorf("invalid schema identifier %q", schema)
	}
	return schema, nil
}

// ─── LEGACY API COMPAT ──────────────────────────────────────────────────────
// These functions are used by existing handlers. They now operate via the
// search_path which defaults to the tenant schema set by middleware.

func UpsertDeviceState(schema string, deviceID string, stateJSON []byte, model string, lastIP string, agentVersion string) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.devices (id, state_json, model, last_ip, agent_version, last_seen_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET 
			state_json = EXCLUDED.state_json,
			model = EXCLUDED.model,
			last_ip = EXCLUDED.last_ip,
			agent_version = EXCLUDED.agent_version,
			last_seen_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`, schema)
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

func InsertDeviceLogs(schema string, deviceID string, logs []LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(fmt.Sprintf(`
		WITH input AS (
			SELECT CAST($1 AS VARCHAR) as device_id,
			       CAST($2 AS TIMESTAMP WITH TIME ZONE) as log_timestamp,
			       CAST($3 AS VARCHAR) as severity,
			       CAST($4 AS TEXT) as message
		)
		INSERT INTO %s.system_logs (device_id, log_timestamp, severity, message)
		SELECT device_id, log_timestamp, severity, message FROM input
		WHERE NOT EXISTS (
			SELECT 1 FROM %s.system_logs 
			WHERE device_id = input.device_id 
			  AND log_timestamp = input.log_timestamp 
			  AND message = input.message
		)
	`, schema, schema))
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

// ─── UTILITY ─────────────────────────────────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetTenantSchemaForSiteKey looks up which tenant schema contains a site with the given API key.
// This is used by the telemetry endpoint which authenticates via X-Site-Key.
func GetTenantSchemaForSiteKey(siteKey string) (string, error) {
	if strings.TrimSpace(siteKey) == "" {
		return "", fmt.Errorf("empty site key")
	}

	// Query all active tenant schemas
	rows, err := DB.Query("SELECT schema_alias FROM tenants WHERE is_active = true")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			continue
		}
		schema := "tenant_" + alias
		var count int
		err := DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.sites WHERE api_key = $1", schema), siteKey).Scan(&count)
		if err == nil && count > 0 {
			return schema, nil
		}
	}

	return "", fmt.Errorf("site key not found in any tenant schema")
}

type Queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

type txKeyType string

const TxKey = txKeyType("tx")

func Tx(ctx context.Context) Queryer {
	if tx, ok := ctx.Value(TxKey).(*sql.Tx); ok {
		return tx
	}
	return DB // Fallback to global connection pool if no transaction
}
