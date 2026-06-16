package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// SealedEnvelope is the on-disk format for at-rest secrets. The first
// 12 bytes are the nonce; the rest is AES-GCM ciphertext + tag.
type SealedEnvelope struct {
	Nonce    []byte
	Cipher   []byte
	FullBlob []byte // nonce || ciphertext, used directly by the DB column
}

// ErrSecretKeyMissing is returned when no encryption key is configured.
var ErrSecretKeyMissing = errors.New("TELEGRAM_ENCRYPTION_KEY is not set")

// Seal encrypts plaintext with AES-256-GCM under the supplied 32-byte key.
// Returns a SealedEnvelope whose FullBlob is the concatenation (nonce ||
// ciphertext) suitable for storing in a single TEXT column.
func Seal(plaintext string, key []byte) (SealedEnvelope, error) {
	if len(key) != 32 {
		return SealedEnvelope{}, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return SealedEnvelope{}, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return SealedEnvelope{}, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SealedEnvelope{}, fmt.Errorf("nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return SealedEnvelope{Nonce: nonce, Cipher: ct, FullBlob: append(nonce, ct...)}, nil
}

// Open reverses Seal. Returns the plaintext and an error if authentication
// fails (which would indicate either the wrong key or tampering).
func Open(env SealedEnvelope, key []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	if len(env.FullBlob) < 12 {
		return "", errors.New("sealed envelope too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonce := env.FullBlob[:gcm.NonceSize()]
	ct := env.FullBlob[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("gcm.Open (auth failure): %w", err)
	}
	return string(pt), nil
}

// DeriveKeyFromPassphrase returns a 32-byte AES key from a user-supplied
// passphrase. The derivation uses SHA-256; for stronger keys use
// scrypt/argon2 in a future iteration. The current scheme is sufficient
// to protect at-rest secrets against casual DB dumps.
func DeriveKeyFromPassphrase(passphrase string) []byte {
	sum := sha256.Sum256([]byte(passphrase))
	return sum[:]
}

// DecodeEnvelope parses a base64-encoded envelope as produced by Seal + base64.
func DecodeEnvelope(b64 string) (SealedEnvelope, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return SealedEnvelope{}, fmt.Errorf("base64 decode: %w", err)
	}
	if len(raw) < 12 {
		return SealedEnvelope{}, errors.New("envelope too short")
	}
	return SealedEnvelope{FullBlob: raw}, nil
}

// EncodeEnvelope is the inverse of DecodeEnvelope.
func EncodeEnvelope(env SealedEnvelope) string {
	return base64.StdEncoding.EncodeToString(env.FullBlob)
}
