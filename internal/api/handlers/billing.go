package handlers

import (
	"encoding/json"
	"net/http"
	"openwrt-controller/internal/database"
)

type TenantBilling struct {
	TenantName  string `json:"tenant_name"`
	SchemaAlias string `json:"schema_alias"`
	TotalSites  int    `json:"total_sites"`
	TotalNodes  int    `json:"total_nodes"`
}

func GetBillingUsageHandler(w http.ResponseWriter, r *http.Request) {
	// Only Landlord Admin should call this, verified by middleware
	rows, err := database.DB.Query("SELECT name, schema_alias FROM tenants WHERE is_active = true")
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var billings []TenantBilling
	for rows.Next() {
		var name, alias string
		if err := rows.Scan(&name, &alias); err != nil {
			continue
		}
		
		schema := "tenant_" + alias
		var sites, nodes int
		
		database.DB.QueryRow("SELECT COUNT(id) FROM " + schema + ".sites").Scan(&sites)
		database.DB.QueryRow("SELECT COUNT(id) FROM " + schema + ".devices").Scan(&nodes)

		billings = append(billings, TenantBilling{
			TenantName:  name,
			SchemaAlias: alias,
			TotalSites:  sites,
			TotalNodes:  nodes,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(billings)
}
