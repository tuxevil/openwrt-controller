package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || !validVPNEndpoint(req.Endpoint) {
		http.Error(w, `{"error":"endpoint must be an IPv4 address or DNS hostname followed by :port"}`, http.StatusBadRequest)
		return
	}

	// One statement keeps the mutation and audit event atomic, including when
	// invoked without the request-scoped transaction middleware.
	payload, _ := json.Marshal(req)
	remoteIP, _, splitErr := net.SplitHostPort(r.RemoteAddr)
	if splitErr != nil {
		remoteIP = r.RemoteAddr
	}
	var changed int
	err := database.Tx(r.Context()).QueryRowContext(r.Context(), `
		WITH changed AS (
			UPDATE sites SET wg_endpoint = $1 WHERE id = $2 RETURNING id
		), logged AS (
			INSERT INTO audit_logs (username, action, resource_type, resource_id, payload, ip_addr)
			SELECT $3, 'VPN_ENDPOINT_UPDATE', 'SITE', id::text, $4, $5 FROM changed
			RETURNING id
		) SELECT count(*) FROM logged`, req.Endpoint, siteID,
		GetUsernameFromReq(r), string(payload), remoteIP).Scan(&changed)
	if err != nil {
		http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
		return
	}
	if changed == 0 {
		http.Error(w, `{"error":"site not found"}`, http.StatusNotFound)
		return
	}
	if err := database.CommitRequestTx(r.Context()); err != nil {
		http.Error(w, `{"error":"could not commit endpoint update"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success"}`))
}

// The deployed site agent splits host:port at the first colon. IPv6 endpoints
// must remain rejected until that renderer supports bracketed IPv6 literals.
func validVPNEndpoint(endpoint string) bool {
	if endpoint == "" || endpoint != strings.TrimSpace(endpoint) || strings.ContainsAny(endpoint, "[]") {
		return false
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" || len(host) > 253 || port == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Is4() && !addr.IsUnspecified() && !addr.IsMulticast()
	}
	if strings.Trim(host, "0123456789.") == "" {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
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
