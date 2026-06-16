package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"openwrt-controller/internal/authtickets"
)

// WsTicketRequest is the JSON body of POST /api/ws-ticket. The
// optional DeviceID field lets a future iteration scope the ticket
// to a specific device (defence-in-depth: a stolen ticket for /ssh
// can't be re-used for /some-other-handler). Currently accepted
// but not enforced.
type WsTicketRequest struct {
	DeviceID string `json:"device_id,omitempty"`
}

// WsTicketResponse is the JSON returned by POST /api/ws-ticket. The
// client should immediately open the WebSocket (use the ticket
// within ExpiresIn seconds) and never store the ticket.
type WsTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"` // seconds until expiry
	ExpiresAt string `json:"expires_at"` // ISO-8601 timestamp
	TokenType string `json:"token_type"` // always "Ticket"
}

// IssueWSTicketHandler mints a short-lived single-use ticket that the
// client exchanges for a WebSocket upgrade. Requires a valid JWT
// (Authorization: Bearer ...).
//
// Flow:
//  1. Dashboard calls this endpoint (with the regular JWT in the
//     Authorization header) and receives a ticket.
//  2. Dashboard opens the WS with ?ticket=<ticket>.
//  3. The WS handler (DeviceSSHHandler) calls
//     authtickets.Store.Consume to atomically validate + mark the
//     ticket as used. A leaked ticket can therefore be used at
//     most once.
func IssueWSTicketHandler(w http.ResponseWriter, r *http.Request) {
	// Accept the JWT from the Authorization header only — never from
	// the query string (which is what we're trying to fix).
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"missing bearer token"}`))
		return
	}
	tok := strings.TrimPrefix(auth, "Bearer ")

	// Parse the JWT to extract the username + role. The ticket store
	// keeps these so the WS handler can log without re-parsing.
	parsed, err := jwt.Parse(tok, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return getJWTSecret(), nil
	})
	if err != nil || !parsed.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
		return
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid claims"}`))
		return
	}
	username, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	if username == "" {
		username = "system"
	}

	// Parse the body (we accept but currently ignore DeviceID).
	var body WsTicketRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	store := authtickets.GetStore()
	if store == nil {
		RespondError(w, http.StatusServiceUnavailable, "ws ticket store not initialised", nil)
		return
	}
	id, t, err := store.Issue(username, role)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to issue ticket", err)
		return
	}

	ttl := time.Until(t.ExpiresAt)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(WsTicketResponse{
		Ticket:    id,
		ExpiresIn: int(ttl.Seconds()),
		ExpiresAt: t.ExpiresAt.UTC().Format(time.RFC3339),
		TokenType: "Ticket",
	})
}
