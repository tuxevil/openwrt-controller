package handlers

import (
	"encoding/json"
	"net/http"
	"openwrt-controller/internal/database"
)

type RadiusUser struct {
	ID       int    `json:"id,omitempty"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	VLAN     string `json:"vlan,omitempty"`
	SiteID   string `json:"site_id"`
}

func GetRadiusUsersHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Context().Value("tenant_id").(string)

	rows, err := database.DB.Query(`
		SELECT c.id, c.username, c.value, r.value, c.site_id
		FROM public.radcheck c
		LEFT JOIN public.radreply r ON c.username = r.username AND c.site_id = r.site_id AND r.attribute = 'Tunnel-Private-Group-Id'
		WHERE c.tenant_id = $1 AND c.attribute = 'Cleartext-Password'
	`, tenantID)
	
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []RadiusUser
	for rows.Next() {
		var u RadiusUser
		var vlan *string
		if err := rows.Scan(&u.ID, &u.Username, &u.Password, &vlan, &u.SiteID); err == nil {
			if vlan != nil {
				u.VLAN = *vlan
			}
			users = append(users, u)
		}
	}
	
	if users == nil {
		users = make([]RadiusUser, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func CreateRadiusUserHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Context().Value("tenant_id").(string)
	
	var req RadiusUser
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	tx, err := database.DB.Begin()
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO public.radcheck (username, attribute, op, value, site_id, tenant_id)
		VALUES ($1, 'Cleartext-Password', ':=', $2, $3, $4)
	`, req.Username, req.Password, req.SiteID, tenantID)

	if req.VLAN != "" && req.VLAN != "0" {
		tx.Exec(`INSERT INTO public.radreply (username, attribute, op, value, site_id, tenant_id) VALUES ($1, 'Tunnel-Type', '=', 'VLAN', $2, $3)`, req.Username, req.SiteID, tenantID)
		tx.Exec(`INSERT INTO public.radreply (username, attribute, op, value, site_id, tenant_id) VALUES ($1, 'Tunnel-Medium-Type', '=', 'IEEE-802', $2, $3)`, req.Username, req.SiteID, tenantID)
		tx.Exec(`INSERT INTO public.radreply (username, attribute, op, value, site_id, tenant_id) VALUES ($1, 'Tunnel-Private-Group-Id', '=', $2, $3, $4)`, req.Username, req.VLAN, req.SiteID, tenantID)
	}

	tx.Commit()
	w.WriteHeader(http.StatusCreated)
}

func DeleteRadiusUserHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Context().Value("tenant_id").(string)
	username := r.URL.Query().Get("username")
	siteID := r.URL.Query().Get("site_id")

	database.DB.Exec("DELETE FROM public.radcheck WHERE username = $1 AND site_id = $2 AND tenant_id = $3", username, siteID, tenantID)
	database.DB.Exec("DELETE FROM public.radreply WHERE username = $1 AND site_id = $2 AND tenant_id = $3", username, siteID, tenantID)
	w.WriteHeader(http.StatusOK)
}
