package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"golang.org/x/crypto/ssh"
)

var pingAvgRegex = regexp.MustCompile(`(?:round-trip|rtt)[^=]*=\s*[\d.]+/([\d.]+)/`)

type LinkBaseline struct {
	Medium            string  `json:"medium"`
	ExpectedMbps      float64 `json:"expected_mbps"`
	ExpectedLatencyMs float64 `json:"expected_latency_ms"`
}

type NodeBenchmarkResult struct {
	DeviceID       string       `json:"device_id"`
	DeviceName     string       `json:"device_name"`
	IP             string       `json:"ip"`
	ThroughputMbps float64      `json:"throughput_mbps"`
	Retransmits    int          `json:"retransmits"`
	LatencyMs      float64      `json:"latency_ms"`
	Baseline       LinkBaseline `json:"baseline"`
	DeviationPct   float64      `json:"deviation_pct"`
	Status         string       `json:"status"` // "NOMINAL", "DEGRADED", "LATENCY_SPIKE", "ERROR"
	Assessment     string       `json:"assessment"`
	Anomaly        bool         `json:"anomaly"`
	Error          string       `json:"error,omitempty"`
}

type SiteBenchmarkReport struct {
	SiteID         string                `json:"site_id"`
	Timestamp      time.Time             `json:"timestamp"`
	Nodes          []NodeBenchmarkResult `json:"nodes"`
	OverallStatus  string                `json:"overall_status"`
	AnomaliesCount int                   `json:"anomalies_count"`
}

// Known or default physical baselines
func GetBaselineForDevice(deviceID, ip string) LinkBaseline {
	switch ip {
	case "10.128.128.1":
		return LinkBaseline{
			Medium:            "Gigabit Ethernet (IPQ4019 Local CPU)",
			ExpectedMbps:      450.0,
			ExpectedLatencyMs: 1.0,
		}
	case "10.128.128.2":
		return LinkBaseline{
			Medium:            "Fast Ethernet (100M PoE Switch)",
			ExpectedMbps:      80.0,
			ExpectedLatencyMs: 1.5,
		}
	case "10.128.128.3":
		return LinkBaseline{
			Medium:            "Powerline PLC (Electrical)",
			ExpectedMbps:      35.0,
			ExpectedLatencyMs: 3.5,
		}
	default:
		return LinkBaseline{
			Medium:            "Auto-Detected Link",
			ExpectedMbps:      0,
			ExpectedLatencyMs: 2.0,
		}
	}
}

func baselineForDevice(config json.RawMessage, deviceID, ip string) LinkBaseline {
	var baselines map[string]LinkBaseline
	if json.Unmarshal(config, &baselines) == nil {
		if baseline, ok := baselines[deviceID]; ok {
			return baseline
		}
		if baseline, ok := baselines[ip]; ok {
			return baseline
		}
	}
	return GetBaselineForDevice(deviceID, ip)
}

// EvaluateBenchmark compares measured metrics against the established baseline.
// It explicitly ignores known limits (e.g. 100M switch or PLC) and only flags
// deviations that drop below expected capacity or suffer abnormal latency.
func EvaluateBenchmark(measuredMbps, latencyMs float64, baseline LinkBaseline) (status string, deviation float64, assessment string, isAnomaly bool) {
	if baseline.ExpectedMbps <= 0 {
		return "NOMINAL", 0, "No static baseline set; operating as reference.", false
	}

	deviation = ((measuredMbps - baseline.ExpectedMbps) / baseline.ExpectedMbps) * 100.0

	// Check if latency is abnormally high (> 2.5x baseline and > 5ms)
	highLatency := baseline.ExpectedLatencyMs > 0 && latencyMs > (baseline.ExpectedLatencyMs*2.5) && latencyMs > 5.0

	if deviation >= -20.0 && !highLatency {
		return "NOMINAL", deviation, fmt.Sprintf("Operating within normal parameters for %s (%.1f Mbps vs %.1f Mbps baseline, %.1f ms RTT).",
			baseline.Medium, measuredMbps, baseline.ExpectedMbps, latencyMs), false
	}

	if highLatency && deviation >= -20.0 {
		return "LATENCY_SPIKE", deviation, fmt.Sprintf("Latency anomaly detected on %s: %.1f ms (expected ~%.1f ms). Potential electrical noise or queue delay.",
			baseline.Medium, latencyMs, baseline.ExpectedLatencyMs), true
	}

	if deviation < -20.0 {
		return "DEGRADED", deviation, fmt.Sprintf("Throughput degraded by %.1f%% below baseline on %s: measured %.1f Mbps (expected ~%.1f Mbps).",
			-deviation, baseline.Medium, measuredMbps, baseline.ExpectedMbps), true
	}

	return "NOMINAL", deviation, "Operating normally.", false
}

func MeasurePingLatency(targetIP string) float64 {
	cmd := exec.Command("ping", "-c", "3", "-W", "1", targetIP)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	m := pingAvgRegex.FindStringSubmatch(string(out))
	if len(m) > 1 {
		if lat, err := strconv.ParseFloat(m[1], 64); err == nil {
			return lat
		}
	}
	return 0
}

func executeRemoteSSH(ip string, cmdStr string) error {
	ks := orchestrator.GetKeyStore()
	if ks == nil {
		return fmt.Errorf("SSH key not loaded")
	}
	signer, err := ks.Get()
	if err != nil || signer == nil {
		return fmt.Errorf("SSH signer error: %v", err)
	}

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
		Timeout:         5 * time.Second,
	}

	conn, err := ssh.Dial("tcp", ip+":22", cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	sess, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	return sess.Run(cmdStr)
}

