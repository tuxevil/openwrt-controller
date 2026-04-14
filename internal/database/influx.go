package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"openwrt-controller/internal/models"
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
		|> filter(fn: (r) => r["_field"] == "cpu_load")
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
