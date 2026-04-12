package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func TelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload models.TelemetryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request: invalid json", http.StatusBadRequest)
		return
	}

	if payload.DeviceID == "" {
		http.Error(w, "Bad request: missing device_id", http.StatusBadRequest)
		return
	}

	// Combine Hardware and Network into a single state map to store as JSONB
	stateMap := map[string]interface{}{}
	if len(payload.Hardware) > 0 {
		var hw map[string]interface{}
		_ = json.Unmarshal(payload.Hardware, &hw)
		stateMap["hardware"] = hw
	}
	if len(payload.Network) > 0 {
		var nw map[string]interface{}
		_ = json.Unmarshal(payload.Network, &nw)
		stateMap["network"] = nw
	}

	stateJSON, err := json.Marshal(stateMap)
	if err != nil {
		http.Error(w, "Internal server error: processing state", http.StatusInternalServerError)
		return
	}

	// 1. Goroutine for PostgreSQL (upsert state)
	go func(devID string, state []byte) {
		if err := database.UpsertDeviceState(devID, state); err != nil {
			log.Printf("Error upserting device state to postgres: %v\n", err)
		}
	}(payload.DeviceID, stateJSON)

	// 2. Goroutine for InfluxDB (metrics)
	go func(devID string, metrics models.DeviceMetrics) {
		if err := database.WriteMetrics(devID, &metrics); err != nil {
			log.Printf("Error writing metrics to influx: %v\n", err)
		}
	}(payload.DeviceID, payload.Metrics)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}
