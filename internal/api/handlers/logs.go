package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"
)

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func GetLogsHandler(w http.ResponseWriter, r *http.Request) {
	// Mock generator for Matrix typewriter
	// We extract anomaly metrics probabilistically. Since dump_a0 lacks explicit log arrays natively, we inject brutalist flavor.
	
	logs := []LogEntry{}
	levels := []string{"INFO", "WARN", "CRIT"}

	for i := 0; i < 15; i++ {
		lvl := levels[rand.Intn(len(levels))]
		msg := "NETWORK_SYNC_OK"
		if lvl == "CRIT" {
			msg = "SECURITY_BREACH_AUTH_ATTEMPT_DENIED"
		} else if lvl == "WARN" {
			msg = "LATENCY_SPIKE_DETECTED CPU_TEMP_W>"
		} else {
			msgs := []string{"DEVICE_DHCP_LEASED MAC_ASSIGNED", "UPSTREAM_PACKET_DROPPED RECOVERED", "WLAN_HANDSHAKE_COMPLETED"}
			msg = msgs[rand.Intn(len(msgs))]
		}
		
		logs = append(logs, LogEntry{
			Timestamp: time.Now().Add(-time.Duration(i*3) * time.Minute).Format(time.RFC3339),
			Level:     lvl,
			Message:   msg,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": logs})
}
