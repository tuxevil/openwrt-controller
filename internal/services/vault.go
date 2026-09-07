package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/orchestrator"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/semaphore"
)

const backupCommandTimeout = 2 * time.Minute

func backupSSHAddress(ip string) string {
	return net.JoinHostPort(ip, "22")
}

// CreateBackup creates a backup for a device resolved within a tenant schema.
func CreateBackup(ctx context.Context, schema, deviceID string) error {
	return createBackup(ctx, schema, "", deviceID)
}

// CreateBackupForSite creates a backup only after resolving the device inside
// the supplied tenant site.
func CreateBackupForSite(ctx context.Context, schema, siteID, deviceID string) error {
	return createBackup(ctx, schema, siteID, deviceID)
}

func createBackup(ctx context.Context, schema, siteID, deviceID string) error {
	backupCtx, backupCancel := context.WithTimeout(ctx, backupCommandTimeout)
	defer backupCancel()
	sqlSchema, err := database.SafeSQLSchemaIdent(schema)
	if err != nil {
		return err
	}
	// Resolve the most recent topology/IP within the authorized scope.
	var ip string
	query := fmt.Sprintf(`SELECT COALESCE(last_ip, '') FROM %s.devices WHERE id = $1`, sqlSchema)
	args := []any{deviceID}
	if siteID != "" {
		query = fmt.Sprintf(`SELECT COALESCE(last_ip, '') FROM %s.devices WHERE id = $1 AND site_id = $2`, sqlSchema)
		args = append(args, siteID)
	}
	err = database.DB.QueryRowContext(backupCtx, query, args...).Scan(&ip)
	if err != nil || ip == "" {
		return fmt.Errorf("device IP not found")
	}

	// SSH transports binary data natively, so base64 is unnecessary.
	// /sbin/sysupgrade --create-backup - writes the tar.gz to stdout.
	cmd := "/sbin/sysupgrade --create-backup -"

	// Load the asymmetric authentication key.
	keyStore := orchestrator.GetKeyStore()
	if keyStore == nil {
		return fmt.Errorf("controller SSH key not configured")
	}
	signer, err := keyStore.Get()
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: orchestrator.TofuHostKeyCallback,
		Timeout:         10 * time.Second,
	}

	if err := backupCtx.Err(); err != nil {
		return err
	}
	client, err := ssh.Dial("tcp", backupSSHAddress(ip), config)
	if err != nil {
		return fmt.Errorf("ssh dial fail: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session fail: %w", err)
	}
	defer session.Close()

	// SSH transports binary data natively, so base64 is unnecessary. Use
	// Start/Wait instead of Output so a cancelled rollout can close the remote command.
	var raw bytes.Buffer
	session.Stdout = &raw
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("backup command start fail: %w", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- session.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			return fmt.Errorf("backup command fail: %w", err)
		}
	case <-backupCtx.Done():
		_ = session.Close()
		_ = client.Close()
		<-wait
		return backupCtx.Err()
	}
	rawBytes := raw.Bytes()

	// Calculate checksum
	hasher := sha256.New()
	hasher.Write(rawBytes)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	_, err = database.DB.ExecContext(backupCtx, fmt.Sprintf(`
		INSERT INTO %s.backups (device_id, checksum, content)
		VALUES ($1, $2, $3)
	`, sqlSchema), deviceID, checksum, rawBytes)

	log.Printf("[VAULT] Backup completed for %s. Checksum: %s", deviceID, checksum[:8])
	return err
}

var _ = ssh.AuthMethod(nil) // keep ssh import in use while migrating

// vaultBackupLimiter caps the number of concurrent sysupgrade backups so a
// large fleet does not exhaust file descriptors or saturate the network.
// Defaults to 10; override with VAULT_BACKUP_CONCURRENCY.
var vaultBackupLimiter = func() *semaphore.Weighted {
	n := int64(10)
	if raw := os.Getenv("VAULT_BACKUP_CONCURRENCY"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			n = int64(v)
		}
	}
	return semaphore.NewWeighted(n)
}()

func StartVaultCron() {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			log.Println("[VAULT] Running scheduled mass backup...")
			tenants, err := ListTenants()
			if err != nil {
				continue
			}
			for _, t := range tenants {
				schema := "tenant_" + t.SchemaAlias
				rows, err := database.DB.Query(fmt.Sprintf(`SELECT id FROM %s.devices WHERE last_ip IS NOT NULL AND status != 'OFFLINE'`, schema))
				if err != nil {
					continue
				}
				var devices []string
				for rows.Next() {
					var id string
					if err := rows.Scan(&id); err == nil {
						devices = append(devices, id)
					}
				}
				rows.Close()

				for _, dev := range devices {
					dev := dev
					go func() {
						_ = vaultBackupLimiter.Acquire(context.Background(), 1)
						defer vaultBackupLimiter.Release(1)
						_ = CreateBackup(context.Background(), schema, dev)
					}()
				}
			}
		}
	}()
}
