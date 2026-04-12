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
	
	// Check connection
	_, err := InfluxClient.Health(context.Background())
	if err != nil {
		return fmt.Errorf("failed to connect to influxdb: %w", err)
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

func CloseInflux() {
	if InfluxClient != nil {
		InfluxClient.Close()
	}
}
