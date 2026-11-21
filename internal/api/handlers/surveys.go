package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/api/middleware"
	"openwrt-controller/internal/database"
)

// ─── Survey token rate limiter ──────────────────────────────────────────────
// In-memory sliding window per survey. 10 samples / second / survey. Anything
// over that is treated as a runaway client / abuse / token leak. The window
// keeps only the last 1 second of timestamps so memory is bounded.

type tokenBucket struct {
	mu     sync.Mutex
	events []time.Time
}

var (
	bucketsMu sync.Mutex
	buckets   = make(map[string]*tokenBucket)
)

const (
	surveyRateLimitPerSecond = 10
)

func surveyAllow(surveyID string) bool {
	now := time.Now()
	bucketsMu.Lock()
	b, ok := buckets[surveyID]
	if !ok {
		b = &tokenBucket{}
		buckets[surveyID] = b
	}
	bucketsMu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	// Drop events older than 1 second.
	cutoff := now.Add(-time.Second)
	i := 0
	for ; i < len(b.events); i++ {
		if b.events[i].After(cutoff) {
			break
		}
	}
	b.events = b.events[i:]
	if len(b.events) >= surveyRateLimitPerSecond {
		return false
	}
	b.events = append(b.events, now)
	return true
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func surveyClaims(r *http.Request) (username string, role string) {
	if c, ok := middleware.GetClaims(r); ok {
		if u, ok := c["username"].(string); ok {
			username = u
		}
		if rr, ok := c["role"].(string); ok {
			role = rr
		}
	}
	return
}

// roleAtLeast returns true if the role is OPERATOR or higher (i.e. ADMIN/SUPERADMIN).
func roleAtLeast(role, min string) bool {
	rank := map[string]int{"VIEWER": 0, "OPERATOR": 1, "ADMIN": 2, "SUPERADMIN": 3}
	a, ok1 := rank[strings.ToUpper(role)]
	b, ok2 := rank[strings.ToUpper(min)]
	if !ok1 || !ok2 {
		return false
	}
	return a >= b
}

// surveyBaseURL returns the absolute base URL to embed in survey
// QR codes. The default uses r.Host (which the Go HTTP server sets
// from the Host header), with X-Forwarded-Proto honoured when the
// request came through a TLS-terminating reverse proxy.
//
// SURVEY_PUBLIC_URL overrides everything. Use it when the controller
// is reached through a proxy (e.g. Coolify) that injects scripts
// into responses — the proxy URL might be a hostname the phone
// cannot reach from outside the LAN, or it might break the SPA
// with environment shims. The override forces the QR to point at
// a known-routable address (typically https://<lan-ip>:3000).
//
// Set in .env: SURVEY_PUBLIC_URL=https://10.128.128.6:3000
func surveyBaseURL(r *http.Request) string {
	if v := os.Getenv("SURVEY_PUBLIC_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		scheme = v
	}
	host := r.Host
	return scheme + "://" + host
}

// ─── Survey CRUD ────────────────────────────────────────────────────────────

// CreateSurveyHandler — POST /api/sites/{site_id}/surveys
// Body: { name, surveyor_label, access_mode }
// Returns: full survey with token + URL (only on create).
func CreateSurveyHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}

	username, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Name          string `json:"name"`
		SurveyorMAC   string `json:"surveyor_mac"`
		SurveyorLabel string `json:"surveyor_label"`
		AccessMode    string `json:"access_mode"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	accessMode := strings.ToLower(strings.TrimSpace(req.AccessMode))
	if accessMode == "" {
		accessMode = "authenticated"
	}
	if accessMode != "authenticated" && accessMode != "public" {
		http.Error(w, `{"error":"access_mode must be 'authenticated' or 'public'"}`, http.StatusBadRequest)
		return
	}

	mac, macErr := net.ParseMAC(strings.TrimSpace(req.SurveyorMAC))
	if macErr != nil || len(mac) != 6 || mac[0]&1 != 0 {
		http.Error(w, `{"error":"Enter the phone Wi-Fi MAC for this SSID"}`, http.StatusBadRequest)
		return
	}

	// Public mode requires the site-level toggle + no global lockdown.
	if accessMode == "public" {
		allowed, err := database.GetSiteConfigAllowPublicSurveys(r.Context(), schema, siteID)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to check public-surveys toggle", err)
			return
		}
		if !allowed {
			http.Error(w, `{"error":"PUBLIC_SURVEY_DISABLED","message":"Enable 'Allow public surveys' in Site Settings to use access_mode=public"}`, http.StatusForbidden)
			return
		}
		lockdown, _ := database.IsGlobalSurveysPublicLockdown()
		if lockdown {
			http.Error(w, `{"error":"PUBLIC_SURVEY_LOCKDOWN","message":"Platform admin has disabled public surveys globally"}`, http.StatusForbidden)
			return
		}
	}

	surveyID, err := database.CreateSurvey(r.Context(), schema, siteID, req.Name, req.SurveyorLabel, mac.String(), accessMode, username)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to create survey", err)
		return
	}

	// For public surveys, generate + hash a token.
	var rawToken, surveyURL string
	if accessMode == "public" {
		t, h, err := database.GenerateSurveyToken()
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to generate token", err)
			return
		}
		if err := database.SetSurveyToken(r.Context(), schema, surveyID, h); err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to store token hash", err)
			return
		}
		rawToken = t
		surveyURL = fmt.Sprintf("%s/survey/%s?token=%s", surveyBaseURL(r), surveyID, t)
	}

	s, err := database.GetSurvey(r.Context(), schema, surveyID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load survey", err)
		return
	}
	s.SurveyToken = rawToken
	s.SurveyURL = surveyURL

	go database.InsertAuditLog(username, "SURVEY_CREATE", "SURVEY", surveyID, fmt.Sprintf("name=%s mode=%s", req.Name, accessMode), r.RemoteAddr)

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": s})
}

func ListSiteSurveysHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	out, err := database.GetSiteSurveys(r.Context(), schema, siteID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list surveys", err)
		return
	}
	if out == nil {
		out = []database.Survey{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": out})
}

func GetSurveyHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	surveyID := r.PathValue("survey_id")
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	s, err := database.GetSurvey(r.Context(), schema, surveyID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "survey not found", err)
		return
	}
	if s.SiteID != siteID {
		http.Error(w, `{"error":"survey does not belong to this site"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": s})
}

