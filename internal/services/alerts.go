package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

var (
	// In-memory tracker for consecutive CPU overloads. Each entry records
	// the last CPU load observed together with the time it was recorded so
	// pruneCPULoads() can evict entries for devices that have gone silent.
	lastCPULoad = make(map[string]cpuSample)
	cpuMu       sync.Mutex
)

type cpuSample struct {
	load     float64
	recorded time.Time
}

const cpuLoadRetention = 7 * 24 * time.Hour

// pruneCPULoads removes entries older than cpuLoadRetention. Without this,
// the map grew unboundedly as devices were added or went offline forever,
// leaking memory in long-running controllers.
func pruneCPULoads() {
	cpuMu.Lock()
	defer cpuMu.Unlock()
	cutoff := time.Now().Add(-cpuLoadRetention)
	for id, s := range lastCPULoad {
		if s.recorded.Before(cutoff) {
			delete(lastCPULoad, id)
		}
	}
}

// ProcessTelemetry is called exactly when a device successfully reports telemetry.
// This is where we evaluate SIGNAL_CRITICAL and CPU_OVERLOAD, and potentially resolve NODE_DOWN.
func ProcessTelemetry(schema, deviceID string, siteID string, metrics models.DeviceMetrics) {
	// 1. Resolve NODE_DOWN if it exists (since we just got telemetry, it is up)
	ResolveIncident(schema, "NODE_DOWN", deviceID)

	// 2. Evaluate SIGNAL_CRITICAL
	if metrics.SignalDBM != 0 && metrics.SignalDBM < -80 {
		OpenIncident(schema, "SIGNAL_CRITICAL", deviceID, siteID, "CRITICAL")
	} else {
		ResolveIncident(schema, "SIGNAL_CRITICAL", deviceID)
	}

	// 3. Evaluate CPU_OVERLOAD (Supera el 90% en DOS muestras consecutivas)
	cpuMu.Lock()
	lastCPU, exists := lastCPULoad[deviceID]
	if exists && lastCPU.load > 0.90 && metrics.CPULoad > 0.90 {
		OpenIncident(schema, "CPU_OVERLOAD", deviceID, siteID, "WARNING")
	} else if metrics.CPULoad <= 0.90 {
		ResolveIncident(schema, "CPU_OVERLOAD", deviceID)
	}
	lastCPULoad[deviceID] = cpuSample{load: metrics.CPULoad, recorded: time.Now()}
	cpuMu.Unlock()
}

// StartAlertEngine kicks off periodic tasks, like checking for DEAD devices (NODE_DOWN)
func StartAlertEngine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		pruneTicker := time.NewTicker(1 * time.Hour)
		defer pruneTicker.Stop()
		for {
			select {
			case <-ticker.C:
				checkDownNodes()
			case <-pruneTicker.C:
				pruneCPULoads()
			}
		}
	}()
	log.Println("[THE SIGNAL] Reactive Alert Engine Online.")
}

func checkDownNodes() {
	tenants, err := ListTenants()
	if err != nil {
		return
	}
	for _, t := range tenants {
		schema := "tenant_" + t.SchemaAlias
		query := fmt.Sprintf(`
			SELECT id, site_id 
			FROM %s.devices 
			WHERE extract(epoch from (CURRENT_TIMESTAMP - last_seen_at)) > 60
		`, schema)
		rows, err := database.DB.Query(query)
		if err != nil {
			continue
		}

		for rows.Next() {
			var devID, siteID string
			if err := rows.Scan(&devID, &siteID); err == nil {
				OpenIncident(schema, "NODE_DOWN", devID, siteID, "CRITICAL")
			}
		}
		rows.Close()
	}
}

func OpenIncident(schema, incidentType, deviceID, siteID, severity string) {
	// Idempotent: check if OPEN exists
	var existingID string
	err := database.DB.QueryRow(fmt.Sprintf(`
		SELECT id FROM %s.incidents 
		WHERE device_id = $1 AND incident_type = $2 AND status = 'OPEN'
	`, schema), deviceID, incidentType).Scan(&existingID)

	if err == nil && existingID != "" {
		return // Already open
	}

	// Create newly!
	_, err = database.DB.Exec(fmt.Sprintf(`
		INSERT INTO %s.incidents (site_id, device_id, incident_type, severity)
		VALUES ($1, $2, $3, $4)
	`, schema), siteID, deviceID, incidentType, severity)
	if err != nil {
		log.Printf("[THE SIGNAL] Failed to open incident: %v", err)
		return
	}

	msg := fmt.Sprintf("[!] ALERT: %s | Device: %s", incidentType, deviceID)
	log.Printf("\x1b[31m%s\x1b[0m", msg) // Red in terminal
	notifyTelegram(msg)

	go DispatchWebhook(schema, "incident_created", map[string]interface{}{
		"device_id":     deviceID,
		"site_id":       siteID,
		"incident_type": incidentType,
		"severity":      severity,
	})
}

func ResolveIncident(schema, incidentType, deviceID string) {
	_, err := database.DB.Exec(fmt.Sprintf(`
		UPDATE %s.incidents 
		SET status = 'RESOLVED', resolved_at = CURRENT_TIMESTAMP
		WHERE device_id = $1 AND incident_type = $2 AND status = 'OPEN'
	`, schema), deviceID, incidentType)
	if err != nil {
		log.Printf("[THE SIGNAL] Failed to resolve incident: %v", err)
	}
}

func notifyTelegram(message string) {
	settings := database.GetPlatformSettings()
	token := decryptIfSealed(settings.TelegramBotToken)
	chatID := settings.TelegramChatID
	if token == "" || chatID == "" {
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       "🤖 *[MATRIX_CONTROLLER]*\n" + message,
		"parse_mode": "Markdown",
	}
	body, _ := json.Marshal(payload)
	// Use a context with a short timeout so a stuck Telegram API does not
	// keep the goroutine around forever. We no longer use the bare
	// http.Post (which silently dropped errors); NewRequestWithContext
	// + client.Do lets us log non-2xx.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err != nil {
		log.Printf("[Telegram] send failed: %v", err)
	} else {
		_ = resp.Body.Close()
	}
}

// decryptIfSealed transparently handles both legacy plaintext tokens and
// the new envelope format. If the value does not parse as a base64
// envelope of the expected length, we fall back to the raw value so a
// misconfigured deployment (or an old DB row) still works.
func decryptIfSealed(raw string) string {
	if raw == "" {
		return ""
	}
	key, err := telegramEncryptionKey()
	if err != nil {
		return raw
	}
	env, err := DecodeEnvelope(raw)
	if err != nil {
		return raw // legacy plaintext
	}
	pt, err := Open(env, key)
	if err != nil {
		return raw
	}
	return pt
}

func telegramEncryptionKey() ([]byte, error) {
	p := os.Getenv("TELEGRAM_ENCRYPTION_KEY")
	if p == "" {
		return nil, ErrSecretKeyMissing
	}
	return DeriveKeyFromPassphrase(p), nil
}
