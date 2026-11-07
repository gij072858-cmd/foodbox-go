package app

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshots writes the milestone evidence the plan asks for. It is a
// test so the images are regenerated with the code, and it asserts only
// that rendering produced a frame.
// screenshotApp is the test app with the nutrition catalog read, so the
// evidence frames show the recipe rows' per-serving column and the detail
// hero rather than the "catalog loading" line a cold start flashes past.
func screenshotApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp()
	if err := a.loadNutritionState(context.Background(), nil); err != nil {
		t.Fatalf("load nutrition catalog: %v", err)
	}
	return a
}

func TestScreenshots(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := screenshotApp(t)
	tt := ui.NewTester(a.View, 1280, 860)
	// Capture the settled page: the first frame is mid-entrance.
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m2-pantry.png"))

	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m3-recipes.png"))

	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m3-recipe-detail.png"))

	if err := tt.Click("加入掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m4-schedule.png"))

	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m1-settings.png"))

	// The dark palette is a first-class appearance, not an afterthought:
	// keep one frame of it beside the light ones.
	if err := tt.Click("仓库"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	tt.SetDark(true)
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m2-pantry-dark.png"))

	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m3-recipes-dark.png"))

	tt.SetDark(false)
	settle(tt)
	if err := tt.Click("仓库"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("土豆"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m2-pantry-item.png"))
}

// TestScreenshotsSmall captures the same pages at the minimum window, where
// the recipe detail actually scrolls and the mini header appears.
func TestScreenshotsSmall(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := screenshotApp(t)
	tt := ui.NewTester(a.View, 1024, 700)
	settle(tt)
	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	tt.Scroll(600, 400, 0, 400)
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m3-recipe-detail-scrolled.png"))
}

// TestToastScreenshot proves the framework's toast host is the feedback
// path: set a notice and the next frame shows it at the window's bottom.
func TestToastScreenshot(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	a.notice = "已保存「青椒肉丝」"
	tt.Frame()
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m6-toast.png"))
}

func capture(t *testing.T, tt *ui.Tester, path string) {
	t.Helper()
	img := tt.Image()
	if img == nil || img.Bounds().Dx() == 0 {
		t.Fatalf("no frame for %s", path)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
