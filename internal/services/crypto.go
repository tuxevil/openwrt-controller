package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// SealedEnvelope is the on-disk format for at-rest secrets. Seal stores the
// legacy nonce||ciphertext form; SealWithPassphrase stores version||salt||
// nonce||ciphertext so the KDF salt is persisted with each record.
type SealedEnvelope struct {
	Nonce    []byte
	Cipher   []byte
	FullBlob []byte // nonce || ciphertext, used directly by the DB column
}

// ErrSecretKeyMissing is returned when no encryption key is configured.
var ErrSecretKeyMissing = errors.New("TELEGRAM_ENCRYPTION_KEY is not set")

const (
	passphraseEnvelopeVersion byte = 2
	passphraseSaltSize             = 16
	argon2MemoryKiB                = 64 * 1024
	argon2Iterations               = 3
	argon2Threads                  = 4
	argon2KeySize                  = 32
)

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

// DeriveKeyFromPassphrase returns a 32-byte AES key using Argon2id and the
// caller-provided salt. Callers must generate and persist a random salt with
// the encrypted envelope; SealWithPassphrase handles that correctly.
func DeriveKeyFromPassphrase(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, argon2Iterations, argon2MemoryKiB, argon2Threads, argon2KeySize)
}

// SealWithPassphrase derives a key with a per-envelope random salt and stores
// the version and salt before the existing nonce||ciphertext payload.
func SealWithPassphrase(plaintext, passphrase string) (SealedEnvelope, error) {
	salt := make([]byte, passphraseSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return SealedEnvelope{}, fmt.Errorf("passphrase salt: %w", err)
	}

	key := DeriveKeyFromPassphrase(passphrase, salt)
	inner, err := Seal(plaintext, key)
	if err != nil {
		return SealedEnvelope{}, err
	}

	fullBlob := make([]byte, 1+len(salt)+len(inner.FullBlob))
	fullBlob[0] = passphraseEnvelopeVersion
	copy(fullBlob[1:], salt)
	copy(fullBlob[1+len(salt):], inner.FullBlob)
	inner.FullBlob = fullBlob
	return inner, nil
}

// OpenWithPassphrase opens an envelope produced by SealWithPassphrase.
func OpenWithPassphrase(env SealedEnvelope, passphrase string) (string, error) {
	minimum := 1 + passphraseSaltSize + 12
	if len(env.FullBlob) < minimum || env.FullBlob[0] != passphraseEnvelopeVersion {
		return "", errors.New("unsupported passphrase envelope")
	}

	saltStart := 1
	saltEnd := saltStart + passphraseSaltSize
	salt := env.FullBlob[saltStart:saltEnd]
	key := DeriveKeyFromPassphrase(passphrase, salt)
	return Open(SealedEnvelope{FullBlob: env.FullBlob[saltEnd:]}, key)
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
