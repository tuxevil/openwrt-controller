package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"openwrt-controller/internal/database"
)

const maxDeviceEnrollmentBody = 64 * 1024

var enrollmentDeviceIDPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9:._-]{0,49}$`)
var enrollmentNoncePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

type deviceEnrollmentRequest struct {
	DeviceID     string          `json:"device_id"`
	Nonce        string          `json:"nonce"`
	Capabilities json.RawMessage `json:"capabilities"`
}

func normalizeEnrollmentDeviceID(raw string) (string, error) {
	deviceID := strings.ToUpper(strings.TrimSpace(raw))
	if !enrollmentDeviceIDPattern.MatchString(deviceID) {
		return "", fmt.Errorf("invalid device_id")
	}
	return deviceID, nil
}

func parseDeviceEnrollmentRequest(body io.Reader) (deviceEnrollmentRequest, error) {
	limited := io.LimitReader(body, maxDeviceEnrollmentBody+1)
	rawBody, err := io.ReadAll(limited)
	if err != nil {
		return deviceEnrollmentRequest{}, fmt.Errorf("read enrollment request: %w", err)
	}
	if len(rawBody) > maxDeviceEnrollmentBody {
		return deviceEnrollmentRequest{}, fmt.Errorf("enrollment request too large")
	}

	var req deviceEnrollmentRequest
	decoder := json.NewDecoder(strings.NewReader(string(rawBody)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return deviceEnrollmentRequest{}, fmt.Errorf("invalid enrollment json: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return deviceEnrollmentRequest{}, fmt.Errorf("enrollment request has trailing data")
		}
		return deviceEnrollmentRequest{}, fmt.Errorf("invalid trailing enrollment data: %w", err)
	}

	deviceID, err := normalizeEnrollmentDeviceID(req.DeviceID)
	if err != nil {
		return deviceEnrollmentRequest{}, err
	}
	nonce := strings.TrimSpace(req.Nonce)
	if !enrollmentNoncePattern.MatchString(nonce) {
		return deviceEnrollmentRequest{}, fmt.Errorf("invalid enrollment nonce")
	}

	capabilities := req.Capabilities
	if len(capabilities) == 0 {
		capabilities = json.RawMessage(`{}`)
	}
	var capabilityObject map[string]json.RawMessage
	if err := json.Unmarshal(capabilities, &capabilityObject); err != nil || capabilityObject == nil {
		return deviceEnrollmentRequest{}, fmt.Errorf("capabilities must be an object")
	}

	return deviceEnrollmentRequest{
		DeviceID:     deviceID,
		Nonce:        nonce,
		Capabilities: capabilities,
	}, nil
}

// DeviceEnrollmentHandler enrolls a device using a short-lived site token.
func DeviceEnrollmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeEnrollmentError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	enrollmentToken := strings.TrimSpace(r.Header.Get("X-Site-Enrollment-Token"))
	if enrollmentToken == "" {
		writeEnrollmentError(w, http.StatusUnauthorized, "missing site enrollment token")
		return
	}
	if len(enrollmentToken) > 256 {
		writeEnrollmentError(w, http.StatusUnauthorized, "invalid site enrollment token")
		return
	}
	req, err := parseDeviceEnrollmentRequest(r.Body)
	if err != nil {
		writeEnrollmentError(w, http.StatusBadRequest, err.Error())
		return
	}

	schema, siteID, err := database.ResolveDeviceEnrollmentToken(r.Context(), enrollmentToken)
	if err != nil {
		if errors.Is(err, database.ErrEnrollmentTokenNotFound) {
			writeEnrollmentError(w, http.StatusForbidden, "invalid or expired site enrollment token")
			return
		}
		writeEnrollmentError(w, http.StatusServiceUnavailable, "enrollment service unavailable")
		return
	}

	result, err := database.EnrollDevice(r.Context(), schema, siteID, database.HashDeviceEnrollmentToken(enrollmentToken), req.DeviceID, req.Nonce, req.Capabilities)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrEnrollmentTokenNotFound):
			writeEnrollmentError(w, http.StatusForbidden, "invalid or expired site enrollment token")
		case errors.Is(err, database.ErrEnrollmentDisabled):
			writeEnrollmentError(w, http.StatusForbidden, "device enrollment is disabled for this site")
		case errors.Is(err, database.ErrEnrollmentNonceConflict):
			writeEnrollmentError(w, http.StatusConflict, "enrollment nonce was already used")
		case errors.Is(err, database.ErrDeviceAlreadyEnrolled):
			writeEnrollmentError(w, http.StatusConflict, "device is already enrolled")
		case errors.Is(err, database.ErrEnrollmentSiteNotFound):
			writeEnrollmentError(w, http.StatusNotFound, "enrollment site not found")
		default:
			writeEnrollmentError(w, http.StatusInternalServerError, "device enrollment failed")
		}
		return
	}

	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": result, "error": nil})
}

// IssueDeviceEnrollmentTokenHandler rotates a site's enrollment token for an
// authenticated administrator.
func IssueDeviceEnrollmentTokenHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		writeEnrollmentError(w, http.StatusInternalServerError, "invalid tenant context")
		return
	}
	siteID := strings.TrimSpace(r.PathValue("site_id"))
	if siteID == "" {
		writeEnrollmentError(w, http.StatusBadRequest, "site_id is required")
		return
	}
	token, err := database.IssueDeviceEnrollmentToken(r.Context(), schema, siteID, database.DeviceEnrollmentTokenTTL)
	if err != nil {
		if errors.Is(err, database.ErrEnrollmentSiteNotFound) {
			writeEnrollmentError(w, http.StatusNotFound, "site not found")
			return
		}
		writeEnrollmentError(w, http.StatusInternalServerError, "could not issue enrollment token")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": token, "error": nil})
}

// RevokeDeviceEnrollmentTokenHandler invalidates a site's enrollment token for
// an authenticated administrator.
func RevokeDeviceEnrollmentTokenHandler(w http.ResponseWriter, r *http.Request) {
	schema, err := getTenantSchema(r)
	if err != nil {
		writeEnrollmentError(w, http.StatusInternalServerError, "invalid tenant context")
		return
	}
	siteID := strings.TrimSpace(r.PathValue("site_id"))
	if siteID == "" {
		writeEnrollmentError(w, http.StatusBadRequest, "site_id is required")
		return
	}
	if err := database.RevokeDeviceEnrollmentToken(r.Context(), schema, siteID); err != nil {
		if errors.Is(err, database.ErrEnrollmentSiteNotFound) {
			writeEnrollmentError(w, http.StatusNotFound, "site not found")
			return
		}
		writeEnrollmentError(w, http.StatusInternalServerError, "could not revoke enrollment token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeEnrollmentError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": http.StatusText(status), "message": message},
	})
}
