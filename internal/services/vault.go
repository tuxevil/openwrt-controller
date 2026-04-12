package services

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"openwrt-controller/internal/database"

	"golang.org/x/crypto/ssh"
)

func CreateBackup(deviceID string) error {
	// Obtenemos la topología/IP más reciente
	var ip string
	err := database.DB.QueryRow(`SELECT COALESCE(last_ip, '') FROM devices WHERE id = $1`, deviceID).Scan(&ip)
	if err != nil || ip == "" {
		return fmt.Errorf("device IP not found")
	}

	cmd := "tar -cz -C /etc config | base64"
	
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

	out, err := session.Output(cmd)
	if err != nil {
		return fmt.Errorf("backup command fail: %w", err)
	}

	b64out := strings.TrimSpace(string(out))
	rawBytes, err := base64.StdEncoding.DecodeString(b64out)
	if err != nil {
		return fmt.Errorf("base64 decode fail: %w", err)
	}

	// Calculate checksum
	hasher := sha256.New()
	hasher.Write(rawBytes)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	_, err = database.DB.Exec(`
		INSERT INTO backups (device_id, checksum, content)
		VALUES ($1, $2, $3)
	`, deviceID, checksum, rawBytes)

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
			rows, err := database.DB.Query(`SELECT id FROM devices WHERE last_ip IS NOT NULL AND status != 'OFFLINE'`)
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
				go CreateBackup(dev) // Parallel backup map-reduce
			}
		}
	}()
}
