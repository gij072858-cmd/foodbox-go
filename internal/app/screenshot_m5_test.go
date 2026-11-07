package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestM5Screenshots writes the private-kitchen evidence: the two-column
// workbench, the in-page generated plan, the body profile and the AI settings.
func TestM5Screenshots(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)

	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m5-sichu.png"))

	if err := tt.Click("生成配餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m5-sichu-result.png"))

	a.router.Push("/sichu/body")
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m5-body.png"))

	a.router.Push("/settings/ai")
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m5-ai.png"))
}
