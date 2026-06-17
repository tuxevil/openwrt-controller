package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"openwrt-controller/internal/api"
	"openwrt-controller/internal/authtickets"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/metrics"
	"openwrt-controller/internal/orchestrator"
	"openwrt-controller/internal/services"
)

// version is set at build time via -ldflags "-X main.version=…". The
// release workflow injects the tag here; local builds default to "dev".
var version = "dev"

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

	// CLI flags. Port value is normalised (":3000" or "3000" both work).
	port := flag.String("port", envOr("PORT", ":3000"), "TCP port to listen on (e.g. :3000 or :8443)")
	tlsCert := flag.String("tls-cert", os.Getenv("TLS_CERT"), "Path to TLS certificate (PEM). Enables HTTPS if set together with --tls-key.")
	tlsKey := flag.String("tls-key", os.Getenv("TLS_KEY"), "Path to TLS private key (PEM). Enables HTTPS if set together with --tls-cert.")
	requireTLS := flag.Bool("require-tls", envBool("REQUIRE_TLS", false), "If true, refuse to start without --tls-cert and --tls-key. Recommended for production.")
	logFormat := flag.String("log-format", envOr("LOG_FORMAT", "text"), "Log output format: text (default) or json.")
	flag.Parse()

	// Structured logger. The slog default is wired here so new code
	// uses it by default; legacy log.* calls are forwarded to the
	// same handler so everything is in the same stream.
	logger := newLogger(*logFormat)
	slog.SetDefault(logger)
	log.SetOutput(loggerWriter{logger: logger})
	log.SetFlags(0)

	// Normalise: if the port is missing the ':' prefix, add it.
	// This handles both the old .env convention (PORT=3000) and the
	// new one (PORT=:3000). Override via --port=:3000 or -port :3000.
	if !strings.HasPrefix(*port, ":") {
		*port = ":" + *port
	}

	// Initialise the metrics registry and inject it into the api
	// package. Custom metrics are added via the registry; /metrics
	// is wired by SetupRoutes.
	mreg := metrics.New()
	mreg.SetVersion(version)
	api.SetMetrics(mreg)
	logger.Info("metrics registry initialised", "version", version)

	// Initialize PostgreSQL
	if err := database.InitPostgres(); err != nil {
		logger.Warn("postgres init failed", "err", err)
	}

	// Initialize InfluxDB
	if err := database.InitInflux(); err != nil {
		logger.Warn("influx config/init failed", "err", err)
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

	// Build the route mux and wrap it with the metrics middleware.
	// The route label is taken from Go 1.22+ ServeMux patterns
	// (req.Pattern.Path) which are low cardinality by construction.
	mux := api.SetupRoutes()
	handler := mreg.Middleware(func(r *http.Request) string {
		// Go 1.22+ ServeMux exposes the matched pattern as a string
		// on the request. We use it as the route label so the
		// /api/sites/{site_id} cardinality is bounded.
		if r.Pattern != "" {
			return r.Pattern
		}
		return r.URL.Path
	})(mux)

	tlsEnabled := *tlsCert != "" && *tlsKey != ""
	if *requireTLS && !tlsEnabled {
		logger.Error("REQUIRE_TLS is set but --tls-cert/--tls-key are missing; refusing to start on plain HTTP")
		os.Exit(1)
	}

	// Build the http.Server with timeouts. IdleTimeout=120s is the
	// recommended value for keep-alive behind a reverse proxy
	// (Traefik / Caddy) which already enforces its own timeout.
	srv := &http.Server{
		Addr:              *port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if tlsEnabled {
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	// Graceful shutdown on SIGTERM (Docker / k8s default) or SIGINT
	// (Ctrl-C in dev). Drain for 15s before forcing a close.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		if tlsEnabled {
			logger.Info("starting openwrt-controller (HTTPS)", "addr", *port)
			if err := srv.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "err", err)
				os.Exit(1)
			}
			return
		}
		logger.Info("starting openwrt-controller (HTTP)", "addr", *port, "hint", "set REQUIRE_TLS=true for production")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-stop
	logger.Info("shutdown signal received; draining")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
}

// newLogger returns a slog.Logger configured for the requested output
// format. The "text" handler is human-readable; the "json" handler is
// aggregator-friendly (Loki, Datadog, ELK, etc.).
func newLogger(format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	switch strings.ToLower(format) {
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	default:
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
}

// loggerWriter adapts a *slog.Logger to the io.Writer interface that
// the legacy `log` package expects. Existing log.Printf calls are
// forwarded as a single Info record.
type loggerWriter struct{ logger *slog.Logger }

func (w loggerWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	w.logger.Info(msg)
	return len(p), nil
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
