package testutil

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func CreateTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func CreateTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)

	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0755); err != nil { //nolint:gosec
		t.Fatalf("failed to create directory %s: %v", dirPath, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

func FindAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0") //nolint:noctx
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	return listener.Addr().(*net.TCPAddr).Port
}
