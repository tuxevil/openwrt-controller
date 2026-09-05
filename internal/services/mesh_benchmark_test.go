package services

import (
	"testing"
)

func TestEvaluateBenchmark_KnownBaselines(t *testing.T) {
	// Case 1: Ubiquiti on 100M switch delivering 81.7 Mbps
	baseline100M := LinkBaseline{
		Medium:            "Fast Ethernet (100M PoE Switch)",
		ExpectedMbps:      80.0,
		ExpectedLatencyMs: 1.5,
	}
	status, dev, assessment, isAnomaly := EvaluateBenchmark(81.7, 0.7, baseline100M)
	if isAnomaly {
		t.Errorf("expected 81.7 Mbps on 100M baseline to NOT be an anomaly, got assessment: %s", assessment)
	}
	if status != "NOMINAL" {
		t.Errorf("expected status NOMINAL, got %s", status)
	}
	if dev < 0 {
		t.Errorf("expected positive deviation, got %f", dev)
	}

	// Case 2: Netgear on Powerline delivering 38.5 Mbps
	baselinePLC := LinkBaseline{
		Medium:            "Powerline PLC (Electrical)",
		ExpectedMbps:      35.0,
		ExpectedLatencyMs: 3.5,
	}
	status, _, assessment, isAnomaly = EvaluateBenchmark(38.5, 2.4, baselinePLC)
	if isAnomaly {
		t.Errorf("expected 38.5 Mbps on PLC baseline to NOT be an anomaly, got: %s", assessment)
	}
	if status != "NOMINAL" {
		t.Errorf("expected status NOMINAL, got %s", status)
	}

	// Case 3: Powerline degraded by electrical noise (delivering 12 Mbps)
	status, dev, assessment, isAnomaly = EvaluateBenchmark(12.0, 3.2, baselinePLC)
	if !isAnomaly {
		t.Errorf("expected 12.0 Mbps on PLC baseline to BE an anomaly, got: %s", assessment)
	}
	if status != "DEGRADED" {
		t.Errorf("expected status DEGRADED, got %s", status)
	}
	if dev > -50.0 {
		t.Errorf("expected large negative deviation, got %f", dev)
	}

	// Case 4: High latency anomaly (RTT spiked to 18 ms on local 1.5 ms link)
	status, _, assessment, isAnomaly = EvaluateBenchmark(80.0, 18.0, baseline100M)
	if !isAnomaly {
		t.Errorf("expected 18 ms latency on local link to BE an anomaly, got: %s", assessment)
	}
	if status != "LATENCY_SPIKE" {
		t.Errorf("expected status LATENCY_SPIKE, got %s", status)
	}
}
