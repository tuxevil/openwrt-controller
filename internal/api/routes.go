package api

import (
	"net/http"

	"openwrt-controller/internal/api/handlers"
)

func SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/telemetry", handlers.TelemetryHandler)

	mux.HandleFunc("GET /api/sites", handlers.GetSitesHandler)
	mux.HandleFunc("POST /api/sites", handlers.CreateSiteHandler)

	mux.HandleFunc("GET /api/devices", handlers.GetDevicesHandler)
	mux.HandleFunc("GET /api/sites/{site_id}/devices", handlers.GetSiteDevicesHandler)
	mux.HandleFunc("POST /api/devices/{device_id}/adopt", handlers.AdoptDeviceHandler)

	mux.HandleFunc("POST /api/sites/{site_id}/wlans", handlers.CreateWLANHandler)
	mux.HandleFunc("GET /api/sites/{site_id}/wlans", handlers.GetWLANsHandler)
	mux.HandleFunc("GET /api/devices/{device_id}/config", handlers.GetDeviceConfigHandler)

	return mux
}
