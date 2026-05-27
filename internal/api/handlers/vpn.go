package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

type VPNConfig struct {
	Endpoint string `json:"endpoint"`
	PubKey   string `json:"pubkey"`
}

func GetVPNConfigHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	var wgEndpoint, wgPubKey sql.NullString
	_ = database.Tx(r.Context()).QueryRow("SELECT wg_endpoint, wg_pubkey FROM sites WHERE id = $1", siteID).Scan(&wgEndpoint, &wgPubKey)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(VPNConfig{
		Endpoint: wgEndpoint.String,
		PubKey:   wgPubKey.String,
	})
}

func UpdateVPNEndpointHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}

	_, err := database.Tx(r.Context()).Exec("UPDATE sites SET wg_endpoint = $1 WHERE id = $2", req.Endpoint, siteID)
	if err != nil {
		http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}

// GetVPNPeersHandler returns devices with their assigned wg_ip
func GetVPNPeersHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	rows, err := database.Tx(r.Context()).Query("SELECT id, name, wg_ip, wg_pubkey, status FROM devices WHERE site_id = $1 AND wg_ip IS NOT NULL", siteID)
	if err != nil {
		http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var peers []map[string]interface{}
	for rows.Next() {
		var id, name string
		var wgIP, wgPubKey, status sql.NullString
		if err := rows.Scan(&id, &name, &wgIP, &wgPubKey, &status); err == nil {
			peers = append(peers, map[string]interface{}{
				"id":        id,
				"name":      name,
				"wg_ip":     wgIP.String,
				"wg_pubkey": wgPubKey.String,
				"status":    status.String,
			})
		}
	}
	if peers == nil {
		peers = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}
