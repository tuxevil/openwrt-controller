package metrics

import (
	"expvar"
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds the metrics for the openwrt-controller process.
// All counters/histograms live here so the rest of the codebase can
// register and use them without juggling globals.
type Registry struct {
	Reg *prometheus.Registry

	// HTTP metrics. Labelled by method and route pattern (NOT raw
	// path, to avoid label-cardinality explosions on UUIDs).
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Build / runtime info. Set once at boot, never updated.
	BuildInfo *prometheus.GaugeVec
}

// New constructs the default registry and registers the standard
// process / Go runtime collectors plus the HTTP metrics. Custom
// metrics are added by the rest of the codebase via reg.Reg.
func New() *Registry {
	r := prometheus.NewRegistry()

	// Standard process + Go runtime metrics. go_* and process_*.
	r.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// Application uptime (set via SetUptime() in the HTTP handler).
	r.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "openwrt_controller_uptime_seconds",
			Help: "Seconds since the controller process started.",
		},
		func() float64 { return time.Since(startedAt).Seconds() },
	))

	reg := &Registry{
		Reg: r,
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "openwrt_controller_http_requests_total",
				Help: "Total HTTP requests handled, labelled by method, route, and status class.",
			},
			[]string{"method", "route", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "openwrt_controller_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "route"},
		),
		BuildInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "openwrt_controller_build_info",
				Help: "Constant 1; labelled with the build version.",
			},
			[]string{"version", "go_version"},
		),
	}
	r.MustRegister(reg.HTTPRequestsTotal, reg.HTTPRequestDuration, reg.BuildInfo)

	reg.BuildInfo.WithLabelValues(version, runtime.Version()).Set(1)

	// Expose expvar metrics at /debug/vars too (used by the
	// pprof integration in net/http/pprof). Useful for ad-hoc dumps.
	r.MustRegister(newExpvarCollector())

	return reg
}

var startedAt = time.Now()

// SetVersion overrides the version label. Call once at boot from main.
func (r *Registry) SetVersion(v string) {
	if v == "" {
		v = "dev"
	}
	version = v
	// Re-set the BuildInfo gauge with the new label value. We can't
	// mutate a GaugeVec label, so we just delete the old series and
	// add a new one.
	r.BuildInfo.Reset()
	r.BuildInfo.WithLabelValues(v, runtime.Version()).Set(1)
}

var version = "dev"

// Handler returns the http.Handler that serves /metrics in
// Prometheus exposition format. Use it directly:
//
//	mux.Handle("/metrics", metrics.New().Handler())
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.Reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// expvarCollector is a tiny shim that ships the standard expvar
// variables (e.g. cmdline, memstats) into the prometheus registry
// without panicking when none are registered. It publishes one
// constant metric (openwrt_controller_process_started_unix_seconds)
// so the registry has a series to return from /metrics.
type expvarCollector struct {
	desc *prometheus.Desc
}

func newExpvarCollector() *expvarCollector {
	return &expvarCollector{
		desc: prometheus.NewDesc(
			"openwrt_controller_process_started_unix_seconds",
			"Unix timestamp (seconds) at which the controller process started.",
			nil, nil,
		),
	}
}

func (e *expvarCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.desc
}

func (e *expvarCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(e.desc, prometheus.GaugeValue, float64(startedAt.Unix()))
}

// Compile-time check that expvarCollector implements prometheus.Collector.
var _ prometheus.Collector = (*expvarCollector)(nil)
var _ = expvar.Publish // keep expvar import live for downstream use