func DeleteSurveyHandler(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("site_id")
	surveyID := r.PathValue("survey_id")
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "ADMIN") {
		http.Error(w, `{"error":"FORBIDDEN: ADMIN+ required"}`, http.StatusForbidden)
		return
	}
	if err := database.DeleteSurvey(r.Context(), schema, surveyID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete survey", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_DELETE", "SURVEY", surveyID, "", r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// StartSurveyHandler — POST /api/sites/{site_id}/surveys/{survey_id}/start
// Marks a pending survey active so regular device telemetry can supply RSSI.
func StartSurveyHandler(w http.ResponseWriter, r *http.Request) {
	schema := middleware.GetTenantSchema(r)
	surveyID := r.PathValue("survey_id")
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	s, err := database.GetSurvey(r.Context(), schema, surveyID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "survey not found", err)
		return
	}
	if s.SiteID != r.PathValue("site_id") {
		http.NotFound(w, r)
		return
	}
	if s.SurveyorMAC == nil || *s.SurveyorMAC == "" {
		http.Error(w, `{"error":"Create a survey with the phone Wi-Fi MAC"}`, http.StatusConflict)
		return
	}
	if s.Status == "active" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "active", "survey_id": s.ID})
		return
	}
	if s.Status != "pending" {
		http.Error(w, "Survey already ended", http.StatusGone)
		return
	}
	if err := database.SetSurveyStatus(r.Context(), schema, surveyID, "active"); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to set status", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_START", "SURVEY", surveyID, "", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "active", "survey_id": s.ID})
}

func StopSurveyHandler(w http.ResponseWriter, r *http.Request) {
	schema := middleware.GetTenantSchema(r)
	surveyID := r.PathValue("survey_id")
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	if s, err := database.GetSurvey(r.Context(), schema, surveyID); err != nil || s.SiteID != r.PathValue("site_id") {
		http.NotFound(w, r)
		return
	}
	if err := database.SetSurveyStatus(r.Context(), schema, surveyID, "completed"); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to set status", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_STOP", "SURVEY", surveyID, "", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "completed"})
}

