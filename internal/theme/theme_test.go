package theme

import (
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	themes, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error: %v", err)
	}

	names := make(map[string]*Theme, len(themes))
	for _, thm := range themes {
		names[thm.Name] = thm
	}

	for _, want := range []string{"iceberg-dark", "iceberg-light"} {
		if _, ok := names[want]; !ok {
			t.Errorf("LoadEmbedded() missing theme %q", want)
		}
	}
}

func TestLoadEmbeddedGeneratesScopedChromaCSS(t *testing.T) {
	themes, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error: %v", err)
	}

	for _, thm := range themes {
		if thm.ChromaStyle == "" {
			t.Errorf("theme %q has empty ChromaStyle", thm.Name)
		}
		if thm.MermaidTheme == "" {
			t.Errorf("theme %q has empty MermaidTheme", thm.Name)
		}
		scope := `[data-theme="` + thm.Name + `"]`
		if !strings.Contains(thm.ChromaCSS, scope+" .chroma") {
			t.Errorf("theme %q ChromaCSS not scoped under %s", thm.Name, scope)
		}
		if strings.Contains(thm.ChromaCSS, "\n.chroma") {
			t.Errorf("theme %q ChromaCSS contains unscoped .chroma rule", thm.Name)
		}
	}
}

func TestLoadUnknownThemeFails(t *testing.T) {
	if _, err := Load("no-such-theme"); err == nil {
		t.Fatal("Load(no-such-theme) succeeded, want error")
	}
}

func TestScopedChromaCSSUnknownStyleFails(t *testing.T) {
	if _, err := scopedChromaCSS("no-such-style", "[data-theme=\"x\"]"); err == nil {
		t.Fatal("scopedChromaCSS(no-such-style) succeeded, want error")
	}
}
