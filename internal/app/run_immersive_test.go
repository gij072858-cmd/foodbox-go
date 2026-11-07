package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// TestRunScreenIsImmersive: with the fire lit the cook is at the stove, so
// the run screen takes the whole window. The original does this by routing
// schedule/run above the shell (app/lib/core/app_router.dart:296-301); here
// the root simply leaves the sidebar and the page title out.
func TestRunScreenIsImmersive(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)

	if !a.sched.runActive {
		t.Fatal("the run did not start")
	}
	if a.pagePath != "/schedule/run" {
		t.Fatalf("the run did not open its own page: %q", a.pagePath)
	}
	for _, gone := range []string{"仓库", "菜式", "私厨", "离线 · 数据在本机"} {
		if tt.HasText(gone) {
			t.Errorf("the run screen still shows %q; it should own the whole window", gone)
		}
	}
	if !tt.HasText("退出这桌") {
		t.Errorf("the immersive screen has no way out: %q", tt.Texts())
	}

	if err := tt.Click("退出这桌"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sched.runActive {
		t.Error("退出这桌 did not end the run")
	}
	if !tt.HasText("仓库") {
		t.Errorf("the shell did not come back: %q", tt.Texts())
	}
}

// TestDragStepToTrayHangsIt is the mobile FoodBox gesture carried over: a
// step is lifted by its handle and dropped on the band above the flow, and
// the drop hangs a countdown for that step. Hanging twice is one hanging,
// and a hands-on step refuses the band — both as the original validates them
// (app/lib/features/schedule/schedule_run_page.dart:1274-1285).
func TestDragStepToTrayHangsIt(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	wait := waitingLineIndex(a)
	if wait < 0 {
		t.Fatal("the fixture has no waiting step")
	}
	a.runStepTo(wait)
	tt.Frame()
	settle(tt)

	handle, ok := tt.Find(fmt.Sprintf("把第 %d 步挂起来计时", wait+1))
	if !ok {
		t.Fatalf("the step offers no handle to hang it with: %q", tt.Texts())
	}
	hangByDrag(tt, handle)

	if len(a.sched.hang) != 1 {
		t.Fatalf("hanging by drag gave %+v, want one countdown", a.sched.hang)
	}
	if a.sched.hang[0].Line != wait {
		t.Errorf("hung step = %d, want the step that was dragged (%d)", a.sched.hang[0].Line, wait)
	}
	if !tt.HasText("挂着 1 步计时") {
		t.Errorf("the tray does not show it: %q", tt.Texts())
	}

	// The same step again is refused, so a second drop cannot double it.
	again, ok := tt.Find(fmt.Sprintf("把第 %d 步挂起来计时", wait+1))
	if !ok {
		t.Fatal("the handle vanished once hung")
	}
	hangByDrag(tt, again)
	settle(tt)
	if len(a.sched.hang) != 1 {
		t.Errorf("dropping the same step twice made %d countdowns", len(a.sched.hang))
	}

	// A step the cook must stand at gets no handle: only the current wait
	// step rides a hang pill.
	if _, ok := tt.Find("把第 1 步挂起来计时"); ok {
		t.Error("a hands-on step still offers a hang handle")
	}
}

// hangByDrag carries a step by its handle into the band and lets go there.
// The band is only live from the frame the drag starts in, so the pointer
// has to be given that frame before it goes on, exactly as a hand moving
// over the screen does.
func hangByDrag(tt *ui.Tester, handle ui.Rect) {
	const bandX = 600
	y := float32(design.TitleBand) + 60
	tt.Press(handle.X+handle.W/2, handle.Y+handle.H/2)
	tt.Move(handle.X+handle.W/2, handle.Y-12)
	tt.Frame()
	tt.Move(bandX, y)
	tt.Frame()
	tt.Release(bandX, y)
	tt.Frame()
}

// The tray survives a restart, in the order it was hung and with the seconds
// it still owes rather than the full length: the original FoodBox saved the
// timers but not the tray, so a restart came back to an empty list. A
// countdown that ran out while the app was closed stays visible, so the cook
// is told 时间到 instead of nothing at all.
func TestHangingSurvivesRestart(t *testing.T) {
	store := openM6Store(t)
	ctx := context.Background()

	a := m4TestApp()
	a.store = store
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if wait := waitingLineIndex(a); wait >= 0 {
		a.hangLine(wait)
	}
	if len(a.sched.hang) == 0 {
		t.Fatal("nothing was hung")
	}
	a.persistRunSession()
	settle(tt)

	// The write is on a goroutine; wait for it rather than race it.
	deadline := time.Now().Add(3 * time.Second)
	var loaded domain.RunSession
	for time.Now().Before(deadline) {
		s, ok, err := store.LoadRunSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ok && len(s.Hang) == len(a.sched.hang) {
			loaded = s
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(loaded.Hang) != len(a.sched.hang) {
		t.Fatalf("the tray never reached storage: %+v", loaded)
	}

	b := New()
	b.phase = PhaseReady
	if err := b.loadScheduleState(ctx, store); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the tray restored", func() bool {
		return len(b.sched.hang) == len(a.sched.hang)
	})
	for i, h := range b.sched.hang {
		if h.Line != loaded.Hang[i].Line || h.Title != loaded.Hang[i].Title {
			t.Errorf("slot %d came back as %d/%q, want %d/%q",
				i, h.Line, h.Title, loaded.Hang[i].Line, loaded.Hang[i].Title)
		}
	}
}
