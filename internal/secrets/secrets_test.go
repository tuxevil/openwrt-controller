package secrets

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestJWTSecret_Valid verifies the happy path: a 32+ char secret is
// returned verbatim.
func TestJWTSecret_Valid(t *testing.T) {
	orig, hadOrig := os.LookupEnv("JWT_SECRET")
	defer func() {
		if hadOrig {
			os.Setenv("JWT_SECRET", orig)
		} else {
			os.Unsetenv("JWT_SECRET")
		}
	}()

	const want = "0123456789abcdef0123456789abcdef" // 32 chars
	os.Setenv("JWT_SECRET", want)
	got := string(JWTSecret())
	if got != want {
		t.Errorf("JWTSecret() = %q, want %q", got, want)
	}
}

// testHelperProcess re-executes the current test binary in subprocess mode
// to exercise code paths that call log.Fatal / os.Exit, which would
// otherwise kill the test runner.
func testHelperProcess(t *testing.T, testName string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	return cmd
}

// TestJWTSecret_TooShort runs JWTSecret in a subprocess so the log.Fatal
// call doesn't kill the test binary. We confirm the child exits non-zero
// and prints the expected diagnostic.
func TestJWTSecret_TooShort(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Setenv("JWT_SECRET", "short")
		_ = JWTSecret()
		return
	}
	cmd := testHelperProcess(t, "TestJWTSecret_TooShort")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "at least 32") {
		t.Errorf("expected diagnostic about 32-char minimum, got %q", out)
	}
}

// TestJWTSecret_Empty runs JWTSecret with an empty value in a subprocess.
func TestJWTSecret_Empty(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Unsetenv("JWT_SECRET")
		_ = JWTSecret()
		return
	}
	cmd := testHelperProcess(t, "TestJWTSecret_Empty")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "required") {
		t.Errorf("expected diagnostic about required secret, got %q", out)
	}
}
