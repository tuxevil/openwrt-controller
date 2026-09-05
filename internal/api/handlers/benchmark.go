package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/services"
)

func RunSiteBenchmarkHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}

	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"site_id is required"}`, http.StatusBadRequest)
		return
	}

	report, err := services.RunSiteMeshBenchmark(r.Context(), schema, siteID)
	if err != nil {
		http.Error(w, `{"error":"benchmark execution failed: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func RunDeviceBenchmarkHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		http.Error(w, `{"error":"invalid tenant context"}`, http.StatusInternalServerError)
		return
	}

	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		http.Error(w, `{"error":"device_id is required"}`, http.StatusBadRequest)
		return
	}

	var targetIP string
	if err := database.Tx(r.Context()).QueryRow(
		"SELECT COALESCE(last_ip, '') FROM "+schema+".devices WHERE id = $1",
		deviceID,
	).Scan(&targetIP); err != nil || targetIP == "" {
		http.Error(w, `{"error":"device not found or IP unavailable"}`, http.StatusNotFound)
		return
	}

	var devName string
	_ = database.Tx(r.Context()).QueryRow(
		"SELECT COALESCE(name, model, id) FROM "+schema+".devices WHERE id = $1",
		deviceID,
	).Scan(&devName)
	if devName == "" {
		devName = deviceID
	}

	baseline := services.GetBaselineForDevice(deviceID, targetIP)
	res := services.RunNodeBenchmark(r.Context(), deviceID, devName, targetIP, baseline)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
