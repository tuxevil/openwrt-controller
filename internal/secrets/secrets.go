// Package secrets centralises the loading of process-wide cryptographic
// material. Before this package existed, getJWTSecret() was duplicated in
// api/handlers/auth.go and api/middleware/auth.go, each with its own
// log.Fatal. A drift between the two could (and historically would) lead
// to JWTs being signed with one secret and verified with another after
// only one side was reloaded.
package secrets

import (
	"log"
	"os"
)

const minSecretLength = 32

// JWTSecret returns the configured JWT signing secret, or calls log.Fatal
// if it is missing or shorter than the minimum length. Called from package
// init() and main() so a misconfiguration is caught at startup.
func JWTSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		log.Fatal("JWT_SECRET environment variable is required and must not be empty")
	}
	if len(s) < minSecretLength {
		log.Fatalf("JWT_SECRET must be at least %d characters (got %d)", minSecretLength, len(s))
	}
	return []byte(s)
}
