package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/services"
)

func GetRFOptimizationHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"invalid site"}`, http.StatusBadRequest)
		return
	}

	result, err := services.AnalyzeSiteRF(r.Context(), siteID)
	if err != nil {
		http.Error(w, `{"error":"rf analysis failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": result})
}

func RunRFFixHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"invalid site"}`, http.StatusBadRequest)
		return
	}

	// Dynamic orchestrator command to reset radio interfaces and re-scan for channels
	// Actually hardcodes auto channel and restarts wireless
	cmd := "uci set wireless.radio0.channel=auto && uci commit wireless && wifi"

	results := services.RunMassCommand(r.Context(), siteID, cmd)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "AUTO_OPTIMIZATION_TRIGGERED",
		"results": results,
	})
}
