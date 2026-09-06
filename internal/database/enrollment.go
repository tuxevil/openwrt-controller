package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeviceEnrollmentTokenTTL bounds how long a site enrollment credential can
// authorize first-boot enrollment.
const DeviceEnrollmentTokenTTL = 24 * time.Hour

var (
	// ErrEnrollmentTokenNotFound indicates that a token is absent, revoked, or expired.
	ErrEnrollmentTokenNotFound = errors.New("device enrollment token not found")
	// ErrEnrollmentSiteNotFound indicates that the target site does not exist.
	ErrEnrollmentSiteNotFound = errors.New("enrollment site not found")
	// ErrEnrollmentDisabled indicates that the site does not permit auto-adoption.
	ErrEnrollmentDisabled = errors.New("device enrollment is disabled")
	// ErrEnrollmentNonceConflict indicates that a nonce is bound to another device.
	ErrEnrollmentNonceConflict = errors.New("enrollment nonce belongs to another device")
	// ErrDeviceAlreadyEnrolled indicates that the device already belongs to a site.
	ErrDeviceAlreadyEnrolled = errors.New("device is already enrolled")
)

// EnrollmentToken is the plaintext site credential and its expiration. The
// plaintext is returned only when the token is issued or rotated.
type EnrollmentToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DeviceEnrollmentResult contains the one-time device credential returned to
// the enrolling device. Created is omitted from the API response.
type DeviceEnrollmentResult struct {
	DeviceID string `json:"device_id"`
	SiteID   string `json:"site_id"`
	Token    string `json:"device_token"`
	Created  bool   `json:"-"`
}

// HashDeviceEnrollmentToken returns the database representation of a site
// enrollment token without retaining the plaintext credential.
func HashDeviceEnrollmentToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func hashEnrollmentNonce(nonce string) string {
	digest := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(digest[:])
}

func randomEnrollmentToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// IssueDeviceEnrollmentToken rotates and returns a short-lived site enrollment
// token for an existing site.
func IssueDeviceEnrollmentToken(ctx context.Context, schema, siteID string, ttl time.Duration) (EnrollmentToken, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return EnrollmentToken{}, err
	}
	if strings.TrimSpace(siteID) == "" {
		return EnrollmentToken{}, ErrEnrollmentSiteNotFound
	}
	if ttl <= 0 {
		return EnrollmentToken{}, fmt.Errorf("enrollment token TTL must be positive")
	}

	var exists bool
	if err := Tx(ctx).QueryRow("SELECT EXISTS (SELECT 1 FROM "+safeSchema+".sites WHERE id = $1)", siteID).Scan(&exists); err != nil {
		return EnrollmentToken{}, err
	}
	if !exists {
		return EnrollmentToken{}, ErrEnrollmentSiteNotFound
	}

	token, err := randomEnrollmentToken()
	if err != nil {
		return EnrollmentToken{}, err
	}
	expiresAt := time.Now().UTC().Add(ttl)
	result, err := Tx(ctx).Exec(
		"UPDATE "+safeSchema+".sites SET enrollment_token_hash = $1, enrollment_token_expires_at = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3",
		HashDeviceEnrollmentToken(token), expiresAt, siteID,
	)
	if err != nil {
		return EnrollmentToken{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return EnrollmentToken{}, err
	} else if affected != 1 {
		return EnrollmentToken{}, ErrEnrollmentSiteNotFound
	}
	return EnrollmentToken{Token: token, ExpiresAt: expiresAt}, nil
}

// RevokeDeviceEnrollmentToken invalidates the active enrollment token for a
// site.
func RevokeDeviceEnrollmentToken(ctx context.Context, schema, siteID string) error {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return err
	}
	result, err := Tx(ctx).Exec(
		"UPDATE "+safeSchema+".sites SET enrollment_token_hash = NULL, enrollment_token_expires_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1",
		siteID,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrEnrollmentSiteNotFound
	}
	return nil
}

// ResolveDeviceEnrollmentToken finds the site bound to a still-valid enrollment
// token. The token itself is never stored or returned by the database layer.
func ResolveDeviceEnrollmentToken(ctx context.Context, token string) (string, string, error) {
	if strings.TrimSpace(token) == "" {
		return "", "", ErrEnrollmentTokenNotFound
	}
	hash := HashDeviceEnrollmentToken(token)
	rows, err := DB.QueryContext(ctx, "SELECT schema_alias FROM tenants WHERE is_active = true")
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	var matchedSchema, matchedSite string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return "", "", err
		}
		schema, schemaErr := SafeTenantSchema(alias)
		if schemaErr != nil {
			continue
		}
		sitesTable := pgx.Identifier{schema, "sites"}.Sanitize()
		var siteID string
		err := DB.QueryRowContext(ctx, fmt.Sprintf(`
			SELECT id::text FROM %s
			WHERE enrollment_token_hash = $1
			  AND enrollment_token_expires_at > CURRENT_TIMESTAMP
			  AND COALESCE(auto_adopt, false) = true`, sitesTable), hash).Scan(&siteID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if matchedSchema != "" {
			return "", "", fmt.Errorf("device enrollment token is ambiguous")
		}
		matchedSchema, matchedSite = schema, siteID
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}
	if matchedSchema == "" {
		return "", "", ErrEnrollmentTokenNotFound
	}
	return matchedSchema, matchedSite, nil
}

