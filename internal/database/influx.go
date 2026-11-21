package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"openwrt-controller/internal/models"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

var (
	InfluxClient influxdb2.Client
	WriteAPI     api.WriteAPIBlocking
	bucket       string
	org          string
)

func InitInflux() error {
	url := os.Getenv("INFLUX_URL")
	token := os.Getenv("INFLUX_TOKEN")
	org = os.Getenv("INFLUX_ORG")
	bucket = os.Getenv("INFLUX_BUCKET")

	if url == "" {
		url = "http://localhost:8086"
	}
	if org == "" {
		org = "openwrthub"
	}
	if bucket == "" {
		bucket = "telemetry"
	}

	InfluxClient = influxdb2.NewClient(url, token)

	// Check connection with timeout to prevent blocking startup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := InfluxClient.Health(ctx)
	if err != nil {
		log.Printf("Warning: InfluxDB health check failed (non-fatal): %v", err)
		// Continue anyway — InfluxDB may become available later
	}

	WriteAPI = InfluxClient.WriteAPIBlocking(org, bucket)
	log.Println("InfluxDB initialized successfully")
	return nil
}

func WriteMetrics(deviceID string, metrics *models.DeviceMetrics) error {
	if WriteAPI == nil {
		return fmt.Errorf("influx write api is not initialized")
	}

	p := influxdb2.NewPointWithMeasurement("device_metrics").
		AddTag("device_id", deviceID).
		AddField("cpu_load", metrics.CPULoad).
		AddField("ram_free", metrics.RAMFree).
		AddField("uptime", metrics.Uptime).
		AddField("dhcp_clients", metrics.DHCPClients).
		AddField("signal_dbm", metrics.SignalDBM).
		AddField("rx_mbps", metrics.RxMbps).
		AddField("tx_mbps", metrics.TxMbps).
		SetTime(time.Now())

	return WriteAPI.WritePoint(context.Background(), p)
}

func GetInfluxOrg() string {
	if org == "" {
		return "openwrthub"
	}
	return org
}

func GetInfluxBucket() string {
	if bucket == "" {
		return "telemetry"
	}
	return bucket
}

