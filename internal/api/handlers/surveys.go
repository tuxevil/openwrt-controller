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
	"openwrt-controller/internal/services"
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

	surveyID, err := database.CreateSurvey(r.Context(), schema, siteID, req.Name, req.SurveyorLabel, accessMode, username)
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
	services.UnregisterSchema(surveyID)
	go database.InsertAuditLog(username, "SURVEY_DELETE", "SURVEY", surveyID, "", r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

// StartSurveyHandler — POST /api/sites/{site_id}/surveys/{survey_id}/start
// Marks the survey active; the next agent pull of /api/devices/{id}/config
// will see the survey_mode flag and ramp up to 2 s telemetry.
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
	if err := database.SetSurveyStatus(r.Context(), schema, surveyID, "active"); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to set status", err)
		return
	}
	services.RegisterSchema(surveyID, schema)
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
	if _, err := database.GetSurvey(r.Context(), schema, surveyID); err != nil {
		RespondError(w, http.StatusNotFound, "survey not found", err)
		return
	}
	if err := database.SetSurveyStatus(r.Context(), schema, surveyID, "completed"); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to set status", err)
		return
	}
	services.UnregisterSchema(surveyID)
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

// ─── PUBLIC sample ingest ───────────────────────────────────────────────────

// PostSurveySampleHandler — POST /api/surveys/{id}/samples
// Auth: X-Survey-Token. Body: { lat, lon, accuracy_m, ts }.
// Validates:
//   - token not empty, not revoked, not expired (if ended_at + 1h passed)
//   - site allow_public_surveys + not globally locked down
//   - access_mode == "public"
//   - rate-limit 10 req/s
// Records first_seen IP+UA, forwards to survey worker for correlation.
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

	providedToken := r.Header.Get("X-Survey-Token")
	if providedToken == "" {
		http.Error(w, `{"error":"UNAUTHORIZED: missing X-Survey-Token"}`, http.StatusUnauthorized)
		return
	}

	// Survey IDs are UUIDs that span tenants. We need to locate the survey
	// across all active tenant schemas. This is O(N) tenants but only fires
	// per-sample, and N is small (<100 for typical MSP).
	schema, survey, ok := findSurveyAcrossTenants(surveyID)
	if !ok {
		http.Error(w, `{"error":"SURVEY_NOT_FOUND"}`, http.StatusNotFound)
		return
	}
	if survey.AccessMode != "public" {
		http.Error(w, `{"error":"UNAUTHORIZED: survey is not public"}`, http.StatusUnauthorized)
		return
	}

	// Token: constant-time compare against stored hash.
	if survey.SurveyTokenHash == nil || *survey.SurveyTokenHash == "" {
		http.Error(w, `{"error":"UNAUTHORIZED: no token issued for this survey"}`, http.StatusUnauthorized)
		return
	}
	if subtle.ConstantTimeCompare([]byte(database.HashToken(providedToken)), []byte(*survey.SurveyTokenHash)) != 1 {
		http.Error(w, `{"error":"UNAUTHORIZED: invalid token"}`, http.StatusUnauthorized)
		return
	}
	if survey.TokenRevokedAt != nil && *survey.TokenRevokedAt != "" {
		http.Error(w, `{"error":"TOKEN_REVOKED"}`, http.StatusUnauthorized)
		return
	}
	if survey.Status == "completed" || survey.Status == "aborted" {
		http.Error(w, `{"error":"SURVEY_ENDED"}`, http.StatusGone)
		return
	}

	// Re-check site + platform toggles on every request (they can change
	// while the survey is live). Cheap because they're indexed lookups.
	allowed, _ := database.GetSiteConfigAllowPublicSurveys(r.Context(), schema, survey.SiteID)
	if !allowed {
		http.Error(w, `{"error":"PUBLIC_SURVEY_DISABLED"}`, http.StatusForbidden)
		return
	}
	lockdown, _ := database.IsGlobalSurveysPublicLockdown()
	if lockdown {
		http.Error(w, `{"error":"PUBLIC_SURVEY_LOCKDOWN"}`, http.StatusForbidden)
		return
	}

	// Parse body.
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Lat        float64 `json:"lat"`
		Lon        float64 `json:"lon"`
		AccuracyM  float32 `json:"accuracy_m"`
		Timestamp  int64   `json:"ts"` // ms since epoch (optional, defaults to now)
	}
	if len(body) == 0 {
		http.Error(w, `{"error":"empty body"}`, http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Lat < -90 || req.Lat > 90 || req.Lon < -180 || req.Lon > 180 {
		http.Error(w, `{"error":"lat/lon out of range"}`, http.StatusBadRequest)
		return
	}
	ts := time.Now()
	if req.Timestamp > 0 {
		ts = time.UnixMilli(req.Timestamp)
	}

	// Record first_seen fingerprint (best effort, do not block on error).
	ip, ua := clientFingerprint(r)
	_ = database.SetSurveyTokenFirstUse(r.Context(), schema, surveyID, ip, ua)

	// Register the schema mapping for the worker (idempotent).
	services.RegisterSchema(surveyID, schema)

	// Enqueue for correlation. AP ID is resolved by the worker when it
	// pulls the most recent client_signal from InfluxDB.
	services.GetSurveyWorker().EnqueueGPS(services.GPSSampleForWorker(surveyID, "", req.Lat, req.Lon, req.AccuracyM, ts))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
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
