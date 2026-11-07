package app

import (
	"bytes"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestPageSwitchUsesTheShellTransition locks the one way pages change: the
// router swaps at once and the arriving page carries the motion itself. The
// stock slide rebuilt the old page beside the new one every frame, which is
// what made a switch stutter.
func TestPageSwitchUsesTheShellTransition(t *testing.T) {
	a := newTestApp()
	withCatalog(t, a)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := a.router.Transition; got != ui.TransitionNone {
		t.Errorf("the router still animates page changes: %v", got)
	}
	settle(tt)
	if !tt.HasText("营养估算 · 每 1 人份") {
		t.Fatalf("the dish page did not arrive: %q", tt.Texts())
	}
}

// Back reuses the page state the router kept for the history, so an
// ElementTransition.Enter would not play. The time-driven page entrance has
// to move the page on the way back too, and then stop asking for frames.
func TestPageEnterRunsWhenGoingBack(t *testing.T) {
	a := newTestApp()
	withCatalog(t, a)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)

	a.router.Back()
	tt.Frame()
	first := tt.Image()
	tt.Advance(80 * time.Millisecond)
	second := tt.Image()
	if bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("the page did not move on the way back")
	}

	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("the back transition still asks for frames once it should have settled")
	}
	if !tt.HasText("新建菜式") {
		t.Errorf("the recipe list did not come back: %q", tt.Texts())
	}
}
