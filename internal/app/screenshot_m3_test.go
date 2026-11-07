package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsM3 writes the milestone's two pieces of evidence: the
// editor filled from a seeded dish, and a detail page showing its version
// group. It is a test so the images are regenerated with the code.
func TestScreenshotsM3(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newShowcaseApp(t)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	// A dish with two versions: the detail page lists and switches them.
	if err := tt.Click("口水鸡（家常版）"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("2 种做法") {
		t.Fatalf("version section missing from the screenshot frame: %q", tt.Texts())
	}
	capture(t, tt, filepath.Join(dir, "m3-versions.png"))

	// The editor, opened from the detail page so every field is filled.
	if err := tt.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("编辑菜式") {
		t.Fatalf("editor missing from the screenshot frame: %q", tt.Texts())
	}
	capture(t, tt, filepath.Join(dir, "m3-editor.png"))
}