func RunNodeBenchmark(ctx context.Context, deviceID, deviceName, targetIP string, baseline LinkBaseline) NodeBenchmarkResult {
	res := NodeBenchmarkResult{
		DeviceID:   deviceID,
		DeviceName: deviceName,
		IP:         targetIP,
		Baseline:   baseline,
	}

	// 1. Measure ICMP Ping Latency
	res.LatencyMs = MeasurePingLatency(targetIP)

	// 2. Start iperf3 server daemon on the node
	_ = executeRemoteSSH(targetIP, "killall iperf3 2>/dev/null || true; iperf3 -s -D")
	defer func() {
		_ = executeRemoteSSH(targetIP, "killall iperf3 2>/dev/null || true")
	}()

	// Wait up to 3 seconds for port 5201 to accept connections
	for i := 0; i < 6; i++ {
		conn, err := net.DialTimeout("tcp", targetIP+":5201", 500*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 3. Run controller-side client test (3 seconds, JSON output)
	cmd := exec.CommandContext(ctx, "iperf3", "-c", targetIP, "-t", "3", "-J")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		res.Status = "ERROR"
		res.Error = fmt.Sprintf("iperf3 failed: %v, %s", err, stderr.String())
		res.Assessment = "Unable to complete throughput test."
		return res
	}

	// 4. Parse iperf3 JSON
	var iperfRaw struct {
		End struct {
			SumSent struct {
				BitsPerSecond float64 `json:"bits_per_second"`
				Retransmits   int     `json:"retransmits"`
			} `json:"sum_sent"`
			SumReceived struct {
				BitsPerSecond float64 `json:"bits_per_second"`
			} `json:"sum_received"`
		} `json:"end"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &iperfRaw); err != nil {
		res.Status = "ERROR"
		res.Error = fmt.Sprintf("failed to parse iperf3 JSON: %v", err)
		return res
	}

	bps := iperfRaw.End.SumSent.BitsPerSecond
	if bps == 0 {
		bps = iperfRaw.End.SumReceived.BitsPerSecond
	}
	res.ThroughputMbps = bps / 1000000.0
	res.Retransmits = iperfRaw.End.SumSent.Retransmits

	// If baseline was auto, initialize it from measurement
	if res.Baseline.ExpectedMbps <= 0 {
		if res.ThroughputMbps >= 500 {
			res.Baseline.Medium = "Gigabit Ethernet"
			res.Baseline.ExpectedMbps = 850.0
			res.Baseline.ExpectedLatencyMs = 1.0
		} else if res.ThroughputMbps >= 60 {
			res.Baseline.Medium = "Fast Ethernet"
			res.Baseline.ExpectedMbps = 80.0
			res.Baseline.ExpectedLatencyMs = 1.5
		} else {
			res.Baseline.Medium = "Restricted Link (PLC/WiFi)"
			res.Baseline.ExpectedMbps = 35.0
			res.Baseline.ExpectedLatencyMs = 3.5
		}
	}

	// 5. Evaluate against baseline
	res.Status, res.DeviationPct, res.Assessment, res.Anomaly = EvaluateBenchmark(res.ThroughputMbps, res.LatencyMs, res.Baseline)

	// 6. Push to InfluxDB if available
	RecordBenchmarkMetric(res)

	return res
}

func RecordBenchmarkMetric(r NodeBenchmarkResult) {
	if database.WriteAPI == nil {
		return
	}
	p := influxdb2.NewPointWithMeasurement("mesh_benchmarks").
		AddTag("device_id", r.DeviceID).
		AddTag("ip", r.IP).
		AddTag("status", r.Status).
		AddField("throughput_mbps", r.ThroughputMbps).
		AddField("latency_ms", r.LatencyMs).
		AddField("retransmits", r.Retransmits).
		AddField("deviation_pct", r.DeviationPct).
		SetTime(time.Now())

	_ = database.WriteAPI.WritePoint(context.Background(), p)
}

func RunSiteMeshBenchmark(ctx context.Context, schema, siteID string) (*SiteBenchmarkReport, error) {
	rows, err := database.DB.Query(fmt.Sprintf(
		"SELECT id, COALESCE(name, model, id), last_ip FROM %s.devices WHERE site_id = $1 AND status != 'OFFLINE'",
		schema,
	), siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type devRow struct {
		id   string
		name string
		ip   string
	}
	var targetDevs []devRow

	for rows.Next() {
		var d devRow
		if err := rows.Scan(&d.id, &d.name, &d.ip); err == nil && d.ip != "" {
			targetDevs = append(targetDevs, d)
		}
	}

	report := &SiteBenchmarkReport{
		SiteID:        siteID,
		Timestamp:     time.Now(),
		Nodes:         make([]NodeBenchmarkResult, 0),
		OverallStatus: "HEALTHY",
	}

	cfg, err := GetSiteConfig(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for _, d := range targetDevs {
		baseline := baselineForDevice(cfg.BenchmarkBaseline, d.id, d.ip)
		nodeRes := RunNodeBenchmark(ctx, d.id, d.name, d.ip, baseline)
		report.Nodes = append(report.Nodes, nodeRes)
		if nodeRes.Anomaly {
			report.AnomaliesCount++
			report.OverallStatus = "DEGRADED"
		}
	}

	return report, nil
}
