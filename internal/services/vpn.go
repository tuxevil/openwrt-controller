package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"

	"openwrt-controller/internal/database"

	"golang.org/x/crypto/curve25519"
)

// GenerateWireGuardKeys generates a pair of Curve25519 keys suitable for WireGuard
func GenerateWireGuardKeys() (priv string, pub string, err error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return "", "", err
	}
	// WireGuard specific key clamping
	privateKey[0] &= 248
	privateKey[31] = (privateKey[31] & 127) | 64

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return base64.StdEncoding.EncodeToString(privateKey[:]), base64.StdEncoding.EncodeToString(publicKey[:]), nil
}

// AssignInternalIP assigns a unique 10.8.0.x IP for the given device ID
func AssignInternalIP(schema string, deviceID string) (string, error) {
	if schema == "" {
		schema = "public"
	}
	var wgIP sql.NullString
	err := database.DB.QueryRow(fmt.Sprintf("SELECT wg_ip FROM %s.devices WHERE id = $1", schema), deviceID).Scan(&wgIP)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if wgIP.Valid && wgIP.String != "" {
		return wgIP.String, nil
	}

	// Lock the table for concurrent protection or simply pick max and increment
	// We'll use a transaction
	tx, err := database.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Find highest assigned IP in 10.8.0.x
	// E.g. IPs are like 10.8.0.X. We can cast the last octet or just sequentially look up.
	// We'll pick sequential starting from 2
	rows, err := tx.Query(fmt.Sprintf("SELECT wg_ip FROM %s.devices WHERE wg_ip IS NOT NULL AND wg_ip LIKE '10.8.0.%%'", schema))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	ipSet := make(map[string]bool)
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err == nil {
			ipSet[ip] = true
		}
	}

	newIP := ""
	for i := 2; i < 254; i++ {
		testIP := fmt.Sprintf("10.8.0.%d", i)
		if !ipSet[testIP] {
			newIP = testIP
			break
		}
	}

	if newIP == "" {
		return "", fmt.Errorf("no available IPs in 10.8.0.x subnet")
	}

	_, err = tx.Exec(fmt.Sprintf("UPDATE %s.devices SET wg_ip = $1 WHERE id = $2", schema), newIP, deviceID)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return newIP, nil
}
