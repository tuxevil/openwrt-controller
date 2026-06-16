package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func DeleteWLANHandler(w http.ResponseWriter, r *http.Request) {
	wlanID := r.PathValue("wlan_id")
	if wlanID == "" {
		http.Error(w, `{"error": "wlan_id is required"}`, http.StatusBadRequest)
		return
	}

	res, err := database.Tx(r.Context()).Exec("DELETE FROM wlans WHERE id = $1", wlanID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "wlan not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "deleted"})
}

type createWLANRequest struct {
	SSID           string   `json:"ssid"`
	Security       string   `json:"security"`
	Password       string   `json:"password"`
	Enabled        *bool    `json:"enabled"`
	RoamingEnabled *bool    `json:"roaming_enabled"`
	Band           string   `json:"band"`
	TargetMode     string   `json:"target_mode"`
	Ieee80211w     string   `json:"ieee80211w"`
	AuthServer     string   `json:"auth_server"`
	AuthSecret     string   `json:"auth_secret"`
	DynamicVlan    string   `json:"dynamic_vlan"`
	CustomDevices  []string `json:"custom_devices"`
	Ieee80211k     *bool    `json:"ieee80211k"`
	Ieee80211v     *bool    `json:"ieee80211v"`
}

func CreateWLANHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	var req createWLANRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}

	if req.SSID == "" || req.Security == "" {
		http.Error(w, `{"error": "ssid and security are required"}`, http.StatusBadRequest)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	roamingEnabled := false
	if req.RoamingEnabled != nil {
		roamingEnabled = *req.RoamingEnabled
	}

	ieee80211k := false
	if req.Ieee80211k != nil {
		ieee80211k = *req.Ieee80211k
	}

	ieee80211v := false
	if req.Ieee80211v != nil {
		ieee80211v = *req.Ieee80211v
	}

	band := "both"
	if req.Band != "" {
		band = req.Band
	}
	targetMode := "all"
	if req.TargetMode != "" {
		targetMode = req.TargetMode
	}

	var newID string
	err := database.Tx(r.Context()).QueryRow(
		"INSERT INTO wlans (site_id, ssid, security, password, enabled, roaming_enabled, band, target_mode, ieee80211k, ieee80211v, ieee80211w, auth_server, auth_secret, dynamic_vlan) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING id",
		siteID, req.SSID, req.Security, req.Password, enabled, roamingEnabled, band, targetMode, ieee80211k, ieee80211v, req.Ieee80211w, req.AuthServer, req.AuthSecret, req.DynamicVlan,
		siteID, req.SSID, req.Security, req.Password, enabled, roamingEnabled, band, targetMode, ieee80211k, ieee80211v,
	).Scan(&newID)

	if err != nil {
		http.Error(w, `{"error": "failed to create wlan"}`, http.StatusInternalServerError)
		return
	}

	if targetMode == "custom" && len(req.CustomDevices) > 0 {
		for _, devID := range req.CustomDevices {
			database.Tx(r.Context()).Exec("INSERT INTO device_wlans (wlan_id, device_id) VALUES ($1, $2)", newID, devID)
		}
	}

	go services.AddWLANConfig(context.Background(), siteID, req.SSID, req.Security, req.Password, roamingEnabled, ieee80211k, ieee80211v, req.Ieee80211w, req.AuthServer, req.AuthSecret, req.DynamicVlan)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  map[string]interface{}{"id": newID},
		"error": nil,
	})
}

func GetWLANsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id is required"}`, http.StatusBadRequest)
		return
	}

	query := `SELECT id, site_id, ssid, security, password, enabled, COALESCE(roaming_enabled, false), band, target_mode, COALESCE(ieee80211k, false), COALESCE(ieee80211v, false) FROM wlans WHERE site_id = $1`
	rows, err := database.Tx(r.Context()).Query(query, siteID)
	if err != nil {
		http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
		return
	}

	type tempWlan struct {
		id, sID, ssid, security, password, band, targetMode string
		enabled, roaming, ieee80211k, ieee80211v            bool
	}
	var temps []tempWlan
	for rows.Next() {
		var t tempWlan
		if err := rows.Scan(&t.id, &t.sID, &t.ssid, &t.security, &t.password, &t.enabled, &t.roaming, &t.band, &t.targetMode, &t.ieee80211k, &t.ieee80211v); err == nil {
			temps = append(temps, t)
		}
	}
	rows.Close()

	var wlans []map[string]interface{}
	for _, t := range temps {
		customDevices := []string{}
		if t.targetMode == "custom" {
			cRows, err := database.Tx(r.Context()).Query("SELECT device_id FROM device_wlans WHERE wlan_id = $1", t.id)
			if err == nil {
				for cRows.Next() {
					var d string
					if cRows.Scan(&d) == nil {
						customDevices = append(customDevices, d)
					}
				}
				cRows.Close()
			}
		}
		wlans = append(wlans, map[string]interface{}{
			"id":              t.id,
			"site_id":         t.sID,
			"ssid":            t.ssid,
			"security":        t.security,
			"password":        t.password,
			"enabled":         t.enabled,
			"roaming_enabled": t.roaming,
			"band":            t.band,
			"target_mode":     t.targetMode,
			"ieee80211k":      t.ieee80211k,
			"ieee80211v":      t.ieee80211v,
			"custom_devices":  customDevices,
		})
	}

	if wlans == nil {
		wlans = make([]map[string]interface{}, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  wlans,
		"error": nil,
	})
}

func UpdateWLANHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	wlanID := r.PathValue("wlan_id")
	if siteID == "" || wlanID == "" {
		http.Error(w, `{"error": "site_id and wlan_id are required"}`, http.StatusBadRequest)
		return
	}

	var req createWLANRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid json"}`, http.StatusBadRequest)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	roamingEnabled := false
	if req.RoamingEnabled != nil {
		roamingEnabled = *req.RoamingEnabled
	}

	ieee80211k := false
	if req.Ieee80211k != nil {
		ieee80211k = *req.Ieee80211k
	}

	ieee80211v := false
	if req.Ieee80211v != nil {
		ieee80211v = *req.Ieee80211v
	}

	band := "both"
	if req.Band != "" {
		band = req.Band
	}
	targetMode := "all"
	if req.TargetMode != "" {
		targetMode = req.TargetMode
	}

	var err error
	if req.Password != "" {
		_, err = database.Tx(r.Context()).Exec(
			"UPDATE wlans SET ssid=$1, security=$2, password=$3, enabled=$4, roaming_enabled=$5, band=$6, target_mode=$7, ieee80211k=$8, ieee80211v=$9 WHERE id=$10 AND site_id=$11",
			req.SSID, req.Security, req.Password, enabled, roamingEnabled, band, targetMode, ieee80211k, ieee80211v, wlanID, siteID,
		)
	} else {
		_, err = database.Tx(r.Context()).Exec(
			"UPDATE wlans SET ssid=$1, security=$2, enabled=$3, roaming_enabled=$4, band=$5, target_mode=$6, ieee80211k=$7, ieee80211v=$8 WHERE id=$9 AND site_id=$10",
			req.SSID, req.Security, enabled, roamingEnabled, band, targetMode, ieee80211k, ieee80211v, wlanID, siteID,
		)
	}

	if err != nil {
		http.Error(w, `{"error": "failed to update wlan"}`, http.StatusInternalServerError)
		return
	}

	database.Tx(r.Context()).Exec("DELETE FROM device_wlans WHERE wlan_id = $1", wlanID)
	if targetMode == "custom" && len(req.CustomDevices) > 0 {
		for _, devID := range req.CustomDevices {
			database.Tx(r.Context()).Exec("INSERT INTO device_wlans (wlan_id, device_id) VALUES ($1, $2)", wlanID, devID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "updated",
		"error":  nil,
	})
}
