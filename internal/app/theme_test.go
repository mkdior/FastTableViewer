package app

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSetTheme(t *testing.T) {
	defer func() { _ = setTheme(defaultThemeName) }()

	if err := setTheme("no-such-theme"); err == nil {
		t.Fatal("expected an error for an unknown theme")
	} else if !strings.Contains(err.Error(), defaultThemeName) {
		t.Errorf("error should list the available themes, got: %v", err)
	}
	if err := setTheme(defaultThemeName); err != nil {
		t.Fatalf("default theme must be selectable: %v", err)
	}
	if theme != builtinThemes[defaultThemeName] {
		t.Error("setTheme did not activate the requested theme")
	}
}

func TestThemeTag(t *testing.T) {
	th := Theme{Accent: tcell.PaletteColor(101), Text: tcell.NewHexColor(0xd0d0d0)}
	if got := th.tag(th.Accent); got != "[#87875f]" {
		t.Errorf("palette colour 101 tag = %q, want [#87875f]", got)
	}
	if got := th.tag(th.Text); got != "[#d0d0d0]" {
		t.Errorf("true colour tag = %q, want [#d0d0d0]", got)
	}
	if got := th.tag(tcell.ColorDefault); got != "[-]" {
		t.Errorf("default colour must reset, got %q", got)
	}
}
