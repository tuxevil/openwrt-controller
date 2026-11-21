package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// GetMapsConfigHandler returns map basemap configuration (e.g. CARTO raster API key)
// to authenticated dashboard sessions so the key is managed via runtime environment
// variables rather than baked into public source code.
func GetMapsConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"carto_api_key": strings.TrimSpace(os.Getenv("CARTO_API_KEY")),
	})
}
