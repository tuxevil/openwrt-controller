package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
)

type WebhookEvent struct {
	Event     string      `json:"event"`
	Tenant    string      `json:"tenant"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

func DispatchWebhook(schema string, event string, payload interface{}) {
	rows, err := database.DB.Query("SELECT url, secret, events FROM " + schema + ".webhooks WHERE enabled = true")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var url, secret string
		var eventsJSON []byte
		if err := rows.Scan(&url, &secret, &eventsJSON); err != nil {
			continue
		}

		var events []string
		json.Unmarshal(eventsJSON, &events)

		subscribed := false
		for _, e := range events {
			if e == event {
				subscribed = true
				break
			}
		}

		if !subscribed {
			continue
		}

		whEvent := WebhookEvent{
			Event:     event,
			Tenant:    schema,
			Payload:   payload,
			Timestamp: time.Now(),
		}

		bodyBytes, _ := json.Marshal(whEvent)

		req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(bodyBytes))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		if secret != "" {
			h := hmac.New(sha256.New, []byte(secret))
			h.Write(bodyBytes)
			signature := hex.EncodeToString(h.Sum(nil))
			req.Header.Set("X-Nerve-Signature", "sha256="+signature)
		}

		go func(r *http.Request, u string) {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(r)
			if err != nil {
				log.Printf("Webhook dispatch failed to %s: %v", u, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				log.Printf("Webhook error %d from %s", resp.StatusCode, u)
			}
		}(req, url)
	}
}
