package orchestrator

import (
	"fmt"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	knownHostsMu   sync.Mutex
	knownHostsPath = "./certs/known_hosts"
)

// TofuHostKeyCallback implements Trust On First Use (TOFU) for SSH connections
func TofuHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
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
	if err != nil {
		if keyErr, ok := err.(*knownhosts.KeyError); ok && len(keyErr.Want) == 0 {
			// Host is unknown, append to known_hosts
			f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("could not append to known_hosts: %v", err)
			}
			defer f.Close()

			line := knownhosts.Line([]string{hostname}, key)
			if _, err := f.WriteString(line + "\n"); err != nil {
				return fmt.Errorf("failed to write known_hosts: %v", err)
			}
			return nil // Now trusted
		}
		// Key mismatch or other error
		return err
	}

	return nil
}
