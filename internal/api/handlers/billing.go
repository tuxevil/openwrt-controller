package handlers

import (
	"encoding/json"
	"log"
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
	// Only SuperAdmin should call this; enforced by the route middleware.
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

		// Validate the schema identifier before interpolating it into SQL.
		schema, err := database.SafeTenantSchema(alias)
		if err != nil {
			log.Printf("[billing] skipping tenant %q: %v", alias, err)
			continue
		}
		var sites, nodes int
		if err := database.DB.QueryRow("SELECT COUNT(id) FROM " + schema + ".sites").Scan(&sites); err != nil {
			log.Printf("[billing] sites count failed for %s: %v", schema, err)
		}
		if err := database.DB.QueryRow("SELECT COUNT(id) FROM " + schema + ".devices").Scan(&nodes); err != nil {
			log.Printf("[billing] devices count failed for %s: %v", schema, err)
		}

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
