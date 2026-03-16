package testutil

import (
	"context"
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
	if err := os.MkdirAll(dirPath, 0750); err != nil {
		t.Fatalf("failed to create directory %s: %v", dirPath, err)
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

func FindAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Logf("failed to close listener: %v", err)
		}
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("listener address is not TCP")
	}
	return addr.Port
}
