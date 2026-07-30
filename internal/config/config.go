package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type AuthConfig struct {
	SessionSecret string   `mapstructure:"session_secret" json:"-"`
	SessionMaxAge string   `mapstructure:"session_max_age"`
	AllowedNpubs  []string `mapstructure:"allowed_npubs"`
	Enabled       bool
}

type Config struct {
	Host        string
	BaseURL     string `mapstructure:"base_url"`
	WatchPath   string
	LogLevel    string
	Theme       string
	DefaultFile string
	// AttachmentRoot is a second directory searched for an embedded
	// attachment named by bare filename, which is the dominant form in an
	// Obsidian vault. It is relative to the watch path. Empty means only
	// the watch path itself is searched.
	AttachmentRoot   string `mapstructure:"attachment_root"`
	IgnorePatterns   []string
	Auth             AuthConfig
	Port             int
	RecentFilesCount int
	RespectGitignore bool
}

func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	v.SetDefault("port", 8080)
	v.SetDefault("host", "localhost")
	v.SetDefault("loglevel", "ERROR")
	v.SetDefault("respectgitignore", true)
	v.SetDefault("theme", "iceberg-dark")
	v.SetDefault("recentfilescount", 5)
	v.SetDefault("base_url", "")
	v.SetDefault("defaultfile", "")
	v.SetDefault("attachment_root", "")
	v.SetDefault("ignorepatterns", []string{
		".git",
		"node_modules",
		".DS_Store",
		"*.swp",
		"*.swo",
		"*~",
		".vscode",
		".idea",
		"vendor",
		"dist",
		"build",
		"target",
		"__pycache__",
		"*.pyc",
	})

	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.allowed_npubs", []string{})
	v.SetDefault("auth.session_secret", "")
	v.SetDefault("auth.session_max_age", "24h")

	v.SetEnvPrefix("STIGMERGIC")
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		v.SetConfigName(".stigmergic")
		v.SetConfigType("toml")
		v.AddConfigPath(".")

		homeDir, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(filepath.Join(homeDir, ".config", "stigmergic"))
		}

		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			} else {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
