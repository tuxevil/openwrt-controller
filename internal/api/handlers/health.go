package handlers

import (
	"encoding/json"
	"net/http"

	"openwrt-controller/internal/services"
)

func GetGlobalHealthHandler(w http.ResponseWriter, r *http.Request) {
	score := services.GetGlobalHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"health": score})
}
