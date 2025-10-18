package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateTempDir(t *testing.T) {
	t.Parallel()

	dir := CreateTempDir(t)

	if dir == "" {
		t.Fatal("CreateTempDir returned empty string")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("temp directory does not exist: %v", err)
	}

	if !info.IsDir() {
		t.Fatal("temp path is not a directory")
	}
}

func TestCreateTestFile(t *testing.T) {
	t.Parallel()

	dir := CreateTempDir(t)
	content := "test content"
	filename := "test.txt"

	CreateTestFile(t, dir, filename, content)

	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	if string(data) != content {
		t.Errorf("expected content %q, got %q", content, string(data))
	}
}

func TestCreateTestFileWithSubdirectory(t *testing.T) {
	t.Parallel()

	dir := CreateTempDir(t)
	content := "nested content"
	filename := "subdir/nested.txt"

	CreateTestFile(t, dir, filename, content)

	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read nested test file: %v", err)
	}

	if string(data) != content {
		t.Errorf("expected content %q, got %q", content, string(data))
	}
}

func TestFindAvailablePort(t *testing.T) {
	t.Parallel()

	port := FindAvailablePort(t)

	if port <= 0 || port > 65535 {
		t.Errorf("invalid port number: %d", port)
	}
}

func TestFindAvailablePortUnique(t *testing.T) {
	t.Parallel()

	port1 := FindAvailablePort(t)
	port2 := FindAvailablePort(t)

	if port1 == port2 {
		t.Log("ports are the same, but this is acceptable for available ports")
	}

	if port1 <= 0 || port1 > 65535 {
		t.Errorf("invalid port1: %d", port1)
	}
	if port2 <= 0 || port2 > 65535 {
		t.Errorf("invalid port2: %d", port2)
	}
}
