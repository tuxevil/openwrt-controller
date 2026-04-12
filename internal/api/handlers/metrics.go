package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
)

func GetDeviceMetricsHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error": "device_id is required"}`, http.StatusBadRequest)
		return
	}

	metrics, err := database.GetDeviceMetrics(deviceID, "-15m")
	if err != nil {
		// As per requirement: return empty array instead of 500
		metrics = []float64{}
	}

	if metrics == nil {
		metrics = []float64{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": metrics,
	})
}
