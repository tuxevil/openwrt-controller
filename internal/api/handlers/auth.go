package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"openwrt-controller/internal/database"
)

var jwtSecret = []byte(getJWTSecret())

func getJWTSecret() string {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s
	}
	return "REPLACE_WITH_JWT_SECRET"
}

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

	// Fetch user from DB
	var storedHash, role string
	err := database.DB.QueryRow(
		"SELECT password_hash, role FROM users WHERE username = $1",
		req.Username,
	).Scan(&storedHash, &role)

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

	// Issue JWT
	claims := jwt.MapClaims{
		"sub":  req.Username,
		"role": role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":    signed,
		"username": req.Username,
		"role":     role,
	})
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
		return jwtSecret, nil
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
