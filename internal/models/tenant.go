package models

import "time"

// Tenant represents a client organization in the MSP multi-tenant architecture.
// Each tenant has an isolated PostgreSQL schema for operational data.
type Tenant struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	SchemaAlias string    `json:"schema_alias"`
	IsActive    bool      `json:"is_active"`
	SiteCount   int       `json:"site_count"`
	DeviceCount int       `json:"device_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
