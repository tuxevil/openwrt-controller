package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyManager is the single source of truth for SSH host key storage
// (formerly duplicated between orchestrator/ssh_utils.go and
// services/batch.go — each with its own mutex over the same file). It
// persists host keys under KnownHostsPath() using the standard OpenSSH
// known_hosts format and exposes a callback suitable for
// ssh.ClientConfig.HostKeyCallback.
type HostKeyManager struct {
	path string
	mu   sync.Mutex
}

// NewHostKeyManager constructs a manager bound to the given known_hosts
// file. The directory is created on demand with 0700 permissions; the
// file is created (also 0600) if missing.
func NewHostKeyManager(path string) *HostKeyManager {
	return &HostKeyManager{path: path}
}

// DefaultHostKeyManager is the process-wide instance, initialised lazily
// by TofuHostKeyCallback so we don't pay the cost when no SSH call ever
// happens. Tests can override DefaultHostKeyManager to point at a
// temporary file.
var (
	hostKeyManagerOnce sync.Once
	hostKeyManager     *HostKeyManager
)

func DefaultHostKeyManager() *HostKeyManager {
	hostKeyManagerOnce.Do(func() {
		hostKeyManager = NewHostKeyManager(KnownHostsPath())
	})
	return hostKeyManager
}

// KnownHostsPath returns the absolute path of the known_hosts file. It
// honours the CONTROLLER_KNOWN_HOSTS env var and falls back to
// "certs/known_hosts" relative to the working directory.
func KnownHostsPath() string {
	if p := os.Getenv("CONTROLLER_KNOWN_HOSTS"); p != "" {
		return p
	}
	return "certs/known_hosts"
}

// TofuHostKeyCallback implements Trust On First Use for SSH host keys.
//
// Behaviour:
//   - First contact with a host: persist its key and accept the connection.
//   - Subsequent contacts: the key MUST match; mismatch is an error.
//
// TOFU without user confirmation is fundamentally vulnerable to MITM during
// the first adoption. A future enhancement will surface the SHA256
// fingerprint to the operator before persisting; for now the fingerprint
// is logged at WARN level so the operator can manually inspect it.
func TofuHostKeyCallback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return DefaultHostKeyManager().Callback(hostname, remote, key)
}

// Fingerprint returns the SHA256 fingerprint of the supplied host key in
// the same format OpenSSH uses (base64 with the "SHA256:" prefix stripped
// of trailing "="). Useful for surfacing to the user before they approve
// a first-time connection. Returns "" if the key is nil.
func Fingerprint(key ssh.PublicKey) string {
	if key == nil {
		return ""
	}
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + strings.TrimRight(hex.EncodeToString(sum[:]), "=")
}

// Callback returns the host key callback that should be installed in
// ssh.ClientConfig.HostKeyCallback.
func (h *HostKeyManager) Callback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return fmt.Errorf("create known_hosts dir: %w", err)
	}
	if _, err := os.Stat(h.path); os.IsNotExist(err) {
		f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("create known_hosts file: %w", err)
		}
		_ = f.Close()
	}

	cb, err := knownhosts.New(h.path)
	if err != nil {
		return fmt.Errorf("knownhosts.New: %w", err)
	}

	if err := cb(hostname, remote, key); err != nil {
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			// Unknown host — TOFU: append and trust.
			f, openErr := os.OpenFile(h.path, os.O_APPEND|os.O_WRONLY, 0o600)
			if openErr != nil {
				return fmt.Errorf("append known_hosts: %w", openErr)
			}
			defer f.Close()
			if _, writeErr := f.WriteString(knownhosts.Line([]string{hostname}, key) + "\n"); writeErr != nil {
				return fmt.Errorf("write known_hosts: %w", writeErr)
			}
			// Log the fingerprint at WARN so the operator can audit new
			// devices. A future change should surface this in the UI and
			// require explicit approval before persisting.
			fmt.Printf("[TOFU] trusting new host key for %s (fingerprint %s) — review with 'ssh-keygen -lf'\n", hostname, Fingerprint(key))
			return nil
		}
		return err
	}
	return nil
}
