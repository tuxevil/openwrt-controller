package handlers

import (
	"encoding/json"
	"net/http"
	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
)

func GetWebhooksHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)

	rows, err := database.Tx(r.Context()).Query("SELECT id, url, secret, events, enabled, created_at FROM " + schema + ".webhooks")
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var webhooks []models.Webhook
	for rows.Next() {
		var wh models.Webhook
		var eventsJSON []byte
		if err := rows.Scan(&wh.ID, &wh.URL, &wh.Secret, &eventsJSON, &wh.Enabled, &wh.CreatedAt); err == nil {
			json.Unmarshal(eventsJSON, &wh.Events)
			webhooks = append(webhooks, wh)
		}
	}
	
	if webhooks == nil {
		webhooks = make([]models.Webhook, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

func CreateWebhookHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)

	var req models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	eventsJSON, _ := json.Marshal(req.Events)

	_, err := database.Tx(r.Context()).Exec(
		"INSERT INTO "+schema+".webhooks (url, secret, events, enabled) VALUES ($1, $2, $3, $4)",
		req.URL, req.Secret, eventsJSON, req.Enabled,
	)
	if err != nil {
		http.Error(w, `{"error":"failed to create webhook"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
}

func DeleteWebhookHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	whID := r.PathValue("webhook_id")

	_, err := database.Tx(r.Context()).Exec("DELETE FROM "+schema+".webhooks WHERE id = $1", whID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
