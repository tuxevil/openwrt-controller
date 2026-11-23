package services

import (
	"context"
	"encoding/json"
	"fmt"
	"openwrt-controller/internal/database"
)

type RFClientRecord struct {
	MAC      string  `json:"mac"`
	Hostname string  `json:"hostname,omitempty"`
	Signal   float64 `json:"signal"`
	Noise    float64 `json:"noise"`
	SNR      float64 `json:"snr"`
	TxRate   string  `json:"tx_rate"`
	RxRate   string  `json:"rx_rate"`
}

type RFHealthResult struct {
	OverallHealth int              `json:"overall_health"`
	OptimalChan   string           `json:"optimal_channel"`
	Diagnosis     string           `json:"diagnosis"`
	Clients       []RFClientRecord `json:"clients"`
}

func AnalyzeSiteRF(ctx context.Context, siteID string) (RFHealthResult, error) {
	lookup := database.LoadSiteClientHostnameLookup(ctx, siteID)

	rows, err := database.Tx(ctx).Query(`
		SELECT id, COALESCE(name, ''), COALESCE(last_ip, ''), state_json
		FROM devices 
		WHERE site_id = $1 AND state_json IS NOT NULL
	`, siteID)
	if err != nil {
		return RFHealthResult{}, fmt.Errorf("db error: %v", err)
	}
	defer rows.Close()

	var states []map[string]interface{}
	for rows.Next() {
		var id, name, lastIP string
		var stateJSON []byte
		if err := rows.Scan(&id, &name, &lastIP, &stateJSON); err == nil && len(stateJSON) > 0 {
			var state map[string]interface{}
			if err := json.Unmarshal(stateJSON, &state); err == nil {
				lookup.MergeDeviceState(id, name, lastIP, state)
				states = append(states, state)
			}
		}
	}

	clients := make([]RFClientRecord, 0)
	totalSNR := 0.0
	validClients := 0
	worstNoise := -200.0

	for _, state := range states {
		wsRaw, ok := state["wireless_stations"]
		if !ok || wsRaw == nil {
			continue
		}
		wsBytes, err := json.Marshal(wsRaw)
		if err != nil {
			continue
		}
		var stationsMap map[string][]map[string]interface{}
		if err := json.Unmarshal(wsBytes, &stationsMap); err == nil {
			for _, ifaceClients := range stationsMap {
				for _, apClient := range ifaceClients {
					mac, _ := apClient["mac"].(string)
					sig, _ := apClient["signal"].(float64)
					noise, _ := apClient["noise"].(float64)
					tx, _ := apClient["tx_rate"].(string)
					rx, _ := apClient["rx_rate"].(string)

					if sig == 0 {
						continue
					}
					// If missing noise from driver, assume reasonable baseline
					if noise == 0 {
						noise = -95
					}

					snr := sig - noise
					totalSNR += snr
					validClients++

					if noise > worstNoise {
						worstNoise = noise
					}

					clients = append(clients, RFClientRecord{
						MAC:      mac,
						Hostname: lookup.ResolveMAC(mac),
						Signal:   sig,
						Noise:    noise,
						SNR:      snr,
						TxRate:   tx,
						RxRate:   rx,
					})
				}
			}
		}
	}

	res := RFHealthResult{
		Clients:       clients,
		OverallHealth: 100,
		OptimalChan:   "11", // Default suggestion for 2.4Ghz fallback demo
		Diagnosis:     "OK",
	}

	if validClients == 0 {
		return res, nil
	}

	avgSNR := totalSNR / float64(validClients)

	// Simple heuristic for RF Health based on average SNR and worst Noise floor
	// Ideal SNR > 25dB. Noise floor ideally < -90dBm.
	healthPenalty := 0.0
	if avgSNR < 20 {
		healthPenalty += (20 - avgSNR) * 3
	}
	if worstNoise > -80 {
		// Severe floor noise penalty
		healthPenalty += (worstNoise + 80) * 2
	}

	finalHealth := int(100.0 - healthPenalty)
	if finalHealth < 0 {
		finalHealth = 0
	}
	if finalHealth > 100 {
		finalHealth = 100
	}
	res.OverallHealth = finalHealth

	if finalHealth < 60 {
		res.Diagnosis = "HIGH_INTERFERENCE"
		res.OptimalChan = "6" // Mock dynamic decision
	} else if finalHealth < 80 {
		res.Diagnosis = "DEGRADED_SNR"
	}

	return res, nil
}
