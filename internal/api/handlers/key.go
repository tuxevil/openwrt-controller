package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
)

func RotateSiteKeyHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"site_id required"}`, http.StatusBadRequest)
		return
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		http.Error(w, `{"error":"key generation failed"}`, http.StatusInternalServerError)
		return
	}
	newKey := hex.EncodeToString(b) // 32 hex chars

	_, err := database.Tx(r.Context()).Exec(
		"UPDATE sites SET api_key = $1, updated_at = $2 WHERE id = $3",
		newKey, time.Now(), siteID,
	)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"site_id": siteID,
		"api_key": newKey,
	})
}
