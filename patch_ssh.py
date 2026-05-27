import os
import re

filepath = 'internal/services/batch.go'
with open(filepath, 'r') as f:
    content = f.read()

tofu_code = """
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
	if _, err := f.WriteString(line + "\\n"); err != nil {
		return fmt.Errorf("failed to write to known_hosts: %v", err)
	}

	return nil
}
"""

# add import
content = content.replace('"golang.org/x/crypto/ssh"', '"golang.org/x/crypto/ssh"\n\t"golang.org/x/crypto/ssh/knownhosts"')

# append TOFU code at the end
content += tofu_code

# replace callback
old_cb = """HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil // Trust all (internal network)
		},"""
content = content.replace(old_cb, "HostKeyCallback: tofuHostKeyCallback,")

with open(filepath, 'w') as f:
    f.write(content)
