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

func TestLoadDefaultFileFromConfig(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	configContent := `defaultfile = "index.md"
`
	testutil.CreateTestFile(t, dir, ".stigmergic.toml", configContent)

	cfgPath := filepath.Join(dir, ".stigmergic.toml")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DefaultFile != "index.md" {
		t.Errorf("expected defaultfile index.md, got %s", cfg.DefaultFile)
	}
}

func TestLoadDefaultFileEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.DefaultFile != "" {
		t.Errorf("expected empty defaultfile by default, got %s", cfg.DefaultFile)
	}
}

func TestAuthConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Auth.Enabled {
		t.Error("expected auth disabled by default")
	}

	if cfg.Auth.SessionMaxAge != "24h" {
		t.Errorf("expected default session_max_age 24h, got %s", cfg.Auth.SessionMaxAge)
	}

	if cfg.Auth.SessionSecret != "" {
		t.Error("expected empty default session_secret")
	}

	if len(cfg.Auth.AllowedNpubs) != 0 {
		t.Errorf("expected empty default allowed_npubs, got %v", cfg.Auth.AllowedNpubs)
	}
}

func TestAuthConfigFromFile(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	configContent := `
[auth]
enabled = true
allowed_npubs = ["npub1ywfamkk565469kagtx4m2dmk5g0h7fk079tvdkncfltnau6czt6qm95e08", "npub10utunpt8274e80l4sa4tzarpr6sm9dp4qcsm3dey89ak9qxyc03sl9dnde"]
session_secret = "mysecret"
session_max_age = "12h"
`
	testutil.CreateTestFile(t, dir, ".stigmergic.toml", configContent)

	cfgPath := filepath.Join(dir, ".stigmergic.toml")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.Auth.Enabled {
		t.Error("expected auth enabled")
	}

	if len(cfg.Auth.AllowedNpubs) != 2 {
		t.Fatalf("expected 2 allowed pubkeys, got %d", len(cfg.Auth.AllowedNpubs))
	}

	if cfg.Auth.AllowedNpubs[0] != "npub1ywfamkk565469kagtx4m2dmk5g0h7fk079tvdkncfltnau6czt6qm95e08" {
		t.Errorf("expected first npub, got %s", cfg.Auth.AllowedNpubs[0])
	}

	if cfg.Auth.SessionSecret != "mysecret" {
		t.Errorf("expected session_secret mysecret, got %s", cfg.Auth.SessionSecret)
	}

	if cfg.Auth.SessionMaxAge != "12h" {
		t.Errorf("expected session_max_age 12h, got %s", cfg.Auth.SessionMaxAge)
	}
}

func TestAuthConfigAbsentIsNoOp(t *testing.T) {
	t.Parallel()

	dir := testutil.CreateTempDir(t)
	configContent := `port = 9000`
	testutil.CreateTestFile(t, dir, ".stigmergic.toml", configContent)

	cfgPath := filepath.Join(dir, ".stigmergic.toml")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Auth.Enabled {
		t.Error("expected auth disabled when [auth] section absent")
	}

	if cfg.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Port)
	}
}