func GetDeviceMetrics(deviceID string, duration string) ([]float64, error) {
	if InfluxClient == nil {
		return nil, fmt.Errorf("influx client not initialized")
	}

	queryAPI := InfluxClient.QueryAPI(org)
	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s)
		|> filter(fn: (r) => r["_measurement"] == "device_metrics")
		|> filter(fn: (r) => r["device_id"] == "%s")
		|> filter(fn: (r) => r["_field"] == "tx_mbps")
		|> aggregateWindow(every: 10s, fn: mean, createEmpty: false)
		|> yield(name: "mean")
	`, bucket, duration, deviceID)

	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var metrics []float64
	for result.Next() {
		if val, ok := result.Record().Value().(float64); ok {
			metrics = append(metrics, val)
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return metrics, nil
}

type TimeValuePair struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// GetSiteHistory queries InfluxDB for the aggregated metric over 24h
// deviceIDs should be the list of MAC addresses in the site
func GetSiteHistory(deviceIDs []string, metric string) ([]TimeValuePair, error) {
	if InfluxClient == nil {
		return nil, fmt.Errorf("influx client not initialized")
	}

	if len(deviceIDs) == 0 {
		return []TimeValuePair{}, nil
	}

	var fieldFilter string
	var fn string

	switch metric {
	case "signal":
		fieldFilter = `r["_field"] == "signal_dbm"`
		fn = "mean"
	case "traffic":
		// Traffic is sum of rx and tx, but since we need a single metric or sum, let's just chart sum of RX for now
		// or chart both. The prompt asked for: "El acumulado de Mbps (TX/RX) por interfaz".
		// We'll chart rx sum for simplicity, or we can write a more complex flux. Let's just track tx_mbps + rx_mbps or just rx
		fieldFilter = `(r["_field"] == "rx_mbps" or r["_field"] == "tx_mbps")`
		fn = "sum"
	case "cpu":
		fieldFilter = `r["_field"] == "cpu_load"`
		fn = "mean"
	default:
		return nil, fmt.Errorf("invalid metric type")
	}

	// build device filter: r.device_id == "A" or r.device_id == "B" ...
	deviceFilter := ""
	for i, id := range deviceIDs {
		if i > 0 {
			deviceFilter += " or "
		}
		deviceFilter += fmt.Sprintf(`r["device_id"] == "%s"`, id)
	}

	queryAPI := InfluxClient.QueryAPI(org)

	var query string
	if metric == "traffic" {
		query = fmt.Sprintf(`
			from(bucket: "%s")
			|> range(start: -24h)
			|> filter(fn: (r) => r["_measurement"] == "device_metrics")
			|> filter(fn: (r) => %s)
			|> filter(fn: (r) => %s)
			|> aggregateWindow(every: 5m, fn: mean, createEmpty: false)
			|> group(columns: ["_time"])
			|> sum()
			|> sort(columns: ["_time"])
		`, bucket, fieldFilter, deviceFilter)
	} else {
		query = fmt.Sprintf(`
			from(bucket: "%s")
			|> range(start: -24h)
			|> filter(fn: (r) => r["_measurement"] == "device_metrics")
			|> filter(fn: (r) => %s)
			|> filter(fn: (r) => %s)
			|> aggregateWindow(every: 5m, fn: mean, createEmpty: false)
			|> group(columns: ["_time"])
			|> %s()
			|> sort(columns: ["_time"])
		`, bucket, fieldFilter, deviceFilter, fn)
	}

	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var data []TimeValuePair
	for result.Next() {
		if val, ok := result.Record().Value().(float64); ok {
			data = append(data, TimeValuePair{
				Time:  result.Record().Time(),
				Value: val,
			})
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return data, nil
}

func CloseInflux() {
	if InfluxClient != nil {
		InfluxClient.Close()
	}
}

type FlowAnalytic struct {
	MAC   string
	Port  int
	Conns int
}

func WriteFlowAnalyticsBatch(deviceID string, flows []FlowAnalytic) error {
	if WriteAPI == nil {
		return fmt.Errorf("influx write api is not initialized")
	}

	if len(flows) == 0 {
		return nil
	}

	now := time.Now()
	for _, f := range flows {
		p := influxdb2.NewPointWithMeasurement("client_flows").
			AddTag("device_id", deviceID).
			AddTag("mac", f.MAC).
			AddTag("dport", fmt.Sprintf("%d", f.Port)).
			AddField("conns", f.Conns).
			SetTime(now)

		err := WriteAPI.WritePoint(context.Background(), p)
		if err != nil {
			return err
		}
	}

	return nil
}

// ── WIFI_SURVEY / per-client signal time-series ──────────────────────────────
// Written by the OpenWrt agent while a survey is active (2s cadence). The
// survey_worker pulls from this series to correlate phone GPS with the
// signal the AP saw from that MAC.

type ClientSignalSample struct {
	DeviceID   string
	MAC        string
	SurveyID   string
	SignalDBM  float64
	NoiseDBM   float64
	RxRate     float64
	TxRate     float64
	InactiveMs int64
	Time       time.Time
}

// WriteClientSignalBatch writes one point per sample. Tags are intentionally
// bounded (device_id, mac, survey_id) to keep Influx cardinality sane.
func WriteClientSignalBatch(samples []ClientSignalSample) error {
	if WriteAPI == nil {
		return fmt.Errorf("influx write api is not initialized")
	}
	if len(samples) == 0 {
		return nil
	}
	for _, s := range samples {
		p := influxdb2.NewPointWithMeasurement("client_signal").
			AddTag("device_id", s.DeviceID).
			AddTag("mac", s.MAC).
			AddTag("survey_id", s.SurveyID).
			AddField("signal_dbm", s.SignalDBM).
			AddField("noise_dbm", s.NoiseDBM).
			AddField("rx_rate", s.RxRate).
			AddField("tx_rate", s.TxRate).
			AddField("inactive_ms", s.InactiveMs).
			SetTime(s.Time)
		if err := WriteAPI.WritePoint(context.Background(), p); err != nil {
			return err
		}
	}
	return nil
}

const (
	surveyActiveInactiveMaxMs = 3000  // AP saw packets from phone within 3s -> actively serving
	surveyMaxInactiveMs       = 15000 // >15s inactive in assoclist -> stale ghost after roam/disconnect
)

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// selectBestSurveySignal chooses the best AP signal measurement around capturedAt.
// Stale ghost entries (>15s inactive) are ignored. Actively serving APs
// (inactive_ms <= 3000) always beat stale/fringe entries (>3000ms) even if the
// stale AP happened to report closer to capturedAt; within the same tier, the
// closest timestamp wins (tie-breaking on lower inactive_ms, time, device_id).
func selectBestSurveySignal(candidates []ClientSignalSample, capturedAt time.Time) (ClientSignalSample, bool) {
	var best ClientSignalSample
	bestTier := 99
	var bestDist time.Duration
	found := false

	for _, c := range candidates {
		if c.InactiveMs > surveyMaxInactiveMs {
			continue
		}
		tier := 0
		if c.InactiveMs > surveyActiveInactiveMaxMs {
			tier = 1
		}
		dist := absDuration(c.Time.Sub(capturedAt))
		if !found ||
			tier < bestTier ||
			(tier == bestTier && dist < bestDist) ||
			(tier == bestTier && dist == bestDist && c.InactiveMs < best.InactiveMs) ||
			(tier == bestTier && dist == bestDist && c.InactiveMs == best.InactiveMs && (c.Time.Before(best.Time) || (c.Time.Equal(best.Time) && c.DeviceID < best.DeviceID))) {
			best = c
			bestTier = tier
			bestDist = dist
			found = true
		}
	}
	return best, found
}

// GetClosestSurveySignal finds the best AP measurement near a GPS fix,
// preferring actively serving APs over stale assoclist entries left behind
// after a fast roam.
func GetClosestSurveySignal(ctx context.Context, mac, surveyID string, capturedAt time.Time) (ClientSignalSample, bool, error) {
	if mac == "" || surveyID == "" {
		return ClientSignalSample{}, false, fmt.Errorf("survey ID and MAC required")
	}
	if InfluxClient == nil {
		return ClientSignalSample{}, false, fmt.Errorf("influx client not initialized")
	}

	window := 10 * time.Second
	flux := fmt.Sprintf(`
		from(bucket: %q)
		  |> range(start: %s, stop: %s)
		  |> filter(fn: (r) => r["_measurement"] == "client_signal" and r["mac"] == %q and r["survey_id"] == %q)
		  |> pivot(rowKey: ["_time", "device_id"], columnKey: ["_field"], valueColumn: "_value")
		  |> filter(fn: (r) => exists r.signal_dbm)
		  |> group()
		  |> map(fn: (r) => ({r with offset_ns: int(v: r._time) - %d}))
		  |> map(fn: (r) => ({r with distance_ns: if r.offset_ns < 0 then -r.offset_ns else r.offset_ns}))
		  |> sort(columns: ["distance_ns", "_time", "device_id"])
		  |> limit(n: 32)
	`, bucket, capturedAt.Add(-window).UTC().Format(time.RFC3339Nano),
		capturedAt.Add(window).UTC().Format(time.RFC3339Nano), mac, surveyID, capturedAt.UnixNano())

	result, err := InfluxClient.QueryAPI(org).Query(ctx, flux)
	if err != nil {
		return ClientSignalSample{}, false, err
	}
	defer result.Close()

	var candidates []ClientSignalSample
	for result.Next() {
		rec := result.Record()
		out := ClientSignalSample{
			DeviceID: stringField(rec.ValueByKey("device_id")),
			MAC:      stringField(rec.ValueByKey("mac")),
			SurveyID: stringField(rec.ValueByKey("survey_id")),
			Time:     rec.Time(),
		}
		if v, ok := rec.ValueByKey("signal_dbm").(float64); ok {
			out.SignalDBM = v
		}
		if v, ok := rec.ValueByKey("noise_dbm").(float64); ok {
			out.NoiseDBM = v
		}
		if v, ok := rec.ValueByKey("rx_rate").(float64); ok {
			out.RxRate = v
		}
		if v, ok := rec.ValueByKey("tx_rate").(float64); ok {
			out.TxRate = v
		}
		if v, ok := rec.ValueByKey("inactive_ms").(int64); ok {
			out.InactiveMs = v
		} else if v, ok := rec.ValueByKey("inactive_ms").(float64); ok {
			out.InactiveMs = int64(v)
		}
		candidates = append(candidates, out)
	}
	if result.Err() != nil {
		return ClientSignalSample{}, false, result.Err()
	}
	best, ok := selectBestSurveySignal(candidates, capturedAt)
	return best, ok, nil
}

// GetReportingDevicesAround returns the lower-cased device_ids that reported
// device_metrics telemetry to InfluxDB within [capturedAt-window, capturedAt+window].
// Used to distinguish a real client Wi-Fi drop from an AP telemetry outage,
// even when GPS samples were queued offline and uploaded minutes later.
func GetReportingDevicesAround(ctx context.Context, capturedAt time.Time, window time.Duration) (map[string]bool, error) {
	if InfluxClient == nil {
		return nil, fmt.Errorf("influx client not initialized")
	}
	if window <= 0 {
		window = 30 * time.Second
	}
	flux := fmt.Sprintf(`
		from(bucket: %q)
		  |> range(start: %s, stop: %s)
		  |> filter(fn: (r) => r["_measurement"] == "device_metrics" and r["_field"] == "uptime")
		  |> group(columns: ["device_id"])
		  |> first()
		  |> group()
		  |> keep(columns: ["device_id"])
	`, bucket,
		capturedAt.Add(-window).UTC().Format(time.RFC3339Nano),
		capturedAt.Add(window).UTC().Format(time.RFC3339Nano))

	result, err := InfluxClient.QueryAPI(org).Query(ctx, flux)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	reporting := make(map[string]bool)
	for result.Next() {
		id := strings.ToLower(strings.TrimSpace(stringField(result.Record().ValueByKey("device_id"))))
		if id != "" {
			reporting[id] = true
		}
	}
	if result.Err() != nil {
		return nil, result.Err()
	}
	return reporting, nil
}

func stringField(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
