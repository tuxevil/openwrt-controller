package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// ErrorResponse is the JSON envelope used by RespondError.
type ErrorResponse struct {
	Error   string `json:"error"`
	Request string `json:"request_id,omitempty"`
}

// RespondError writes a generic JSON error to the client and logs the
// detailed underlying error server-side. Use this in place of
// http.Error(w, err.Error(), status) so internal error messages (SQL
// errors, file paths, parser internals) do not leak to the caller.
func RespondError(w http.ResponseWriter, status int, publicMsg string, err error) {
	if err != nil {
		log.Printf("[api] %s: %v", publicMsg, err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: publicMsg})
}
