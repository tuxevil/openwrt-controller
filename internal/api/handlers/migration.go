package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

// AnalyzeOmadaBackup parses an uploaded omada_export.json and returns
// the identified DHCP and Port Forward rules for UI preview.
func AnalyzeOmadaBackup(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file part", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Unable to read file", http.StatusInternalServerError)
		return
	}

	dhcp, fw, err := services.ParseOmadaExport(fileBytes)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "success",
		"dhcp":            dhcp,
		"port_forwarding": fw,
	})
}

// CommitOmadaMigration receives verified data and pushes it to the device via Edge Nexus logic.
func CommitOmadaMigration(w http.ResponseWriter, r *http.Request) {
	username := GetUsernameFromReq(r)

	var payload struct {
		SiteID         string                     `json:"site_id"`
		DHCP           []services.StaticLease     `json:"dhcp"`
		PortForwarding []services.PortForwardRule `json:"port_forwarding"`
	}

	if !readBody(w, r, &payload) {
		return
	}

	if payload.SiteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	if len(payload.DHCP) == 0 && len(payload.PortForwarding) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Nothing to push"})
		return
	}

	dhcpBytes, err := json.Marshal(payload.DHCP)
	if err != nil {
		dhcpBytes = []byte("[]")
	}

	fwBytes, err := json.Marshal(payload.PortForwarding)
	if err != nil {
		fwBytes = []byte("[]")
	}

	sc, err := services.GetSiteConfig(payload.SiteID)
	if err != nil {
		// Initialize empty config if not found
		sc = &services.SiteConfig{
			SiteID: payload.SiteID,
		}
	}

	sc.DHCPReservations = dhcpBytes
	sc.PortForwardingRules = fwBytes

	if err := services.UpsertSiteConfig(*sc); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	database.InsertAuditLog(username, "OMADA_MIGRATOR_COMMIT", "SITE_TEMPLATE", payload.SiteID,
		fmt.Sprintf("Assimilated %d DHCP / %d FW rules into Orchestrator", len(payload.DHCP), len(payload.PortForwarding)), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"output": "Configuration injected securely into Site Orchestrator Template. Apply via Orchestrator Sync.",
	})
}
