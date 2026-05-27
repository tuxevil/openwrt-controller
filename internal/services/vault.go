package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"openwrt-controller/internal/database"

	"golang.org/x/crypto/ssh"
)

func CreateBackup(schema, deviceID string) error {
	// Obtenemos la topología/IP más reciente
	var ip string
	err := database.DB.QueryRow(fmt.Sprintf(`SELECT COALESCE(last_ip, '') FROM %s.devices WHERE id = $1`, schema), deviceID).Scan(&ip)
	if err != nil || ip == "" {
		return fmt.Errorf("device IP not found")
	}

	// SSH soporta binario nativo — no necesitamos base64.
	// /sbin/sysupgrade --create-backup - escribe tar.gz a stdout.
	cmd := "/sbin/sysupgrade --create-backup -"


	// Obtenemos la llave asimétrica para auth
	keyBytes, err := loadControllerPrivateKey()
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", ip+":22", config)
	if err != nil {
		return fmt.Errorf("ssh dial fail: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session fail: %w", err)
	}
	defer session.Close()

	// session.Output() devuelve bytes crudos — el tar.gz de sysupgrade.
	// SSH maneja binarios nativamente, no necesitamos base64.
	rawBytes, err := session.Output(cmd)
	if err != nil {
		return fmt.Errorf("backup command fail: %w", err)
	}

	// Calculate checksum
	hasher := sha256.New()
	hasher.Write(rawBytes)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	_, err = database.DB.Exec(fmt.Sprintf(`
		INSERT INTO %s.backups (device_id, checksum, content)
		VALUES ($1, $2, $3)
	`, schema), deviceID, checksum, rawBytes)

	log.Printf("[VAULT] Backup completed for %s. Checksum: %s", deviceID, checksum[:8])
	return err
}

func loadControllerPrivateKey() ([]byte, error) {
	return os.ReadFile("./certs/id_controller")
}

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
					go CreateBackup(schema, dev) // Parallel backup map-reduce
				}
			}
		}
	}()
}
