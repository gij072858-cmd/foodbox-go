package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTitleBarIsDragRoomOnly covers the hidden title bar. MyGo draws the
// caption buttons itself and reports the room they take through
// ui.Context.TitleBar; the shell's top strip is drag room alone now, so
// nothing of the page may sit under them. The headless tester can report
// that room on demand, which makes the sidebar's clearance testable even
// though the buttons are not drawn here.
func TestTitleBarIsDragRoomOnly(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "菜式")

	if _, ok := tt.Find("窗口拖拽区"); !ok {
		t.Fatalf("the top strip has no drag region: %q", tt.Texts())
	}
	// The page's one action moved into the content, under the strip.
	if _, ok := tt.Find("新建菜式"); !ok {
		t.Fatalf("the page action is missing: %q", tt.Texts())
	}
	// The old top bar repeated the page's name and a one-line description;
	// both are gone from the first-level page.
	if tt.HasText("944 道菜，按主料、菜系、做法找") {
		t.Errorf("the root page still repeats its description: %q", tt.Texts())
	}
	// The brand is the Chinese name alone: no English caption rides under it.
	if tt.HasText("FOODBOX") {
		t.Errorf("the sidebar still carries the English wordmark: %q", tt.Texts())
	}

	// macOS puts its lights on the left, over the sidebar's head.
	brandRest := brandTop(t, tt, ui.TitleBar{})
	brandLeft := brandTop(t, tt, ui.TitleBar{Height: 46, Left: 80})
	if brandLeft <= brandRest {
		t.Errorf("the brand sits at y=%.0f with lights on the left, not below them: it was %.0f with none",
			brandLeft, brandRest)
	}
}

// brandTop is the top of the sidebar's wordmark with the window reporting
// this much room for its controls.
func brandTop(t *testing.T, tt *ui.Tester, bar ui.TitleBar) float32 {
	t.Helper()
	tt.SetTitleBar(bar)
	tt.Frame()
	r, ok := tt.Find("食匣")
	if !ok {
		t.Fatalf("the sidebar wordmark is missing: %q", tt.Texts())
	}
	return r.Y
}
