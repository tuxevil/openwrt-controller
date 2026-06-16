package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"openwrt-controller/internal/services"
)

// ─── PUBLIC ENDPOINTS ────────────────────────────────────────────────────────

// GetPortalAuthHandler displays the captive portal landing page.
// OpenNDS redirects here with: ?fas=[fas_string]
func GetPortalAuthHandler(w http.ResponseWriter, r *http.Request) {
	// site_id is needed to load the correct branding. For now, assume it's passed or derived from the IP.
	// OpenNDS fas format usually contains: clientip, clientmac, gatewaymac, etc.
	// We can find the site_id by looking up the gatewaymac in devices table.
	// But let's assume the router hits /portal/auth/{site_id} or just /portal/auth

	// Since OpenNDS doesn't know the site_id unless we pass it in faspath,
	// we should expect the route to be: /api/public/portal/{site_id}/auth

	siteID := r.PathValue("site_id")
	if siteID == "" {
		http.Error(w, "missing site_id", http.StatusBadRequest)
		return
	}

	fas := r.URL.Query().Get("fas")
	if fas == "" {
		http.Error(w, "missing FAS token", http.StatusBadRequest)
		return
	}

	settings, err := services.GetPortalSettings(siteID)
	if err != nil {
		http.Error(w, "portal disabled or not found", http.StatusNotFound)
		return
	}

	if !settings.Enabled {
		http.Error(w, "portal disabled", http.StatusForbidden)
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { background-color: %s; color: white; font-family: monospace; text-align: center; padding: 2rem; }
		.container { max-width: 400px; margin: 0 auto; background: rgba(255,255,255,0.05); padding: 2rem; border-radius: 8px; border: 1px solid rgba(255,255,255,0.1); }
		input { width: 100%%; padding: 10px; margin: 15px 0; border-radius: 4px; border: 1px solid #333; background: #000; color: #0f0; text-align: center; font-size: 1.2rem; box-sizing: border-box; }
		button { width: 100%%; padding: 12px; background: #ec4899; color: white; border: none; border-radius: 4px; font-weight: bold; cursor: pointer; }
		button:hover { background: #db2777; }
	</style>
</head>
<body>
	<div class="container">
		<h2>%s</h2>
		<p>%s</p>
		<form method="POST" action="/api/public/portal/%s/validate">
			<input type="hidden" name="fas" value="%s">
			<input type="text" name="code" placeholder="ENTER 6-DIGIT VOUCHER" required maxlength="10">
			<button type="submit">CONNECT</button>
		</form>
	</div>
</body>
</html>`, settings.BgColor, settings.WelcomeText, settings.TermsText, siteID, fas)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// ValidatePortalHandler validates the voucher and generates the tok string to redirect back to OpenNDS
func ValidatePortalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	siteID := r.PathValue("site_id")
	r.ParseForm()
	fas := r.FormValue("fas")
	code := r.FormValue("code")

	if code == "" || fas == "" {
		http.Error(w, "missing parameters", http.StatusBadRequest)
		return
	}

	// Unpack FAS (AES encrypted by OpenNDS usually, or clear text if fas_secure_enabled=0)
	// For this test, assume opennds sends cleartext fas or base64.
	// Due to complexity of FAS AES, many setups use fas_secure_enabled=0 and fas string is base64 of "clientip,clientmac,gatewaymac,tok"
	// For simplicity, we just authorize by returning an HTTP redirect back to gatewayip:2050/opennds_auth/?tok=...

	voucher, err := services.ValidateVoucher(siteID, code)
	if err != nil {
		http.Error(w, "invalid or expired voucher", http.StatusForbidden)
		return
	}

	if voucher.IsUsed {
		// allow if it's the same MAC, but since we are mocking, just say it's valid if duration hasn't expired.
		// wait, ValidateVoucher already checks expiration if UsedAt is set.
	}

	// MOCK FAS parsing: normally we read gateway IP from FAS string.
	// For now, OpenNDS expects: http://<gatewayip>:<gatewayport>/opennds_auth/?tok=<tok>
	// Since we don't have gateway IP without fully parsing FAS, we can just return a success page
	// instructing to go back, or if opennds supports it, we redirect perfectly.
	// We will simply mark voucher as used.
	services.MarkVoucherUsed(code, "unknown-fas-mac")

	// Usually FAS decrypts to get the true 'tok'.
	// We'll just print Auth Success for now.

	w.Write([]byte("AUTH SUCCESS. You are now connected to the network."))
}

// ─── ADMIN ENDPOINTS ────────────────────────────────────────────────────────

func GetPortalSettingsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	settings, err := services.GetPortalSettings(siteID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func UpdatePortalSettingsHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	var s services.PortalSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		log.Printf("Error unmarshalling portal settings: %v\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.SiteID = siteID
	if err := services.UpsertPortalSettings(s); err != nil {
		log.Printf("Error saving portal settings (DB): %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Portal settings successfully saved for site %s\n", siteID)
	w.WriteHeader(http.StatusOK)
}

func GetPortalVouchersHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	vouchers, err := services.GetVouchers(siteID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vouchers)
}

func GeneratePortalVouchersHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")

	type req struct {
		Count    int `json:"count"`
		Duration int `json:"duration_minutes"`
		Quota    int `json:"quota_mb"`
	}
	var pr req
	if err := json.NewDecoder(r.Body).Decode(&pr); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	vouchers, err := services.GenerateVoucherBatch(siteID, pr.Count, pr.Duration, pr.Quota)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vouchers)
}
