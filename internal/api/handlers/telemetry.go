package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func TelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "Bad request: invalid json", http.StatusBadRequest)
		return
	}

	deviceID, ok := raw["device_id"].(string)
	if !ok || deviceID == "" {
		http.Error(w, "Bad request: missing device_id", http.StatusBadRequest)
		return
	}

	providedKey := r.Header.Get("X-Site-Key")
	var siteKey *string
	err = database.DB.QueryRow(`
		SELECT s.api_key FROM sites s 
		JOIN devices d ON d.site_id = s.id 
		WHERE d.id = $1`, deviceID).Scan(&siteKey)
	if err == nil && siteKey != nil && *siteKey != "" {
		if providedKey != *siteKey {
			http.Error(w, "Forbidden: invalid site key", http.StatusForbidden)
			return
		}
	}

	modelStr := "UNKNOWN"
	if boardInfo, ok := raw["board"].(map[string]interface{}); ok {
		if model, ok := boardInfo["model"].(string); ok {
			modelStr = model
		}
	}
	
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		remoteIP = host
	}

	// 1. Goroutine for PostgreSQL (upsert state and explicit model)
	go func(devID string, state []byte, mod string, ip string) {
		if err := database.UpsertDeviceState(devID, state, mod, ip); err != nil {
			log.Printf("Error upserting device state to postgres: %v\n", err)
		}
	}(deviceID, body, modelStr, remoteIP)

	// Extract Metrics cleanly bypassing struct matching
	var metrics models.DeviceMetrics
	if sys, ok := raw["system"].(map[string]interface{}); ok {
		if loadArr, ok := sys["load"].([]interface{}); ok && len(loadArr) > 0 {
			if v, ok := loadArr[0].(float64); ok {
				metrics.CPULoad = v / 65536.0
			}
		}
		if mem, ok := sys["memory"].(map[string]interface{}); ok {
			if free, ok := mem["free"].(float64); ok {
				metrics.RAMFree = int64(free)
			}
		}
		if uptime, ok := sys["uptime"].(float64); ok {
			metrics.Uptime = int64(uptime)
		}
	}

	// 2. Goroutine for InfluxDB (metrics)
	go func(devID string, mets models.DeviceMetrics) {
		if err := database.WriteMetrics(devID, &mets); err != nil {
			log.Printf("Error writing metrics to influx: %v\n", err)
		}
	}(deviceID, metrics)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}
