package theme

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

//go:embed themes/*.toml
var embeddedThemes embed.FS

type Colors struct {
	Background       string
	Foreground       string
	BackgroundAlt    string
	Comment          string
	CurrentLine      string
	Selection        string
	LineNumber       string
	LineNumberActive string

	Red    string
	Orange string
	Yellow string
	Green  string
	Cyan   string
	Blue   string
	Purple string

	Link       string
	LinkHover  string
	CodeBg     string
	CodeFg     string
	BorderColor string
}

type Theme struct {
	Name   string
	Colors Colors
}

func Load(themeName string) (*Theme, error) {
	themePath := fmt.Sprintf("themes/%s.toml", themeName)

	data, err := embeddedThemes.ReadFile(themePath)
	if err != nil {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf("theme not found in embedded themes and cannot access home directory: %w", err)
		}

		customPath := filepath.Join(homeDir, ".config", "stigmergic", "themes", fmt.Sprintf("%s.toml", themeName))
		data, err = os.ReadFile(customPath)
		if err != nil {
			return nil, fmt.Errorf("theme '%s' not found in embedded or custom themes: %w", themeName, err)
		}
	}

	v := viper.New()
	v.SetConfigType("toml")

	if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("failed to parse theme file: %w", err)
	}

	theme := &Theme{
		Name: themeName,
	}

	if err := v.Unmarshal(&theme.Colors); err != nil {
		return nil, fmt.Errorf("failed to unmarshal theme colors: %w", err)
	}

	return theme, nil
}
