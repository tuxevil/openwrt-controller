package handlers

import (
	"encoding/json"
	"net/http"
	
	"openwrt-controller/internal/services"
)

type ChatOpsQueryRequest struct {
	Query string `json:"query"`
}

func ChatOpsQueryHandler(w http.ResponseWriter, r *http.Request) {
	var req ChatOpsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid payload"}`, http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, `{"error": "query is required"}`, http.StatusBadRequest)
		return
	}

	response, err := services.ProcessChatOpsQuery(req.Query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
