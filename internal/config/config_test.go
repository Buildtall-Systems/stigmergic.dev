package config

import (
	"path/filepath"
	"testing"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/testutil"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected default host localhost, got %s", cfg.Host)
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	configContent := `port = 9000
host = "0.0.0.0"
`
	testutil.CreateTestFile(t, dir, ".stigmergic.toml", configContent)

	cfgPath := filepath.Join(dir, ".stigmergic.toml")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Port)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
}

func TestLoadEnvironmentVariables(t *testing.T) {
	t.Setenv("STIGMERGIC_PORT", "7000")
	t.Setenv("STIGMERGIC_HOST", "127.0.0.1")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 7000 {
		t.Errorf("expected port 7000 from env, got %d", cfg.Port)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1 from env, got %s", cfg.Host)
	}
}

func TestLoadPrecedence(t *testing.T) {
	dir := testutil.CreateTempDir(t)
	configContent := `port = 9000
host = "0.0.0.0"
`
	testutil.CreateTestFile(t, dir, ".stigmergic.toml", configContent)

	t.Setenv("STIGMERGIC_PORT", "7000")

	cfgPath := filepath.Join(dir, ".stigmergic.toml")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 7000 {
		t.Errorf("expected port 7000 from env (higher precedence), got %d", cfg.Port)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0 from file, got %s", cfg.Host)
	}
}

func TestLoadInvalidFile(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	invalidContent := `invalid toml content [[[`
	testutil.CreateTestFile(t, dir, ".stigmergic.toml", invalidContent)

	cfgPath := filepath.Join(dir, ".stigmergic.toml")
	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid config file, got nil")
	}
}

func TestLoadMissingExplicitFileErrors(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	nonExistentPath := filepath.Join(dir, "nonexistent.toml")

	_, err := Load(nonExistentPath)
	if err == nil {
		t.Fatal("expected error for missing explicit config file, got nil")
	}
}

func TestLoadNoFileUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with no file should use defaults: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected default host localhost, got %s", cfg.Host)
	}
}
