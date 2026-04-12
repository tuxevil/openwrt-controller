package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func GetSiteSettingsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	
	var settings models.SiteSettings
	err := database.DB.QueryRow("SELECT site_id, dns_servers, dhcp_server, updated_at FROM site_settings WHERE site_id = $1", siteID).Scan(
		&settings.SiteID, &settings.DNSServers, &settings.DHCPServer, &settings.UpdatedAt,
	)

	// If no rows, return defaults natively without 404 to avoid ugly frontend checks
	if err != nil {
		settings = models.SiteSettings{
			SiteID:     siteID,
			DNSServers: "9.9.9.9,1.1.1.1",
			DHCPServer: true,
		}
	}

	var apiKey sql.NullString
	_ = database.DB.QueryRow("SELECT api_key FROM sites WHERE id = $1", siteID).Scan(&apiKey)

	resp := map[string]interface{}{
		"data":    settings,
		"api_key": apiKey.String,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func UpdateSiteSettingsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	var s models.SiteSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO site_settings (site_id, dns_servers, dhcp_server, updated_at) 
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (site_id) DO UPDATE SET 
			dns_servers = EXCLUDED.dns_servers,
			dhcp_server = EXCLUDED.dhcp_server,
			updated_at = CURRENT_TIMESTAMP;
	`
	_, err := database.DB.Exec(query, siteID, s.DNSServers, s.DHCPServer)
	if err != nil {
		http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}
