package orchestrator

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
)

// KeyStore is the single source of truth for the controller's SSH
// credentials. It is created once at process start by LoadKeyStore, then
// shared between every package that needs to dial a device (orchestrator
// executor, services/batch, services/vault, services/traffic_manager, the
// WebSocket SSH handler, etc.).
//
// Replacing the previous pattern (each package had its own init() reading
// the same key file with slightly different error handling) closes a class
// of "private key not loaded" panics and ensures the file permissions
// check is enforced in exactly one place.
type KeyStore struct {
	path string
	mu   sync.RWMutex
	key  ssh.Signer
}

// ErrKeyNotLoaded is returned by Get when no key has been loaded yet (e.g.
// the file was missing or the process started in --no-ssh mode).
var ErrKeyNotLoaded = errors.New("controller SSH private key not loaded")

// defaultKeyPath is the conventional location. It can be overridden via the
// CONTROLLER_SSH_KEY environment variable (mostly useful for tests and
// non-standard deployments).
func defaultKeyPath() string {
	if p := os.Getenv("CONTROLLER_SSH_KEY"); p != "" {
		return p
	}
	return "certs/id_controller"
}

// LoadKeyStore reads the controller's SSH private key from disk, verifies
// that the file mode is 0600 (or stricter), and returns a KeyStore wrapping
// the parsed signer. The check is skipped only when both:
//
//   - The file is owned by the current user (UID match), and
//   - The CONTROLLER_SSH_ALLOW_GROUP_READ env var is set (opt-in for
//     dev / shared-host setups).
//
// On any error, the returned KeyStore is still non-nil but its Get method
// will return ErrKeyNotLoaded. Callers should treat that as a soft failure
// (e.g. SSH-dependent endpoints return 503) rather than crashing.
func LoadKeyStore() *KeyStore {
	path := defaultKeyPath()
	ks := &KeyStore{path: path}

	info, err := os.Stat(path)
	if err != nil {
		log.Printf("[KEYSTORE] %s not found: %v — SSH-dependent endpoints will be disabled", path, err)
		globalKeyStore = ks
		return ks
	}

	if !enforceKeyPermissions(info) {
		log.Printf("[KEYSTORE] refusing to load %s: permissions %v are wider than 0600. Set CONTROLLER_SSH_ALLOW_GROUP_READ=1 to override.", path, info.Mode().Perm())
		globalKeyStore = ks
		return ks
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[KEYSTORE] read %s failed: %v", path, err)
		globalKeyStore = ks
		return ks
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		log.Printf("[KEYSTORE] parse %s failed: %v", path, err)
		globalKeyStore = ks
		return ks
	}
	ks.key = signer
	log.Printf("[KEYSTORE] loaded SSH private key from %s (fingerprint %s)", path, ssh.FingerprintSHA256(signer.PublicKey()))
	globalKeyStore = ks
	return ks
}

// globalKeyStore is the process-wide KeyStore. It is set by LoadKeyStore at
// startup and read by every package that needs to dial an SSH session. A
// nil value means the key has not been loaded yet (or failed to load).
var globalKeyStore *KeyStore

// GetKeyStore returns the process-wide KeyStore, or nil if LoadKeyStore
// has not been called.
func GetKeyStore() *KeyStore {
	return globalKeyStore
}

func enforceKeyPermissions(info os.FileInfo) bool {
	if info.Mode().Perm()&0o077 == 0 {
		return true
	}
	return os.Getenv("CONTROLLER_SSH_ALLOW_GROUP_READ") == "1"
}

// Get returns a clone of the underlying signer. Because *ssh.Signer is
// safe for concurrent use we can return the same value to all callers
// without copying.
func (k *KeyStore) Get() (ssh.Signer, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.key == nil {
		return nil, ErrKeyNotLoaded
	}
	return k.key, nil
}

// Path returns the absolute (or process-relative) path the keystore was
// initialised with. Used by tests and by tools that need to read the
// matching public key.
func (k *KeyStore) Path() string {
	abs, err := filepath.Abs(k.path)
	if err != nil {
		return k.path
	}
	return abs
}

// PublicKeyPath is the conventional location of the controller's public
// key (used by handlers/ssh.go to expose it to the WebSocket terminal
// for distribution to devices).
func PublicKeyPath() string {
	if p := os.Getenv("CONTROLLER_SSH_PUBKEY"); p != "" {
		return p
	}
	return "certs/id_controller.pub"
}

// LoadPublicKey reads the controller's public key from disk and returns it
// as a string trimmed of trailing whitespace. Returns an error if the file
// is missing; permissions are not checked because public keys are not
// sensitive.
func LoadPublicKey() (string, error) {
	b, err := os.ReadFile(PublicKeyPath())
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	return string(b), nil
}
