package models

import (
	"encoding/json"
	"time"
)

type Controller struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MAC       string    `json:"mac"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Site struct {
	ID           string    `json:"id"`
	ControllerID string    `json:"controller_id"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Device struct {
	ID         string          `json:"id"` // MAC usually
	SiteID     string          `json:"site_id"`
	Name       string          `json:"name"`
	Model      string          `json:"model"`
	Status     string          `json:"status"`
	StateJSON  json.RawMessage `json:"state_json"` // mapped to JSONB
	LastSeenAt time.Time       `json:"last_seen_at"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// Payload for POST /api/telemetry
type TelemetryPayload struct {
	DeviceID string          `json:"device_id"` // MAC Address
	Hardware json.RawMessage `json:"hardware"`  // block for hardware from openwrt
	Network  json.RawMessage `json:"network"`   // block for network from openwrt
	Metrics  DeviceMetrics   `json:"metrics"`
}

type DeviceMetrics struct {
	CPULoad     float64 `json:"cpu_load"`
	RAMFree     int64   `json:"ram_free"`
	Uptime      int64   `json:"uptime"`
	DHCPClients int     `json:"dhcp_clients"`
}

type WLAN struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"site_id"`
	SSID      string    `json:"ssid"`
	Security  string    `json:"security"`
	Password  string    `json:"password"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
