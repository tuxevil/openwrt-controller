package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/services"
)

func GetSiteEchoLocationHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error": "site_id required"}`, http.StatusBadRequest)
		return
	}

	graph, err := services.GenerateEchoLocation(siteID)
	if err != nil {
		http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": graph})
}
