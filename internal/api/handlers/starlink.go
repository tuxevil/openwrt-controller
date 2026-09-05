package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"openwrt-controller/internal/database"
)

type StarlinkStatus struct {
	Status        string    `json:"status"`
	DishRTTMs     float64   `json:"dish_rtt_ms"`
	PopRTTMs      float64   `json:"pop_rtt_ms"`
	PacketLossPct float64   `json:"packet_loss_pct"`
	MicroOutage   bool      `json:"micro_outage"`
	LastSampledAt time.Time `json:"last_sampled_at"`
	HealthRating  string    `json:"health_rating"`
}

func GetSiteStarlinkStatusHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, `{"error":"site_id required"}`, http.StatusBadRequest)
		return
	}

	res := StarlinkStatus{
		Status:        "OFFLINE",
		HealthRating:  "UNKNOWN",
		LastSampledAt: time.Now(),
	}

	if database.InfluxClient == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
		return
	}

	queryAPI := database.InfluxClient.QueryAPI(database.GetInfluxOrg())
	query := `from(bucket: "` + database.GetInfluxBucket() + `")
		|> range(start: -2m)
		|> filter(fn: (r) => r["_measurement"] == "starlink_health")
		|> last()`

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := queryAPI.Query(ctx, query)
	if err == nil {
		for result.Next() {
			record := result.Record()
			field := record.Field()
			val := record.Value()
			res.LastSampledAt = record.Time()

			if statusTag, ok := record.ValueByKey("status").(string); ok && statusTag != "" {
				res.Status = statusTag
			}

			switch field {
			case "dish_rtt_ms":
				if v, ok := val.(float64); ok {
					res.DishRTTMs = v
				}
			case "pop_rtt_ms":
				if v, ok := val.(float64); ok {
					res.PopRTTMs = v
				}
			case "packet_loss":
				if v, ok := val.(float64); ok {
					res.PacketLossPct = v
				}
			case "micro_outage":
				if v, ok := val.(int64); ok {
					res.MicroOutage = (v == 1)
				}
			}
		}
	}

	// Compute rating
	if res.Status == "ONLINE" {
		if res.DishRTTMs < 5.0 && res.PopRTTMs < 55.0 && res.PacketLossPct == 0 {
			res.HealthRating = "OPTIMAL"
		} else {
			res.HealthRating = "SATISFACTORY"
		}
	} else if res.Status == "MICRO_OUTAGE" {
		res.HealthRating = "MICRO_OUTAGE"
	} else if res.Status == "DEGRADED" {
		res.HealthRating = "DEGRADED"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
