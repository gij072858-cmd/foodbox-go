package app

// The M5 dashboard's screenshot evidence. It is a test so the image is
// regenerated with the code, and it asserts only that rendering produced a
// frame with content.

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// nutritionShotTime is the fixed clock the screenshot uses, so the image and
// its date labels do not change with the day the tests run.
var nutritionShotTime = time.Date(2026, 11, 5, 20, 0, 0, 0, time.Local)

// TestNutritionScreenshot writes test/screenshots/m5-nutrition.png with a
// week of records and a filled body profile.
func TestNutritionScreenshot(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	records := nutritionTestRecordsAt(nutritionShotTime)
	a := newNutritionAppAt(t, records, nutritionTestProfile(), true, nutritionShotTime)
	// A taller viewport so one frame carries the whole dashboard: target card,
	// macro meters, the seven-day chart, purine load and the contributor list.
	tt := ui.NewTester(a.View, 1280, 1700)
	settle(tt)
	if !tt.HasText("食材贡献") {
		t.Fatalf("dashboard content missing: %q", tt.Texts())
	}
	img := tt.Image()
	if img == nil || img.Bounds().Dx() == 0 {
		t.Fatal("no frame for the nutrition dashboard")
	}
	f, err := os.Create(filepath.Join(dir, "m5-nutrition.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
