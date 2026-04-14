package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"openwrt-controller/internal/database"
)

type GuestVoucher struct {
	ID              string `json:"id"`
	SiteID          string `json:"site_id"`
	Code            string `json:"code"`
	DurationMinutes int    `json:"duration_minutes"`
	QuotaMB         int    `json:"quota_mb"`
	IsUsed          bool   `json:"is_used"`
	UsedByMAC       string `json:"used_by_mac"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
	UsedAt          string `json:"used_at"`
}

type PortalSettings struct {
	SiteID      string `json:"site_id"`
	Enabled     bool   `json:"enabled"`
	WelcomeText string `json:"welcome_text"`
	TermsText   string `json:"terms_text"`
	BgColor     string `json:"bg_color"`
	LogoURL     string `json:"logo_url"`
	RedirectURL string `json:"redirect_url"`
}

// ─── VOUCHERS ─────────────────────────────────────────────────────────────

func generateCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GenerateVoucherBatch(siteID string, count, duration, quota int) ([]GuestVoucher, error) {
	var vouchers []GuestVoucher

	for i := 0; i < count; i++ {
		code := generateCode()
		_, err := database.DB.Exec(`
			INSERT INTO guest_vouchers (site_id, code, duration_minutes, quota_mb)
			VALUES ($1, $2, $3, $4)
		`, siteID, code, duration, quota)
		if err != nil {
			return nil, err
		}
		vouchers = append(vouchers, GuestVoucher{
			SiteID:          siteID,
			Code:            code,
			DurationMinutes: duration,
			QuotaMB:         quota,
		})
	}
	return vouchers, nil
}

func GetVouchers(siteID string) ([]GuestVoucher, error) {
	rows, err := database.DB.Query(`
		SELECT id, site_id, code, duration_minutes, COALESCE(quota_mb, 0), is_used, COALESCE(used_by_mac, ''), created_at, COALESCE(expires_at::text, ''), COALESCE(used_at::text, '')
		FROM guest_vouchers WHERE site_id = $1 ORDER BY created_at DESC
	`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []GuestVoucher
	for rows.Next() {
		var v GuestVoucher
		rows.Scan(&v.ID, &v.SiteID, &v.Code, &v.DurationMinutes, &v.QuotaMB, &v.IsUsed, &v.UsedByMAC, &v.CreatedAt, &v.ExpiresAt, &v.UsedAt)
		res = append(res, v)
	}
	return res, nil
}

func ValidateVoucher(siteID, code string) (*GuestVoucher, error) {
	var v GuestVoucher
	var expiresAt sql.NullTime
	var usedAt sql.NullTime
	err := database.DB.QueryRow(`
		SELECT id, site_id, code, duration_minutes, COALESCE(quota_mb, 0), is_used, COALESCE(used_by_mac, ''), created_at, expires_at, used_at
		FROM guest_vouchers WHERE site_id = $1 AND code = $2
	`, siteID, code).Scan(&v.ID, &v.SiteID, &v.Code, &v.DurationMinutes, &v.QuotaMB, &v.IsUsed, &v.UsedByMAC, &v.CreatedAt, &expiresAt, &usedAt)

	if err != nil {
		return nil, fmt.Errorf("voucher not found")
	}

	if expiresAt.Valid {
		v.ExpiresAt = expiresAt.Time.String()
		if time.Now().After(expiresAt.Time) {
			return nil, fmt.Errorf("voucher expired")
		}
	}

	if usedAt.Valid {
		v.UsedAt = usedAt.Time.String()
	}

	return &v, nil
}

func MarkVoucherUsed(code, mac string) error {
	var duration int
	err := database.DB.QueryRow("SELECT duration_minutes FROM guest_vouchers WHERE code = $1", code).Scan(&duration)
	if err != nil {
		return err
	}

	expires := time.Now().Add(time.Duration(duration) * time.Minute)

	_, err = database.DB.Exec(`
		UPDATE guest_vouchers 
		SET is_used = true, used_by_mac = $1, used_at = CURRENT_TIMESTAMP, expires_at = $2
		WHERE code = $3 AND is_used = false
	`, mac, expires, code)
	return err
}

func DeleteVoucher(id string) error {
	_, err := database.DB.Exec(`DELETE FROM guest_vouchers WHERE id = $1`, id)
	return err
}

// ─── PORTAL SETTINGS ─────────────────────────────────────────────────────────────

func GetPortalSettings(siteID string) (*PortalSettings, error) {
	var s PortalSettings
	err := database.DB.QueryRow(`
		SELECT site_id, enabled, COALESCE(welcome_text, ''), COALESCE(terms_text, ''), COALESCE(bg_color, '#0a0a0a'), COALESCE(logo_url, ''), COALESCE(redirect_url, '')
		FROM portal_settings WHERE site_id = $1
	`, siteID).Scan(&s.SiteID, &s.Enabled, &s.WelcomeText, &s.TermsText, &s.BgColor, &s.LogoURL, &s.RedirectURL)

	if err != nil {
		if err == sql.ErrNoRows {
			return &PortalSettings{SiteID: siteID, Enabled: false, WelcomeText: "Welcome to Guest Wi-Fi", TermsText: "By connecting, you agree to our terms.", BgColor: "#0a0a0a"}, nil
		}
		return nil, err
	}
	return &s, nil
}

func UpsertPortalSettings(s PortalSettings) error {
	_, err := database.DB.Exec(`
		INSERT INTO portal_settings (site_id, enabled, welcome_text, terms_text, bg_color, logo_url, redirect_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (site_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			welcome_text = EXCLUDED.welcome_text,
			terms_text = EXCLUDED.terms_text,
			bg_color = EXCLUDED.bg_color,
			logo_url = EXCLUDED.logo_url,
			redirect_url = EXCLUDED.redirect_url,
			updated_at = CURRENT_TIMESTAMP
	`, s.SiteID, s.Enabled, s.WelcomeText, s.TermsText, s.BgColor, s.LogoURL, s.RedirectURL)
	return err
}
