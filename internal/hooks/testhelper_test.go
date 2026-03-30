package hooks

import (
	"os"
	"testing"
)

// shortTempDir creates a temp directory under /tmp with a short path.
// macOS temp dirs from t.TempDir() can exceed the 104-byte Unix socket
// path limit, causing socket creation failures.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ah")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