// EnrollDevice atomically records the device, issues its device token, and
// binds the caller's nonce so a lost HTTP response can be retried safely.
func EnrollDevice(ctx context.Context, schema, siteID, enrollmentTokenHash, deviceID, nonce string, capabilities []byte) (DeviceEnrollmentResult, error) {
	safeSchema, err := SafeSchemaIdent(schema)
	if err != nil {
		return DeviceEnrollmentResult{}, err
	}
	if siteID == "" || enrollmentTokenHash == "" || deviceID == "" || nonce == "" {
		return DeviceEnrollmentResult{}, fmt.Errorf("enrollment identity is incomplete")
	}
	if len(capabilities) == 0 {
		capabilities = []byte(`{}`)
	}

	tx, err := DB.BeginTx(ctx, nil)
	if err != nil {
		return DeviceEnrollmentResult{}, err
	}
	defer tx.Rollback()

	var autoAdopt, tokenValid bool
	var storedTokenHash sql.NullString
	if err := tx.QueryRow(`SELECT COALESCE(auto_adopt, false), enrollment_token_hash,
		COALESCE(enrollment_token_expires_at > CURRENT_TIMESTAMP, false) FROM `+safeSchema+`.sites
		WHERE id = $1 FOR UPDATE`, siteID).Scan(&autoAdopt, &storedTokenHash, &tokenValid); err == sql.ErrNoRows {
		return DeviceEnrollmentResult{}, ErrEnrollmentTokenNotFound
	} else if err != nil {
		return DeviceEnrollmentResult{}, err
	} else if !autoAdopt {
		return DeviceEnrollmentResult{}, ErrEnrollmentDisabled
	} else if !tokenValid || !storedTokenHash.Valid || storedTokenHash.String != enrollmentTokenHash {
		return DeviceEnrollmentResult{}, ErrEnrollmentTokenNotFound
	}

	nonceHash := hashEnrollmentNonce(nonce)
	var nonceDeviceID string
	err = tx.QueryRow(
		"SELECT device_id FROM "+safeSchema+".device_enrollment_nonces WHERE site_id = $1 AND nonce_hash = $2 AND expires_at > CURRENT_TIMESTAMP",
		siteID, nonceHash,
	).Scan(&nonceDeviceID)
	if err == nil {
		if nonceDeviceID != deviceID {
			return DeviceEnrollmentResult{}, ErrEnrollmentNonceConflict
		}
		var storedToken, storedSiteID sql.NullString
		if err := tx.QueryRow("SELECT device_token, site_id::text FROM "+safeSchema+".devices WHERE id = $1", deviceID).Scan(&storedToken, &storedSiteID); err != nil {
			return DeviceEnrollmentResult{}, err
		}
		if !storedSiteID.Valid || storedSiteID.String != siteID || !storedToken.Valid || storedToken.String == "" {
			return DeviceEnrollmentResult{}, fmt.Errorf("enrollment record is incomplete")
		}
		if err := tx.Commit(); err != nil {
			return DeviceEnrollmentResult{}, err
		}
		return DeviceEnrollmentResult{DeviceID: deviceID, SiteID: siteID, Token: storedToken.String, Created: false}, nil
	} else if err != sql.ErrNoRows {
		return DeviceEnrollmentResult{}, err
	}

	var currentSiteID, currentToken sql.NullString
	deviceQueryErr := tx.QueryRow(
		"SELECT site_id::text, device_token FROM "+safeSchema+".devices WHERE id = $1 FOR UPDATE", deviceID,
	).Scan(&currentSiteID, &currentToken)
	if deviceQueryErr != nil && deviceQueryErr != sql.ErrNoRows {
		return DeviceEnrollmentResult{}, deviceQueryErr
	}
	if deviceQueryErr == nil && currentSiteID.Valid && currentSiteID.String != "" {
		return DeviceEnrollmentResult{}, ErrDeviceAlreadyEnrolled
	}

	deviceToken, err := randomEnrollmentToken()
	if err != nil {
		return DeviceEnrollmentResult{}, err
	}
	if deviceQueryErr == sql.ErrNoRows {
		_, err = tx.Exec(`
			INSERT INTO `+safeSchema+`.devices
				(id, site_id, status, device_token, capabilities, capabilities_updated_at, last_seen_at, updated_at)
			VALUES ($1, $2, 'Adopted', $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			deviceID, siteID, deviceToken, capabilities,
		)
	} else {
		_, err = tx.Exec(`
			UPDATE `+safeSchema+`.devices
			SET site_id = $1, status = 'Adopted', device_token = $2, capabilities = $3,
				capabilities_updated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = $4`, siteID, deviceToken, capabilities, deviceID)
	}
	if err != nil {
		return DeviceEnrollmentResult{}, err
	}

	_, err = tx.Exec(`
		INSERT INTO `+safeSchema+`.device_enrollment_nonces
			(site_id, nonce_hash, device_id, expires_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP + INTERVAL '24 hours')`, siteID, nonceHash, deviceID)
	if err != nil {
		return DeviceEnrollmentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeviceEnrollmentResult{}, err
	}
	return DeviceEnrollmentResult{DeviceID: deviceID, SiteID: siteID, Token: deviceToken, Created: true}, nil
}
