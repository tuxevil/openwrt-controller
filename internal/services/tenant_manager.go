package services

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

// ─── TENANT MANAGER ─────────────────────────────────────────────────────────
// Provisioning engine for MSP multi-tenant architecture.
// Handles tenant lifecycle: registration, schema creation, stats aggregation.

var validAliasRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,54}$`)

// RegisterTenant creates a new tenant: inserts into public.tenants, creates the
// PostgreSQL schema, and runs all operational table migrations inside it.
func RegisterTenant(name, schemaAlias string) (*models.Tenant, error) {
	// Normalize alias
	alias := strings.ToLower(strings.TrimSpace(schemaAlias))
	alias = strings.ReplaceAll(alias, " ", "_")
	alias = strings.ReplaceAll(alias, "-", "_")

	if !validAliasRegex.MatchString(alias) {
		return nil, fmt.Errorf("invalid schema alias '%s': must be lowercase alphanumeric with underscores, 2-55 chars, starting with a letter", alias)
	}

	// Check uniqueness
	var count int
	err := database.DB.QueryRow("SELECT COUNT(*) FROM tenants WHERE schema_alias = $1", alias).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check tenant uniqueness: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("tenant with alias '%s' already exists", alias)
	}

	// 1. Insert into public.tenants
	var tenantID string
	var createdAt time.Time
	err = database.DB.QueryRow(
		"INSERT INTO tenants (name, schema_alias) VALUES ($1, $2) RETURNING id, created_at",
		strings.TrimSpace(name), alias,
	).Scan(&tenantID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert tenant: %w", err)
	}

	// 2. Create schema and run migrations
	fullSchema := "tenant_" + alias
	log.Printf("[TENANT_MANAGER] Provisioning schema '%s' for tenant '%s'...", fullSchema, name)

	if err := database.RunTenantMigrations(fullSchema); err != nil {
		// Rollback tenant record on schema failure
		database.DB.Exec("DELETE FROM tenants WHERE id = $1", tenantID)
		return nil, fmt.Errorf("failed to provision tenant schema: %w", err)
	}

	log.Printf("[TENANT_MANAGER] Tenant '%s' (schema: %s) provisioned successfully.", name, fullSchema)

	return &models.Tenant{
		ID:          tenantID,
		Name:        name,
		SchemaAlias: alias,
		IsActive:    true,
		SiteCount:   0,
		DeviceCount: 0,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}, nil
}

// ListTenants returns all tenants with aggregated site and device counts.
func ListTenants() ([]models.Tenant, error) {
	rows, err := database.DB.Query(`
		SELECT id, name, schema_alias, is_active, created_at, updated_at
		FROM tenants
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []models.Tenant
	for rows.Next() {
		var t models.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.SchemaAlias, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}

		// Aggregate stats from tenant schema
		schema, schemaErr := database.SafeTenantSchema(t.SchemaAlias)
		if schemaErr != nil {
			log.Printf("[TENANT_MANAGER] skipping invalid schema alias %q: %v", t.SchemaAlias, schemaErr)
			continue
		}
		database.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.sites", schema)).Scan(&t.SiteCount)
		database.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.devices", schema)).Scan(&t.DeviceCount)

		tenants = append(tenants, t)
	}

	return tenants, nil
}

// ToggleTenant enables or disables a tenant.
func ToggleTenant(tenantID string, active bool) error {
	result, err := database.DB.Exec(
		"UPDATE tenants SET is_active = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		active, tenantID,
	)
	if err != nil {
		return fmt.Errorf("failed to toggle tenant: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}
	return nil
}

// GetTenantBySchema looks up a tenant by its schema alias for middleware validation.
func GetTenantBySchema(schemaAlias string) (*models.Tenant, error) {
	var t models.Tenant
	err := database.DB.QueryRow(
		"SELECT id, name, schema_alias, is_active, created_at, updated_at FROM tenants WHERE schema_alias = $1",
		schemaAlias,
	).Scan(&t.ID, &t.Name, &t.SchemaAlias, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %s", schemaAlias)
	}
	return &t, nil
}

// GetTenantStats returns detailed stats for a single tenant.
func GetTenantStats(tenantID string) (map[string]interface{}, error) {
	var alias string
	err := database.DB.QueryRow("SELECT schema_alias FROM tenants WHERE id = $1", tenantID).Scan(&alias)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}

	schema, schemaErr := database.SafeTenantSchema(alias)
	if schemaErr != nil {
		return nil, fmt.Errorf("invalid tenant schema: %w", schemaErr)
	}
	stats := map[string]interface{}{
		"tenant_id":    tenantID,
		"schema_alias": alias,
	}

	var siteCount, deviceCount, incidentCount, voucherCount int
	database.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.sites", schema)).Scan(&siteCount)
	database.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.devices", schema)).Scan(&deviceCount)
	database.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.incidents WHERE status = 'OPEN'", schema)).Scan(&incidentCount)
	database.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s.guest_vouchers WHERE is_used = false", schema)).Scan(&voucherCount)

	stats["sites"] = siteCount
	stats["devices"] = deviceCount
	stats["open_incidents"] = incidentCount
	stats["available_vouchers"] = voucherCount

	return stats, nil
}