func RotateSurveyTokenHandler(w http.ResponseWriter, r *http.Request) {
	schema := middleware.GetTenantSchema(r)
	surveyID := r.PathValue("survey_id")
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "ADMIN") {
		http.Error(w, `{"error":"FORBIDDEN: ADMIN+ required"}`, http.StatusForbidden)
		return
	}
	s, err := database.GetSurvey(r.Context(), schema, surveyID)
	if err != nil {
		RespondError(w, http.StatusNotFound, "survey not found", err)
		return
	}
	if s.AccessMode != "public" {
		http.Error(w, `{"error":"survey is not public; no token to rotate"}`, http.StatusBadRequest)
		return
	}
	t, h, err := database.GenerateSurveyToken()
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to generate token", err)
		return
	}
	if err := database.SetSurveyToken(r.Context(), schema, surveyID, h); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to store token hash", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_TOKEN_ROTATE", "SURVEY", surveyID, "", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"survey_token": t,
		"survey_url":   fmt.Sprintf("%s/survey/%s?token=%s", surveyBaseURL(r), surveyID, t),
	})
}

func RevokeSurveyTokenHandler(w http.ResponseWriter, r *http.Request) {
	schema := middleware.GetTenantSchema(r)
	surveyID := r.PathValue("survey_id")
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "ADMIN") {
		http.Error(w, `{"error":"FORBIDDEN: ADMIN+ required"}`, http.StatusForbidden)
		return
	}
	if _, err := database.GetSurvey(r.Context(), schema, surveyID); err != nil {
		RespondError(w, http.StatusNotFound, "survey not found", err)
		return
	}
	if err := database.RevokeSurveyToken(r.Context(), schema, surveyID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to revoke token", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_TOKEN_REVOKE", "SURVEY", surveyID, "", r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func GetSurveyPointsHandler(w http.ResponseWriter, r *http.Request) {
	schema := middleware.GetTenantSchema(r)
	surveyID := r.PathValue("survey_id")
	if _, err := database.GetSurvey(r.Context(), schema, surveyID); err != nil {
		RespondError(w, http.StatusNotFound, "survey not found", err)
		return
	}
	limit := 5000
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	pts, err := database.GetSurveyPoints(r.Context(), schema, surveyID, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load points", err)
		return
	}
	if pts == nil {
		pts = []database.SurveyPoint{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": pts})
}

// ─── PUBLIC sample ingest & helpers ─────────────────────────────────────────

func authenticatePublicSurvey(w http.ResponseWriter, r *http.Request) (string, *database.Survey, bool) {
	surveyID := r.PathValue("id")
	if surveyID == "" {
		http.Error(w, `{"error":"survey id required"}`, http.StatusBadRequest)
		return "", nil, false
	}
	providedToken := r.Header.Get("X-Survey-Token")
	if providedToken == "" {
		http.Error(w, `{"error":"UNAUTHORIZED: missing X-Survey-Token"}`, http.StatusUnauthorized)
		return "", nil, false
	}

	schema, survey, ok := findSurveyAcrossTenants(surveyID)
	if !ok {
		http.Error(w, `{"error":"SURVEY_NOT_FOUND"}`, http.StatusNotFound)
		return "", nil, false
	}
	if survey.AccessMode != "public" {
		http.Error(w, `{"error":"UNAUTHORIZED: survey is not public"}`, http.StatusUnauthorized)
		return "", nil, false
	}
	if survey.SurveyTokenHash == nil || *survey.SurveyTokenHash == "" {
		http.Error(w, `{"error":"UNAUTHORIZED: no token issued for this survey"}`, http.StatusUnauthorized)
		return "", nil, false
	}
	if subtle.ConstantTimeCompare([]byte(database.HashToken(providedToken)), []byte(*survey.SurveyTokenHash)) != 1 {
		http.Error(w, `{"error":"UNAUTHORIZED: invalid token"}`, http.StatusUnauthorized)
		return "", nil, false
	}
	if survey.TokenRevokedAt != nil && *survey.TokenRevokedAt != "" {
		http.Error(w, `{"error":"TOKEN_REVOKED"}`, http.StatusUnauthorized)
		return "", nil, false
	}
	allowed, _ := database.GetSiteConfigAllowPublicSurveys(r.Context(), schema, survey.SiteID)
	if !allowed {
		http.Error(w, `{"error":"PUBLIC_SURVEY_DISABLED"}`, http.StatusForbidden)
		return "", nil, false
	}
	lockdown, _ := database.IsGlobalSurveysPublicLockdown()
	if lockdown {
		http.Error(w, `{"error":"PUBLIC_SURVEY_LOCKDOWN"}`, http.StatusForbidden)
		return "", nil, false
	}
	return schema, survey, true
}

// PostSurveySampleHandler — POST /api/surveys/{id}/samples
// Auth: X-Survey-Token. Body: { lat, lon, accuracy_m, ts, client_sample_id?, probe_rtt_ms?, probe_jitter_ms?, probe_loss_pct? }.
func PostSurveySampleHandler(w http.ResponseWriter, r *http.Request) {
	surveyID := r.PathValue("id")
	if surveyID == "" {
		http.Error(w, `{"error":"survey id required"}`, http.StatusBadRequest)
		return
	}
	if !surveyAllow(surveyID) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"RATE_LIMITED"}`, http.StatusTooManyRequests)
		return
	}
	schema, survey, ok := authenticatePublicSurvey(w, r)
	if !ok {
		return
	}
	saveSurveySample(w, r, schema, survey)
}

func PostOperatorSurveySampleHandler(w http.ResponseWriter, r *http.Request) {
	_, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, "Operator access required", http.StatusForbidden)
		return
	}
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, "Tenant required", http.StatusForbidden)
		return
	}
	survey, err := database.GetSurvey(r.Context(), schema, r.PathValue("survey_id"))
	if err != nil || survey.SiteID != r.PathValue("site_id") || survey.AccessMode != "authenticated" {
		http.NotFound(w, r)
		return
	}
	if !surveyAllow(survey.ID) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Too many samples", http.StatusTooManyRequests)
		return
	}
	saveSurveySample(w, r, schema, survey)
}

func saveSurveySample(w http.ResponseWriter, r *http.Request, schema string, survey *database.Survey) {
	surveyID := survey.ID
	if survey.Status == "completed" || survey.Status == "aborted" {
		http.Error(w, `{"error":"SURVEY_ENDED"}`, http.StatusGone)
		return
	}

	if survey.Status != "active" || survey.SurveyorMAC == nil || *survey.SurveyorMAC == "" {
		http.Error(w, `{"error":"Start a survey configured with the phone Wi-Fi MAC"}`, http.StatusConflict)
		return
	}

	var req struct {
		Lat            *float64 `json:"lat"`
		Lon            *float64 `json:"lon"`
		AccuracyM      float32  `json:"accuracy_m"`
		Timestamp      int64    `json:"ts"`
		ClientSampleID string   `json:"client_sample_id"`
		ProbeRTTMs     *float32 `json:"probe_rtt_ms"`
		ProbeJitterMs  *float32 `json:"probe_jitter_ms"`
		ProbeLossPct   *float32 `json:"probe_loss_pct"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Lat == nil || req.Lon == nil || *req.Lat < -90 || *req.Lat > 90 ||
		*req.Lon < -180 || *req.Lon > 180 || req.AccuracyM < 0 {
		http.Error(w, `{"error":"Invalid GPS coordinates or accuracy"}`, http.StatusBadRequest)
		return
	}
	if req.ProbeRTTMs != nil && (*req.ProbeRTTMs < 0 || *req.ProbeRTTMs > 60000) {
		http.Error(w, `{"error":"Invalid probe_rtt_ms"}`, http.StatusBadRequest)
		return
	}
	if req.ProbeJitterMs != nil && (*req.ProbeJitterMs < 0 || *req.ProbeJitterMs > 60000) {
		http.Error(w, `{"error":"Invalid probe_jitter_ms"}`, http.StatusBadRequest)
		return
	}
	if req.ProbeLossPct != nil && (*req.ProbeLossPct < 0 || *req.ProbeLossPct > 100) {
		http.Error(w, `{"error":"Invalid probe_loss_pct"}`, http.StatusBadRequest)
		return
	}
	if req.Timestamp <= 0 {
		http.Error(w, `{"error":"GPS timestamp required"}`, http.StatusBadRequest)
		return
	}
	ts := time.UnixMilli(req.Timestamp)

	if ts.After(time.Now().Add(5 * time.Second)) {
		http.Error(w, `{"error":"Phone timestamp is in the future"}`, http.StatusBadRequest)
		return
	}
	// Let telemetry arrive on both sides of the GPS timestamp.
	if time.Since(ts) < 15*time.Second {
		http.Error(w, `{"error":"WAITING_FOR_TELEMETRY"}`, http.StatusTooEarly)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	signal, matched, err := database.GetClosestSurveySignal(ctx, *survey.SurveyorMAC, surveyID, ts)
	if err != nil {
		RespondError(w, http.StatusServiceUnavailable, "signal lookup failed", err)
		return
	}
	point := database.SurveyPoint{
		SurveyID:      surveyID,
		Lat:           req.Lat,
		Lon:           req.Lon,
		AccuracyM:     &req.AccuracyM,
		CapturedAt:    ts.UTC().Format(time.RFC3339Nano),
		NeighborAPs:   "[]",
		SilentAPs:     "[]",
		ProbeRTTMs:    req.ProbeRTTMs,
		ProbeJitterMs: req.ProbeJitterMs,
		ProbeLossPct:  req.ProbeLossPct,
	}
	if cid := strings.TrimSpace(req.ClientSampleID); cid != "" {
		if len(cid) > 64 {
			cid = cid[:64]
		}
		point.ClientSampleID = &cid
	}
	if matched {
		point.APID = signal.DeviceID
		dbm := float32(signal.SignalDBM)
		point.SignalDBM = &dbm
		if signal.NoiseDBM != 0 {
			noise := float32(signal.NoiseDBM)
			snr := dbm - noise
			point.NoiseDBM, point.SNR = &noise, &snr
		}
	} else {
		reason, reportingCount, totalCount, silentJSON := diagnoseMissingSignal(ctx, schema, survey.SiteID, ts)
		point.MissingReason = reason
		if totalCount > 0 {
			point.ReportingAPsCount = &reportingCount
			point.TotalAPsCount = &totalCount
		}
		point.SilentAPs = silentJSON
	}
	inserted, err := database.InsertSurveyPoint(ctx, schema, point)
	if err != nil {
		RespondError(w, http.StatusServiceUnavailable, "failed to save survey point", err)
		return
	}
	if survey.AccessMode == "public" && survey.TokenFirstUsedAt == nil {
		ip, ua := clientFingerprint(r)
		_ = database.SetSurveyTokenFirstUse(r.Context(), schema, surveyID, ip, ua)
	}
	statusCode := http.StatusCreated
	if !inserted {
		statusCode = http.StatusOK
	}
	resp := map[string]interface{}{
		"matched":        matched,
		"duplicate":      !inserted,
		"missing_reason": point.MissingReason,
	}
	if point.ReportingAPsCount != nil {
		resp["reporting_aps"] = *point.ReportingAPsCount
	}
	if point.TotalAPsCount != nil {
		resp["total_aps"] = *point.TotalAPsCount
	}
	writeJSON(w, statusCode, resp)
}

// diagnoseMissingSignal checks which site APs reported device_metrics around ts
// so gray points distinguish a client Wi-Fi drop from silent/offline AP telemetry.
func diagnoseMissingSignal(ctx context.Context, schema, siteID string, ts time.Time) (reason string, reportingCount, totalCount int, silentJSON string) {
	silentJSON = "[]"
	devices, err := database.GetSiteDeviceRefs(ctx, schema, siteID)
	if err != nil || len(devices) == 0 {
		return "no_ap_telemetry", 0, 0, silentJSON
	}
	totalCount = len(devices)
	reportingSet, err := database.GetReportingDevicesAround(ctx, ts, 30*time.Second)
	if err != nil {
		return "no_ap_telemetry", 0, totalCount, silentJSON
	}
	silentLabels := make([]string, 0)
	for _, dev := range devices {
		if reportingSet[strings.ToLower(dev.ID)] {
			reportingCount++
		} else {
			silentLabels = append(silentLabels, dev.DisplayLabel())
		}
	}
	if b, err := json.Marshal(silentLabels); err == nil {
		silentJSON = string(b)
	}
	switch {
	case reportingCount == 0:
		reason = "no_ap_telemetry"
	case reportingCount < totalCount:
		reason = "partial_ap_telemetry"
	default:
		reason = "client_not_associated"
	}
	return reason, reportingCount, totalCount, silentJSON
}

// ─── Live AP Telemetry Health & Streaming Probe Ping ────────────────────────

func writeSurveyStatus(w http.ResponseWriter, r *http.Request, schema string, survey *database.Survey) {
	w.Header().Set("Cache-Control", "no-store")
	surveyorMAC := ""
	if survey.SurveyorMAC != nil {
		surveyorMAC = *survey.SurveyorMAC
	}
	aps, err := database.GetSiteAPTelemetryHealth(r.Context(), schema, survey.SiteID, surveyorMAC)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load AP telemetry status", err)
		return
	}
	if aps == nil {
		aps = []database.SiteAPTelemetry{}
	}
	reportingCount := 0
	surveyorSeen := false
	surveyorAP := ""
	var surveyorDBM *float64
	for _, ap := range aps {
		if ap.Reporting {
			reportingCount++
		}
		if ap.SeesSurveyor {
			surveyorSeen = true
			surveyorAP = ap.Label
			surveyorDBM = ap.SurveyorDBM
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"survey_id":       survey.ID,
		"status":          survey.Status,
		"reporting_count": reportingCount,
		"total_count":     len(aps),
		"surveyor_seen":   surveyorSeen,
		"surveyor_ap":     surveyorAP,
		"surveyor_dbm":    surveyorDBM,
		"aps":             aps,
	})
}

func GetSurveyStatusHandler(w http.ResponseWriter, r *http.Request) {
	_, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	survey, err := database.GetSurvey(r.Context(), schema, r.PathValue("survey_id"))
	if err != nil || survey.SiteID != r.PathValue("site_id") {
		http.NotFound(w, r)
		return
	}
	writeSurveyStatus(w, r, schema, survey)
}

func GetPublicSurveyStatusHandler(w http.ResponseWriter, r *http.Request) {
	schema, survey, ok := authenticatePublicSurvey(w, r)
	if !ok {
		return
	}
	writeSurveyStatus(w, r, schema, survey)
}

func SurveyPingHandler(w http.ResponseWriter, r *http.Request) {
	_, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"server_ts": time.Now().UnixMilli(),
	})
}

