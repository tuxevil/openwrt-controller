package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

var (
	// In-memory tracker for consecutive CPU overloads
	lastCPULoad = make(map[string]float64)
	cpuMu       sync.Mutex
)

// ProcessTelemetry is called exactly when a device successfully reports telemetry.
// This is where we evaluate SIGNAL_CRITICAL and CPU_OVERLOAD, and potentially resolve NODE_DOWN.
func ProcessTelemetry(deviceID string, siteID string, metrics models.DeviceMetrics) {
	// 1. Resolve NODE_DOWN if it exists (since we just got telemetry, it is up)
	ResolveIncident("NODE_DOWN", deviceID)

	// 2. Evaluate SIGNAL_CRITICAL
	if metrics.SignalDBM != 0 && metrics.SignalDBM < -80 {
		OpenIncident("SIGNAL_CRITICAL", deviceID, siteID, "CRITICAL")
	} else {
		ResolveIncident("SIGNAL_CRITICAL", deviceID)
	}

	// 3. Evaluate CPU_OVERLOAD (Supera el 90% en DOS muestras consecutivas)
	cpuMu.Lock()
	lastCPU, exists := lastCPULoad[deviceID]
	if exists && lastCPU > 0.90 && metrics.CPULoad > 0.90 {
		OpenIncident("CPU_OVERLOAD", deviceID, siteID, "WARNING")
	} else if metrics.CPULoad <= 0.90 {
		ResolveIncident("CPU_OVERLOAD", deviceID)
	}
	lastCPULoad[deviceID] = metrics.CPULoad
	cpuMu.Unlock()
}

// StartAlertEngine kicks off periodic tasks, like checking for DEAD devices (NODE_DOWN)
func StartAlertEngine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			checkDownNodes()
		}
	}()
	log.Println("[THE SIGNAL] Reactive Alert Engine Online.")
}

func checkDownNodes() {
	// Find nodes where last_seen_at is > 60 seconds ago and status is not 'offline' in memory,
	// but to rely on DB: we just query DB.
	query := `
		SELECT id, site_id 
		FROM devices 
		WHERE extract(epoch from (CURRENT_TIMESTAMP - last_seen_at)) > 60
	`
	rows, err := database.DB.Query(query)
	if err != nil {
		log.Printf("[THE SIGNAL] Error checking down nodes: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var devID, siteID string
		// Since siteID could be null but references a UUID
		if err := rows.Scan(&devID, &siteID); err == nil {
			OpenIncident("NODE_DOWN", devID, siteID, "CRITICAL")
		}
	}
}

func OpenIncident(incidentType, deviceID, siteID, severity string) {
	// Idempotent: check if OPEN exists
	var existingID string
	err := database.DB.QueryRow(`
		SELECT id FROM incidents 
		WHERE device_id = $1 AND incident_type = $2 AND status = 'OPEN'
	`, deviceID, incidentType).Scan(&existingID)

	if err == nil && existingID != "" {
		return // Already open
	}

	// Create newly!
	_, err = database.DB.Exec(`
		INSERT INTO incidents (site_id, device_id, incident_type, severity)
		VALUES ($1, $2, $3, $4)
	`, siteID, deviceID, incidentType, severity)
	if err != nil {
		log.Printf("[THE SIGNAL] Failed to open incident: %v", err)
		return
	}

	msg := fmt.Sprintf("[!] ALERT: %s | Device: %s", incidentType, deviceID)
	log.Printf("\x1b[31m%s\x1b[0m", msg) // Red in terminal
	notifyTelegram(msg)
}

func ResolveIncident(incidentType, deviceID string) {
	_, err := database.DB.Exec(`
		UPDATE incidents 
		SET status = 'RESOLVED', resolved_at = CURRENT_TIMESTAMP
		WHERE device_id = $1 AND incident_type = $2 AND status = 'OPEN'
	`, deviceID, incidentType)
	if err != nil {
		log.Printf("[THE SIGNAL] Failed to resolve incident: %v", err)
	}
}

func notifyTelegram(message string) {
	settings := database.GetPlatformSettings()
	token := settings.TelegramBotToken
	chatID := settings.TelegramChatID
	if token == "" || chatID == "" {
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    "🤖 *[MATRIX_CONTROLLER]*\n" + message,
		"parse_mode": "Markdown",
	}
	body, _ := json.Marshal(payload)
	go http.Post(url, "application/json", bytes.NewBuffer(body))
}
