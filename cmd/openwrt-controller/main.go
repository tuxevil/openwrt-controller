package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"openwrt-controller/internal/api"
	"openwrt-controller/internal/authtickets"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"
	"openwrt-controller/internal/services"
)

func main() {
	banner := `
  ██████╗ ███╗   ███╗███████╗ ██████╗  █████╗
 ██╔═══██╗████╗ ████║██╔════╝██╔════╝ ██╔══██╗
 ██║   ██╗██╔████╔██║█████╗  ██║  ███╗███████║
 ██║   ██╗██║╚██╔╝██║██╔══╝  ██║   ██║██╔══██║
 ╚██████╔╝██║ ╚═╝ ██║███████╗╚██████╔╝██║  ██║
  ╚═════╝ ╚═╝     ╚═╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝
  --- CENTRAL CONTROLLER ONLINE ---
`
	log.Println(banner)

	// CLI flags. Port and TLS material are intentionally explicit so that
	// production deployments fail closed instead of accidentally exposing
	// JWTs / API keys over plain HTTP.
	port := flag.String("port", envOr("PORT", ":3000"), "TCP port to listen on (e.g. :3000 or :8443)")
	tlsCert := flag.String("tls-cert", os.Getenv("TLS_CERT"), "Path to TLS certificate (PEM). Enables HTTPS if set together with --tls-key.")
	tlsKey := flag.String("tls-key", os.Getenv("TLS_KEY"), "Path to TLS private key (PEM). Enables HTTPS if set together with --tls-cert.")
	requireTLS := flag.Bool("require-tls", envBool("REQUIRE_TLS", false), "If true, refuse to start without --tls-cert and --tls-key. Recommended for production.")
	flag.Parse()

	// Initialize PostgreSQL
	if err := database.InitPostgres(); err != nil {
		log.Printf("Warning: Postgres init failed: %v\n", err)
	}

	// Initialize InfluxDB
	if err := database.InitInflux(); err != nil {
		log.Printf("Warning: Influx config/init failed: %v\n", err)
	}
	defer database.CloseInflux()

	// Load the controller's SSH private key once. All SSH-using packages
	// read from this single KeyStore instead of each init() reading the
	// file independently.
	orchestrator.LoadKeyStore()
	// The handlers package's RefreshSSHKeys() is now a no-op: the
	// orchestrator.GetKeyStore().Get() path is the canonical one.

	// Initialise the WebSocket ticket store (30s default TTL). The
	// store is used by /api/ws-ticket to issue single-use tickets
	// that the dashboard exchanges for a WebSocket upgrade, avoiding
	// the JWT ever appearing in the URL.
	authtickets.LoadStore(authtickets.DefaultTicketTTL)
	stopGC := make(chan struct{})
	authtickets.GetStore().StartGC(time.Minute, stopGC)
	defer close(stopGC)

	services.StartAlertEngine()
	services.StartSniperReaper()
	services.StartThreatIntelCron()

	mux := api.SetupRoutes()

	tlsEnabled := *tlsCert != "" && *tlsKey != ""
	if *requireTLS && !tlsEnabled {
		log.Fatalf("REQUIRE_TLS is set but --tls-cert/--tls-key are missing — refusing to start on plain HTTP")
	}

	if tlsEnabled {
		log.Printf("Starting openwrt-controller (HTTPS) on %s", *port)
		srv := &http.Server{
			Addr:      *port,
			Handler:   mux,
			TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}
		if err := srv.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
		return
	}

	log.Printf("Starting openwrt-controller (HTTP) on %s — set --tls-cert/--tls-key or REQUIRE_TLS=true for production", *port)
	if err := http.ListenAndServe(*port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	case "0", "false", "FALSE", "no", "NO":
		return false
	}
	return def
}