func PublicSurveyPingHandler(w http.ResponseWriter, r *http.Request) {
	_, _, ok := authenticatePublicSurvey(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"server_ts": time.Now().UnixMilli(),
	})
}

// ─── Local Offline Floorplan ────────────────────────────────────────────────

const maxFloorplanBodyBytes = 3 << 20 // 3 MiB

func validateSurveyFloorplan(fp database.SurveyFloorplan) error {
	if fp.ImageDataURL == "" {
		return nil // clearing floorplan
	}
	validPrefix := strings.HasPrefix(fp.ImageDataURL, "data:image/png;base64,") ||
		strings.HasPrefix(fp.ImageDataURL, "data:image/jpeg;base64,") ||
		strings.HasPrefix(fp.ImageDataURL, "data:image/webp;base64,")
	if !validPrefix {
		return fmt.Errorf("floorplan must be a PNG, JPEG, or WebP data URL")
	}
	swLat, swLon := fp.Bounds[0][0], fp.Bounds[0][1]
	neLat, neLon := fp.Bounds[1][0], fp.Bounds[1][1]
	if swLat < -90 || swLat > 90 || neLat < -90 || neLat > 90 ||
		swLon < -180 || swLon > 180 || neLon < -180 || neLon > 180 {
		return fmt.Errorf("invalid floorplan geographic bounds")
	}
	if swLat >= neLat || swLon >= neLon {
		return fmt.Errorf("south-west bound must be strictly less than north-east bound")
	}
	if fp.Opacity < 0.05 || fp.Opacity > 1.0 {
		return fmt.Errorf("opacity must be between 0.05 and 1.0")
	}
	return nil
}

func GetSurveyFloorplanHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	fp, err := database.GetSiteSurveyFloorplan(r.Context(), schema, siteID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load floorplan", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": fp})
}

func SaveSurveyFloorplanHandler(w http.ResponseWriter, r *http.Request) {
	siteID := r.PathValue("site_id")
	schema := middleware.GetTenantSchema(r)
	if schema == "" {
		http.Error(w, `{"error":"tenant schema unresolved"}`, http.StatusInternalServerError)
		return
	}
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "OPERATOR") {
		http.Error(w, `{"error":"FORBIDDEN: OPERATOR+ required"}`, http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFloorplanBodyBytes)
	var fp database.SurveyFloorplan
	if err := json.NewDecoder(r.Body).Decode(&fp); err != nil {
		http.Error(w, `{"error":"invalid floorplan payload or image exceeds 3 MB limit"}`, http.StatusBadRequest)
		return
	}
	if fp.ImageDataURL != "" && fp.Opacity == 0 {
		fp.Opacity = 0.75
	}
	if err := validateSurveyFloorplan(fp); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	var toSave *database.SurveyFloorplan
	if fp.ImageDataURL != "" {
		fp.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		toSave = &fp
	}
	if err := database.SaveSiteSurveyFloorplan(r.Context(), schema, siteID, toSave); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to save floorplan", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_FLOORPLAN_UPDATE", "SITE", siteID, fp.Filename, r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": toSave})
}

