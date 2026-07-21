package spa

import (
	"os"
	"path/filepath"
	"testing"
)

// This file intentionally left minimal. The handler tests were
// written for the CSP / script-stripping / boot-fallback layer
// that was added and then removed. Those features caused more
// problems than they solved (blocked legitimate inline scripts,
// added complexity, didn't address the real GPS issue). The SPA
// handler is now back to its original clean form.

func TestFileExistsDoesNotEscapeDistributionDirectory(t *testing.T) {
	root := t.TempDir()
	distDir := filepath.Join(root, "dist")
	if err := os.Mkdir(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	if fileExists(distDir, "../secret.txt") {
		t.Fatal("fileExists accepted a path outside the distribution directory")
	}
}
