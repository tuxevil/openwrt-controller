package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"openwrt-controller/internal/database"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}


func GetUsersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query("SELECT id, username, role, created_at FROM users ORDER BY created_at ASC")
	if err != nil {
		http.Error(w, `{"error":"failed to fetch users"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	req.Role = strings.ToUpper(req.Role)
	if req.Role != "ADMIN" && req.Role != "OPERATOR" && req.Role != "VIEWER" {
		http.Error(w, `{"error":"invalid role"}`, http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"failed to secure password"}`, http.StatusInternalServerError)
		return
	}

	_, err = database.DB.Exec(
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3)",
		req.Username, string(hash), req.Role,
	)

	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, `{"error":"username already exists"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"success"}`))
}

func UpdateUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	req.Role = strings.ToUpper(req.Role)
	if req.Role != "ADMIN" && req.Role != "OPERATOR" && req.Role != "VIEWER" {
		http.Error(w, `{"error":"invalid role"}`, http.StatusBadRequest)
		return
	}

	// Protect against removing the last ADMIN if changing their role
	if req.Role != "ADMIN" {
		var currentRole string
		_ = database.DB.QueryRow("SELECT role FROM users WHERE id = $1", id).Scan(&currentRole)
		if strings.ToUpper(currentRole) == "ADMIN" {
			var adminCount int
			database.DB.QueryRow("SELECT Count(*) FROM users WHERE role = 'ADMIN'").Scan(&adminCount)
			if adminCount <= 1 {
				http.Error(w, `{"error":"cannot demote the last ADMIN"}`, http.StatusConflict)
				return
			}
		}
	}

	_, err := database.DB.Exec("UPDATE users SET role = $1 WHERE id = $2", req.Role, id)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"success"}`))
}

func UpdateUserPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"failed to secure password"}`, http.StatusInternalServerError)
		return
	}

	_, err = database.DB.Exec("UPDATE users SET password_hash = $1 WHERE id = $2", string(hash), id)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"success"}`))
}

func DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	// Protect against terminating the last admin
	var role string
	err := database.DB.QueryRow("SELECT role FROM users WHERE id = $1", id).Scan(&role)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	if strings.ToUpper(role) == "ADMIN" {
		var adminCount int
		database.DB.QueryRow("SELECT Count(*) FROM users WHERE role = 'ADMIN'").Scan(&adminCount)
		if adminCount <= 1 {
			http.Error(w, `{"error":"cannot delete the last ADMIN"}`, http.StatusConflict)
			return
		}
	}

	_, err = database.DB.Exec("DELETE FROM users WHERE id = $1", id)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"success"}`))
}
