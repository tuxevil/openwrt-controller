package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// ─── LANDLORD HANDLERS ──────────────────────────────────────────────────────
// SuperAdmin-only endpoints for MSP multi-tenant management.

type createTenantRequest struct {
	Name        string `json:"name"`
	SchemaAlias string `json:"schema_alias"`
}

// GetTenantsHandler lists all tenants with aggregated stats.
func GetTenantsHandler(w http.ResponseWriter, r *http.Request) {
	tenants, err := services.ListTenants()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": tenants,
	})
}

// CreateTenantHandler provisions a new tenant (schema + tables).
func CreateTenantHandler(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.SchemaAlias == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name and schema_alias are required"})
		return
	}

	tenant, err := services.RegisterTenant(req.Name, req.SchemaAlias)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Audit log
	username := GetUsernameFromReq(r)
	database.InsertAuditLog(username, "CREATE_TENANT", "tenant", tenant.ID, req.Name, r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

type toggleTenantRequest struct {
	IsActive bool `json:"is_active"`
}

// ToggleTenantHandler enables or disables a tenant.
func ToggleTenantHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	if tenantID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "tenant ID required"})
		return
	}

	var req toggleTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if err := services.ToggleTenant(tenantID, req.IsActive); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	username := GetUsernameFromReq(r)
	action := "DISABLE_TENANT"
	if req.IsActive {
		action = "ENABLE_TENANT"
	}
	database.InsertAuditLog(username, action, "tenant", tenantID, "", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GetTenantStatsHandler returns detailed stats for a specific tenant.
func GetTenantStatsHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	if tenantID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "tenant ID required"})
		return
	}

	stats, err := services.GetTenantStats(tenantID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
