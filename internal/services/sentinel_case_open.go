package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"openwrt-controller/internal/database"
)

// OpenSentinelCasePreservingEvidence is the Case-centered variant of the
// legacy opener. Repeated triggers may refresh severity/summary, but they must
// never replace the evidence ledger produced by earlier investigations.
func OpenSentinelCasePreservingEvidence(schema, source, siteID, deviceID, severity, title, summary string, originEvidence interface{}) (string, error) {
	safeSchema, err := sentinelSchema(schema)
	if err != nil {
		return "", err
	}
	fingerprint := strings.Join([]string{source, siteID, deviceID, title}, ":")
	var existing string
	// #nosec G201 -- safeSchema is validated by sentinelSchema; fingerprint is parameterized.
	err = database.DB.QueryRow(fmt.Sprintf(`SELECT id::text FROM %s.sentinel_cases
		WHERE fingerprint = $1 AND status IN ('OPEN','INVESTIGATING') ORDER BY updated_at DESC LIMIT 1`, safeSchema), fingerprint).Scan(&existing)
	if err == nil {
		// #nosec G201 -- safeSchema is validated by sentinelSchema; values are parameterized.
		_, updateErr := database.DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_cases SET severity = $1, summary = $2,
			updated_at = CURRENT_TIMESTAMP WHERE id = $3`, safeSchema), severity, summary, existing)
		return existing, updateErr
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	originJSON, err := json.Marshal(originEvidence)
	if err != nil {
		return "", err
	}
	ledger := SentinelCaseEvidenceLedger{
		Version: sentinelCaseEvidenceLedgerVersion,
		Origin:  originJSON,
		Runs:    []SentinelCaseEvidenceRun{},
	}
	ledgerJSON, err := json.Marshal(ledger)
	if err != nil {
		return "", err
	}
	var id string
	// #nosec G201 -- safeSchema is validated by sentinelSchema; values are parameterized.
	err = database.DB.QueryRow(fmt.Sprintf(`INSERT INTO %s.sentinel_cases
		(fingerprint, source, site_id, device_id, severity, status, title, summary, evidence)
		VALUES ($1, $2, NULLIF($3,'')::uuid, NULLIF($4,''), $5, 'OPEN', $6, $7, $8)
		RETURNING id::text`, safeSchema), fingerprint, source, siteID, deviceID, severity, title, summary, ledgerJSON).Scan(&id)
	return id, err
}
