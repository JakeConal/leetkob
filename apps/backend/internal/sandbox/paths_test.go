package sandbox

import (
	"path/filepath"
	"runtime"
	"testing"
)

func sandboxRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "sandbox"))
}

func sandboxDocPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(sandboxRoot(t), "README.md")
}

func dockerfilePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(sandboxRoot(t), "cpp17", "Dockerfile")
}

func attackPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(sandboxRoot(t), "attacks", name)
}
