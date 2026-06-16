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
	Latitude     *float64  `json:"latitude"`
	Longitude    *float64  `json:"longitude"`
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
	SignalDBM   float64 `json:"signal_dbm"`
	RxMbps      float64 `json:"rx_mbps"`
	TxMbps      float64 `json:"tx_mbps"`
}

type WLAN struct {
	ID          string    `json:"id"`
	SiteID      string    `json:"site_id"`
	SSID        string    `json:"ssid"`
	Security    string    `json:"security"`
	Password    string    `json:"password"`
	Enabled     bool      `json:"enabled"`
	Ieee80211w  string    `json:"ieee80211w"`
	AuthServer  string    `json:"auth_server"`
	AuthSecret  string    `json:"auth_secret"`
	DynamicVlan string    `json:"dynamic_vlan"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TopTalker struct {
	MAC       string `json:"mac"`
	RateRx    int    `json:"rate_rx"`
	RateTx    int    `json:"rate_tx"`
	TotalRate int    `json:"total_rate"`
}

type IfaceStats struct {
	RxBytes int64 `json:"rx_bytes"`
	TxBytes int64 `json:"tx_bytes"`
}

type SiteSettings struct {
	SiteID     string    `json:"site_id"`
	DNSServers string    `json:"dns_servers"`
	DHCPServer bool      `json:"dhcp_server"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Client struct {
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	DeviceID  string `json:"device_id"`
	IPAddress string `json:"ip_address"`
	Signal    int    `json:"signal"`
}

type Incident struct {
	ID           string     `json:"id"`
	SiteID       string     `json:"site_id"`
	DeviceID     string     `json:"device_id"`
	DeviceName   string     `json:"device_name"`
	IncidentType string     `json:"type"`
	Severity     string     `json:"severity"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

type Profile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ConfigJSON  json.RawMessage `json:"config_json"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type VPNMesh struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Topology    string    `json:"topology"`
	HubDeviceID string    `json:"hub_device_id"`
	Subnet      string    `json:"subnet"`
	CreatedAt   time.Time `json:"created_at"`
}

type VPNMeshNode struct {
	ID             string    `json:"id"`
	MeshID         string    `json:"mesh_id"`
	DeviceID       string    `json:"device_id"`
	DeviceName     string    `json:"device_name,omitempty"` // For UI
	Role           string    `json:"role"`
	PrivateKey     string    `json:"private_key"`
	PublicKey      string    `json:"public_key"`
	ListenPort     int       `json:"listen_port"`
	InternalIP     string    `json:"internal_ip"`
	PublicEndpoint string    `json:"public_endpoint,omitempty"` // Hub's public IP[:port] (auto-detected if empty)
	CreatedAt      time.Time `json:"created_at"`
}

type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret"`
	Events    []string  `json:"events"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}
