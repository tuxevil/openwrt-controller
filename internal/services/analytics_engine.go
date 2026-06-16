package services

import (
	"context"
	"fmt"
	"strings"

	"openwrt-controller/internal/database"
)

type SeriesPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

type TopTalkerData struct {
	MAC   string `json:"mac"`
	Bytes int64  `json:"bytes"`
}

type ProtocolData struct {
	Port  int    `json:"port"`
	Bytes int64  `json:"bytes"`
	Name  string `json:"name"`
}

var knownPorts = map[int]string{
	22:   "SSH",
	53:   "DNS",
	80:   "HTTP",
	123:  "NTP",
	443:  "HTTPS",
	6881: "BitTorrent",
}

func getSiteDevices(ctx context.Context, siteID string) ([]string, error) {
	rows, err := database.Tx(ctx).Query("SELECT id FROM devices WHERE site_id = $1", siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func GetWANThroughputHistory(ctx context.Context, siteID, timeRange string) (map[string][]SeriesPoint, error) {
	startStr := "-" + timeRange
	deviceIDs, err := getSiteDevices(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if len(deviceIDs) == 0 {
		return map[string][]SeriesPoint{"rx": {}, "tx": {}}, nil
	}

	bucket := "telemetry" // usually stored in env, assuming defaults like influx.go

	deviceFilter := ""
	for i, id := range deviceIDs {
		if i > 0 {
			deviceFilter += " or "
		}
		deviceFilter += fmt.Sprintf(`r["device_id"] == "%s"`, id)
	}

	window := "5m"
	if timeRange == "7d" {
		window = "1h"
	}

	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s)
		|> filter(fn: (r) => r["_measurement"] == "device_metrics")
		|> filter(fn: (r) => %s)
		|> filter(fn: (r) => r["_field"] == "rx_mbps" or r["_field"] == "tx_mbps")
		|> aggregateWindow(every: %s, fn: mean, createEmpty: false)
		|> group(columns: ["_time", "_field"])
		|> sum()
		|> group(columns: ["_field"])
		|> sort(columns: ["_time"])
	`, bucket, startStr, deviceFilter, window)

	if database.InfluxClient == nil {
		return nil, fmt.Errorf("influx client not initialized")
	}

	queryAPI := database.InfluxClient.QueryAPI("openwrthub")
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	rxPoints := []SeriesPoint{}
	txPoints := []SeriesPoint{}

	for result.Next() {
		val, ok := result.Record().Value().(float64)
		if !ok {
			continue
		}
		timeStr := result.Record().Time().Format("2006-01-02T15:04:05Z")
		pt := SeriesPoint{Time: timeStr, Value: val}

		if result.Record().Field() == "rx_mbps" {
			rxPoints = append(rxPoints, pt)
		} else if result.Record().Field() == "tx_mbps" {
			txPoints = append(txPoints, pt)
		}
	}

	return map[string][]SeriesPoint{"rx": rxPoints, "tx": txPoints}, nil
}

func GetTopTalkers(ctx context.Context, siteID, timeRange string) ([]TopTalkerData, error) {
	startStr := "-" + timeRange
	deviceIDs, err := getSiteDevices(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if len(deviceIDs) == 0 {
		return []TopTalkerData{}, nil
	}

	bucket := "telemetry"

	deviceFilter := ""
	for i, id := range deviceIDs {
		if i > 0 {
			deviceFilter += " or "
		}
		deviceFilter += fmt.Sprintf(`r["device_id"] == "%s"`, id)
	}

	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s)
		|> filter(fn: (r) => r["_measurement"] == "client_flows")
		|> filter(fn: (r) => %s)
		|> filter(fn: (r) => r["_field"] == "conns")
		|> group(columns: ["mac"])
		|> sum()
		|> group()
		|> Experimental.sort(columns: ["_value"], desc: true)
		|> limit(n: 10)
	`, bucket, startStr, deviceFilter)

	// Since Experimental.sort is not standard, we can just use regular sort
	query = strings.Replace(query, "Experimental.sort(columns: [\"_value\"], desc: true)", "sort(columns: [\"_value\"], desc: true)", 1)

	queryAPI := database.InfluxClient.QueryAPI("openwrthub")
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var talkers []TopTalkerData
	for result.Next() {
		mac, ok := result.Record().ValueByKey("mac").(string)
		if !ok {
			continue
		}
		conns, ok := result.Record().Value().(int64)
		if !ok {
			// Influx might return float64 for aggregates
			if fval, ok := result.Record().Value().(float64); ok {
				conns = int64(fval)
			}
		}

		talkers = append(talkers, TopTalkerData{
			MAC:   mac,
			Bytes: conns, // Temp using conns as bytes metric base
		})
	}

	return talkers, nil
}

func GetProtocolDistribution(ctx context.Context, siteID, timeRange string) ([]ProtocolData, error) {
	startStr := "-" + timeRange
	deviceIDs, err := getSiteDevices(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if len(deviceIDs) == 0 {
		return []ProtocolData{}, nil
	}

	bucket := "telemetry"

	deviceFilter := ""
	for i, id := range deviceIDs {
		if i > 0 {
			deviceFilter += " or "
		}
		deviceFilter += fmt.Sprintf(`r["device_id"] == "%s"`, id)
	}

	query := fmt.Sprintf(`
		from(bucket: "%s")
		|> range(start: %s)
		|> filter(fn: (r) => r["_measurement"] == "client_flows")
		|> filter(fn: (r) => %s)
		|> filter(fn: (r) => r["_field"] == "conns")
		|> group(columns: ["dport"])
		|> sum()
		|> group()
		|> sort(columns: ["_value"], desc: true)
		|> limit(n: 8)
	`, bucket, startStr, deviceFilter)

	queryAPI := database.InfluxClient.QueryAPI("openwrthub")
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var protos []ProtocolData
	for result.Next() {
		portStr, ok := result.Record().ValueByKey("dport").(string)
		if !ok {
			continue
		}

		port := 0
		fmt.Sscanf(portStr, "%d", &port)

		conns, ok := result.Record().Value().(int64)
		if !ok {
			if fval, ok := result.Record().Value().(float64); ok {
				conns = int64(fval)
			}
		}

		name := "Other"
		if kn, exists := knownPorts[port]; exists {
			name = kn
		} else if port > 0 {
			name = fmt.Sprintf("Port %d", port)
		}

		protos = append(protos, ProtocolData{
			Port:  port,
			Bytes: conns,
			Name:  name,
		})
	}

	return protos, nil
}
