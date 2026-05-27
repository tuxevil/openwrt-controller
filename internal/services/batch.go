package services

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"openwrt-controller/internal/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type DeviceResult struct {
	DeviceID string `json:"device_id"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
}

func RunMassCommand(siteID, command string) []DeviceResult {
	// Load controller private key
	keyPath := "./certs/id_controller"
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		log.Printf("[BATCH] Cannot read private key: %v", err)
		return []DeviceResult{{DeviceID: "CONTROLLER", Error: "No SSH private key found at " + keyPath}}
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		log.Printf("[BATCH] Cannot parse private key: %v", err)
		return []DeviceResult{{DeviceID: "CONTROLLER", Error: "Invalid private key format"}}
	}

	// Fetch all devices in site with their last known IP
	rows, err := database.DB.Query(`
		SELECT id, COALESCE(last_ip, '') as ip
		FROM devices 
		WHERE site_id = $1 AND last_ip IS NOT NULL AND last_ip != ''
	`, siteID)
	if err != nil {
		log.Printf("[BATCH] DB query error: %v", err)
		return []DeviceResult{{DeviceID: "DB", Error: err.Error()}}
	}
	defer rows.Close()

	type deviceTarget struct {
		id string
		ip string
	}

	var targets []deviceTarget
	for rows.Next() {
		var id, ip string
		if err := rows.Scan(&id, &ip); err == nil && ip != "" {
			targets = append(targets, deviceTarget{id: id, ip: ip})
		}
	}

	if len(targets) == 0 {
		return []DeviceResult{{DeviceID: "BATCH", Error: "No reachable devices found in site"}}
	}

	results := make([]DeviceResult, len(targets))
	var wg sync.WaitGroup

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, dev deviceTarget) {
			defer wg.Done()
			results[idx] = runSSHCommand(dev.id, dev.ip, command, signer)
		}(i, target)
	}

	wg.Wait()
	return results
}

func runSSHCommand(deviceID, ip, command string, signer ssh.Signer) DeviceResult {
	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: tofuHostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := ip + ":22"
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		// Try common fallback port
		addr = ip + ":22"
		return DeviceResult{
			DeviceID: deviceID,
			Error:    fmt.Sprintf("SSH dial failed: %v", err),
		}
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return DeviceResult{
			DeviceID: deviceID,
			Error:    fmt.Sprintf("Session failed: %v", err),
		}
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		errMsg := fmt.Sprintf("Exit error: %v", err)
		if stderr.Len() > 0 {
			errMsg += " | STDERR: " + strings.TrimSpace(stderr.String())
		}
		return DeviceResult{
			DeviceID: deviceID,
			Output:   strings.TrimSpace(stdout.String()),
			Error:    errMsg,
		}
	}

	return DeviceResult{
		DeviceID: deviceID,
		Output:   strings.TrimSpace(stdout.String()),
	}
}

var (
	knownHostsPath = "./certs/known_hosts"
	knownHostsMu   sync.Mutex
)

// tofuHostKeyCallback implements Trust On First Use (TOFU) for SSH connections
func tofuHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()

	// Ensure certs directory exists
	os.MkdirAll("./certs", 0700)

	// Create file if it doesn't exist
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
		}
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return fmt.Errorf("could not create hostkeycallback: %v", err)
	}

	err = hostKeyCallback(hostname, remote, key)
	if err == nil {
		return nil // Key is known and matches
	}

	// If the error is a key mismatch, reject it!
	if keyErr, ok := err.(*knownhosts.KeyError); ok && len(keyErr.Want) > 0 {
		return fmt.Errorf("host key mismatch for %s. MITM attack? %v", hostname, err)
	}

	// Key is unknown, let's append it (Trust On First Use)
	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open known_hosts: %v", err)
	}
	defer f.Close()

	line := knownhosts.Line([]string{hostname}, key)
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("failed to write to known_hosts: %v", err)
	}

	return nil
}
