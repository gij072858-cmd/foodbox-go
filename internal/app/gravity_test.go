package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestTiltedCardStaysClickable guards the one thing the gravity tilt could
// silently break: asking for the pointer makes an element interactive
// (ui/input.go:303), and an interactive element takes the press for itself.
// Tracking it from the cover inside a card button used to leave the card
// unclickable, so the tilt is driven from the card, which is interactive
// already.
func TestTiltedCardStaysClickable(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)

	name, ok := tt.Find("土豆")
	if !ok {
		t.Fatalf("the pantry card is missing: %q", tt.Texts())
	}
	// Hover the cover above the name, which is where the lean now follows
	// the pointer from.
	tt.Move(name.X+name.W/2, name.Y-30)
	tt.Advance(400 * time.Millisecond)

	if err := tt.Click("土豆"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("阴凉干燥处保存。") {
		t.Errorf("hovering the card took its click away: %q", tt.Texts())
	}
}

// TestTiltStopsAskingForFrames is the frame-budget half of the same change:
// a hovered card leans on a spring, and a spring at rest must not keep the
// page drawing.
func TestTiltStopsAskingForFrames(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)

	name, ok := tt.Find("番茄")
	if !ok {
		t.Fatalf("the pantry card is missing: %q", tt.Texts())
	}
	tt.Move(name.X+name.W/2, name.Y-30)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	tt.Advance(2 * time.Second)
	if tt.FramePending() {
		t.Error("a hovered card keeps asking for frames once its lean has settled")
	}
}
