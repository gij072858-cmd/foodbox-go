package design

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestTiltLeansTheWholeBox pins the pantry card's lean: the pointer is read
// from a stage outside the card, and the card itself must turn — not only
// the artwork inside it. Bounds is the previous frame's transformed
// rectangle, so it grows once the card leans while LayoutBounds stays put.
func TestTiltLeansTheWholeBox(t *testing.T) {
	const w, h = 400, 240
	var bounds, layout ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		stage := ui.Column(c).Padding(20).Width(w + 40)
		stage.Children(func() {
			card := ui.ButtonBase(c.Key("tilt-card")).Column().Width(w).Height(h)
			card.Children(func() {
				cover := ui.Box(c).FillWidth().Height(h / 2)
				cover.Children(func() {
					ui.Box(c).Fill()
				})
				Tilt(c, stage, card, cover, 8, GlarePeak, true)
			})
			bounds, layout = card.Bounds(), card.LayoutBounds()
		})
	}, 600, 400)

	tt.Frame()
	tt.Frame()
	if layout.W != w || layout.H != h {
		t.Fatalf("the card was not laid out at %dx%d: %+v", w, h, layout)
	}
	if bounds.W != w || bounds.H != h {
		t.Fatalf("a resting card is already transformed: %+v", bounds)
	}

	// Bottom-right of the card: both axes lean their hardest.
	tt.Move(w, h)
	tt.Advance(700 * time.Millisecond)

	if layout.W != w || layout.H != h {
		t.Fatalf("the lean moved the layout box: %+v", layout)
	}
	if bounds.W <= w || bounds.H <= h {
		t.Errorf("the card itself did not lean, only its artwork: %+v", bounds)
	}
}
