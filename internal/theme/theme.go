package theme

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	Link        string
	LinkHover   string
	CodeBg      string
	CodeFg      string
	BorderColor string
}

type Theme struct {
	Name   string
	Colors Colors

	// ChromaStyle names the chroma highlight style paired with this palette.
	ChromaStyle string
	// MermaidTheme names the mermaid built-in theme paired with this palette.
	MermaidTheme string
	// ChromaCSS is the generated chroma stylesheet, scoped under
	// [data-theme="Name"] so every theme's rules can coexist on one page.
	ChromaCSS string
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
		data, err = os.ReadFile(filepath.Clean(customPath))
		if err != nil {
			return nil, fmt.Errorf("theme '%s' not found in embedded or custom themes: %w", themeName, err)
		}
	}

	v := viper.New()
	v.SetConfigType("toml")

	if readErr := v.ReadConfig(bytes.NewReader(data)); readErr != nil {
		return nil, fmt.Errorf("failed to parse theme file: %w", readErr)
	}

	theme := &Theme{
		Name: themeName,
	}

	if unmarshalErr := v.Unmarshal(&theme.Colors); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal theme colors: %w", unmarshalErr)
	}

	theme.ChromaStyle = v.GetString("ChromaStyle")
	if theme.ChromaStyle == "" {
		theme.ChromaStyle = "nord"
	}
	theme.MermaidTheme = v.GetString("MermaidTheme")
	if theme.MermaidTheme == "" {
		theme.MermaidTheme = "dark"
	}

	css, err := scopedChromaCSS(theme.ChromaStyle, fmt.Sprintf("[data-theme=%q]", themeName))
	if err != nil {
		return nil, fmt.Errorf("theme %q: %w", themeName, err)
	}
	theme.ChromaCSS = css

	return theme, nil
}

// LoadEmbedded loads every theme shipped in the embedded themes directory,
// in directory order (lexicographic by filename).
func LoadEmbedded() ([]*Theme, error) {
	entries, err := embeddedThemes.ReadDir("themes")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded themes: %w", err)
	}

	themes := make([]*Theme, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".toml")
		t, err := Load(name)
		if err != nil {
			return nil, err
		}
		themes = append(themes, t)
	}
	return themes, nil
}
