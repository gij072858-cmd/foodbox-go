package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsKeroThemes writes the Kero-theme evidence: the settings
// picker, a light dusk pantry and a dark meadow recipe list. Theme and
// appearance changes animate their colors, so each shot settles first.
func TestScreenshotsKeroThemes(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := m6Fixture(t)
	withCatalog(t, a)
	tt := ui.NewTester(a.View, 1280, 860)

	clickText(t, tt, "设置")
	capture(t, tt, filepath.Join(dir, "kero-picker.png"))

	a.themeName = "dusk"
	a.theme = "light"
	clickText(t, tt, "仓库")
	capture(t, tt, filepath.Join(dir, "kero-dusk-pantry.png"))

	a.themeName = "meadow"
	a.theme = "dark"
	clickText(t, tt, "菜式")
	capture(t, tt, filepath.Join(dir, "kero-meadow-dark.png"))
}
