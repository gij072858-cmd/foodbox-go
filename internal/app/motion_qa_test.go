package app

import (
	"bytes"
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
)

// accentY is the average y of the sidebar pixels painted in the accent
// color: the nav pill. It returns 0 when the pill is not there.
func accentY(img *image.RGBA, accent ui.Color) float32 {
	if img == nil {
		return 0
	}
	var sum, n float32
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < 232 && x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if abs8(uint8(r>>8), accent.R) < 10 &&
				abs8(uint8(g>>8), accent.G) < 10 &&
				abs8(uint8(bl>>8), accent.B) < 10 {
				sum += float32(y)
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func abs8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// The nav pill slides between the item chosen before and the one chosen
// now, using Match, rather than blinking from one to the other.
func TestNavPillSlides(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	accent := design.PaletteFor(design.DefaultTheme, false).Accent
	start := accentY(tt.Image(), accent)
	if start == 0 {
		t.Fatal("nav pill not found before the click")
	}
	if err := tt.Click("我的"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(60 * time.Millisecond)
	first := accentY(tt.Image(), accent)
	tt.Advance(90 * time.Millisecond)
	second := accentY(tt.Image(), accent)
	if first == 0 || second == 0 {
		t.Fatalf("nav pill vanished mid-flight: %v -> %v", first, second)
	}
	if math.Abs(float64(second-first)) < 8 {
		t.Errorf("nav pill did not move: %.1f -> %.1f", first, second)
	}
	tt.Advance(time.Second)
	end := accentY(tt.Image(), accent)
	if end <= start+40 {
		t.Errorf("nav pill did not land lower: start %.1f end %.1f", start, end)
	}
}

// Idle pages must stop asking for frames once their entrance is done. This
// is the budget rule Cloudy keeps: a still screen costs nothing.
func TestIdlePagesDoNotAskForFrames(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	for _, page := range []string{"仓库", "菜式", "私厨", "食历", "营养", "我的", "设置", "帮助"} {
		if err := tt.Click(page); err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		tt.Advance(1200 * time.Millisecond)
		if tt.FramePending() {
			t.Errorf("%s still asks for frames when idle", page)
		}
	}
}

// With motion off, a page change settles in the same frame: no slide, no
// stagger, no chart growth.
func TestMotionOffSettlesAtOnce(t *testing.T) {
	a := newTestApp()
	a.motion = "off"
	tt := ui.NewTester(a.View, 1280, 860)
	tt.Frame()
	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.FramePending() {
		t.Error("motion off still asked for a frame after the click")
	}
	if !tt.HasText("新建菜式") {
		t.Errorf("page did not change with motion off: %q", tt.Texts())
	}
}

// A page transition moves for a moment and then holds still; a transition
// that keeps painting is the bug this guards.
func TestPageTransitionMovesThenStops(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(80 * time.Millisecond)
	first := tt.Image()
	tt.Advance(80 * time.Millisecond)
	second := tt.Image()
	if bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("page did not move during its transition")
	}
	tt.Advance(1200 * time.Millisecond)
	settled := tt.Image()
	tt.Advance(100 * time.Millisecond)
	later := tt.Image()
	if !bytes.Equal(settled.Pix, later.Pix) {
		t.Error("page still moving after it should have settled")
	}
}
