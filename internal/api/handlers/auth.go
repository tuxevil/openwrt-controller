package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/secrets"
)

// LoginDBTimeout bounds the per-request Postgres work for the login
// handler. Without this, a degraded DB (network blip, exhausted
// connection pool, replica failover) would block the handler on
// QueryRow indefinitely and the user would see a hung browser.
// The value is intentionally short — bcrypt comparison is the slow
// step, not the lookup.
const LoginDBTimeout = 3 * time.Second

// jwtSecret is loaded lazily on first use so that test binaries can set
// JWT_SECRET before the secret is materialised. secrets.JWTSecret() calls
// log.Fatal on misconfiguration, which would otherwise abort the test
// process during package init().
var (
	jwtSecretOnce sync.Once
	jwtSecret     []byte
)

func getJWTSecret() []byte {
	jwtSecretOnce.Do(func() { jwtSecret = secrets.JWTSecret() })
	return jwtSecret
}

// JWTSecret exposes the secret for use in middleware. It is the same
// bytes returned by getJWTSecret() but as a value (not a function) so
// the existing call sites that hold a []byte don't need to change.
//
// Note: this name is also used as the package-level `var jwtSecret` for
// internal call sites; both are kept for backward compatibility.

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// Bound the DB work so a degraded Postgres doesn't hang the UI.
	// Regression: Jun 17 2026 — controller stopped accepting login
	// because DB pool was exhausted and QueryRow had no context
	// timeout, holding the request open until the browser gave up.
	ctx, cancel := context.WithTimeout(r.Context(), LoginDBTimeout)
	defer cancel()

	// Fetch user from DB (now includes tenant_id)
	var storedHash, role string
	var tenantID sql.NullString
	err := database.Tx(ctx).QueryRowContext(ctx,
		"SELECT password_hash, role, tenant_id FROM users WHERE username = $1",
		req.Username,
	).Scan(&storedHash, &role, &tenantID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"DATABASE_UNAVAILABLE"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"ACCESS_DENIED"}`))
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"ACCESS_DENIED"}`))
		return
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"ACCESS_DENIED"}`))
		return
	}

	// Build JWT claims
	claims := jwt.MapClaims{
		"sub":  req.Username,
		"role": role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}

	// If user has a tenant binding, resolve the schema_alias and include it
	var schemaAlias string
	if tenantID.Valid {
		err := database.Tx(ctx).QueryRowContext(ctx,
			"SELECT schema_alias FROM tenants WHERE id = $1 AND is_active = true",
			tenantID.String,
		).Scan(&schemaAlias)
		if err == nil && schemaAlias != "" {
			claims["tenant_id"] = tenantID.String
			claims["schema_alias"] = schemaAlias
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(getJWTSecret())
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"token":    signed,
		"username": req.Username,
		"role":     role,
	}
	if schemaAlias != "" {
		response["schema_alias"] = schemaAlias
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// JWTSecret exposes the secret for use in middleware
func JWTSecret() []byte { return jwtSecret }

func GetUsernameFromReq(r *http.Request) string {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		ah := r.Header.Get("Authorization")
		if len(ah) > 7 && ah[:7] == "Bearer " {
			tokenStr = ah[7:]
		}
	}
	if tokenStr == "" {
		return "system"
	}
	token, _ := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return getJWTSecret(), nil
	})
	if token != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if sub, ok := claims["sub"].(string); ok {
				return sub
			}
		}
	}
	return "system"
}
