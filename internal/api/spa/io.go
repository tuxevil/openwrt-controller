package spa

import "os"

// readFile and stat are tiny wrappers so the handler stays a pure
// function of its inputs and tests can stub them later if needed.
func readFile(p string) ([]byte, error)     { return os.ReadFile(p) }
func stat(p string) (os.FileInfo, error)     { return os.Stat(p) }
