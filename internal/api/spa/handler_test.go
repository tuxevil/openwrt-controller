package spa

import (
	"net/http"
	"net/http/httptest"
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

func TestResolveDistributionPathRejectsNonLocalPaths(t *testing.T) {
	root := t.TempDir()

	for _, path := range []string{
		"../secret.txt",
		"assets/../../secret.txt",
		"/../../etc/passwd",
		"assets\\..\\secret.txt",
	} {
		if _, ok := resolveDistributionPath(root, path); ok {
			t.Fatalf("resolveDistributionPath accepted unsafe path %q", path)
		}
	}
}

func TestResolveDistributionPathReturnsContainedPath(t *testing.T) {
	root := t.TempDir()

	got, ok := resolveDistributionPath(root, "/assets/app.js")
	if !ok {
		t.Fatal("resolveDistributionPath rejected a local asset path")
	}
	want := filepath.Join(root, "assets", "app.js")
	if got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestFileExistsDoesNotFollowSymlinkOutsideDistributionDirectory(t *testing.T) {
	root := t.TempDir()
	distDir := filepath.Join(root, "dist")
	if err := os.Mkdir(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(distDir, "leak.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if fileExists(distDir, "/leak.txt") {
		t.Fatal("fileExists followed a symlink outside the distribution directory")
	}
}

func TestHandlerReloadsIndexAfterFrontendBuild(t *testing.T) {
	distDir := t.TempDir()
	indexPath := filepath.Join(distDir, "index.html")
	oldIndex := []byte(`<script type="module" src="/assets/old.js"></script>`)
	newIndex := []byte(`<script type="module" src="/assets/new.js"></script>`)
	if err := os.WriteFile(indexPath, oldIndex, 0o644); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(distDir)
	if err := os.WriteFile(indexPath, newIndex, 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/site/example", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if got := res.Body.Bytes(); string(got) != string(newIndex) {
		t.Fatalf("handler served stale index.html: got %q, want %q", got, newIndex)
	}
}