func clientFingerprint(r *http.Request) (ip, ua string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		ip = host
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		ip = strings.SplitN(v, ",", 2)[0]
		ip = strings.TrimSpace(ip)
	}
	ua = r.Header.Get("User-Agent")
	return
}

// findSurveyAcrossTenants scans active tenants for a survey ID. Returns the
// tenant schema, the loaded Survey, and ok=true on hit.
func findSurveyAcrossTenants(surveyID string) (string, *database.Survey, bool) {
	ctx := context.Background()
	// This re-uses a worker cache to avoid hammering the DB on every
	// sample. Cache key = surveyID -> (schema, lastSeen).
	cacheMu.Lock()
	if e, ok := surveyLookupCache[surveyID]; ok && time.Since(e.at) < 5*time.Minute {
		s, err := database.GetSurvey(ctx, e.schema, surveyID)
		cacheMu.Unlock()
		if err == nil {
			return e.schema, s, true
		}
		// stale; fall through to rescan
	} else {
		cacheMu.Unlock()
	}

	rows, err := database.DB.Query(`SELECT schema_alias FROM tenants WHERE is_active = true`)
	if err != nil {
		log.Printf("[SURVEY_LOOKUP] tenants query failed: %v", err)
		return "", nil, false
	}
	defer rows.Close()
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			continue
		}
		schema := "tenant_" + alias
		s, err := database.GetSurvey(ctx, schema, surveyID)
		if err == nil {
			cacheMu.Lock()
			surveyLookupCache[surveyID] = lookupEntry{schema: schema, at: time.Now()}
			cacheMu.Unlock()
			return schema, s, true
		}
	}
	return "", nil, false
}

type lookupEntry struct {
	schema string
	at     time.Time
}

var (
	cacheMu           sync.Mutex
	surveyLookupCache = make(map[string]lookupEntry)
)

// ─── Platform settings: global lockdown ─────────────────────────────────────

func SetGlobalSurveyLockdownHandler(w http.ResponseWriter, r *http.Request) {
	username, role := surveyClaims(r)
	if !roleAtLeast(role, "SUPERADMIN") {
		http.Error(w, `{"error":"FORBIDDEN: SUPERADMIN required"}`, http.StatusForbidden)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := database.SetGlobalSurveysPublicLockdown(req.Enabled); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to update lockdown", err)
		return
	}
	go database.InsertAuditLog(username, "SURVEY_GLOBAL_LOCKDOWN", "PLATFORM", "1", strconv.FormatBool(req.Enabled), r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]interface{}{"global_surveys_public_lockdown": req.Enabled})
}

// ─── Misc ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
