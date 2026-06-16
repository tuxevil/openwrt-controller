package handlers

import (
	"os"
	"testing"
)

// TestMain ensures the JWT_SECRET env var is set BEFORE the package's
// init() runs, since api/handlers/auth.go calls secrets.JWTSecret() at
// package init and log.Fatal's if the secret is missing. Without this
// guard the whole test binary would abort the moment any test file
// loads the package.
func TestMain(m *testing.M) {
	os.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	os.Exit(m.Run())
}
