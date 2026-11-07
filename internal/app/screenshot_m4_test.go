package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsM4 writes M4's three evidence frames: the prep list, the
// parallel timeline and the run page. It asserts only that rendering produced
// a frame; the behaviour is covered by m4_schedule_test.go.
func TestScreenshotsM4(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "screenshots")
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}

	if err := tt.Click("备料清单"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m4-prep.png"))

	if err := tt.Click("时间线"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m4-timeline.png"))

	if err := tt.Click("执行"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	// Show the hanging tray: jump to the braise step, hang its countdown, and
	// backdate the session so the floating clock reads like a real run.
	if wait := waitingLineIndex(a); wait >= 0 {
		a.runStepTo(wait)
		tt.Frame()
		settle(tt)
	}
	if err := tt.Click("挂起"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	a.sched.runStart = time.Now().Add(-95 * time.Second).Unix()
	tt.Advance(time.Second)
	settle(tt)
	capture(t, tt, filepath.Join(dir, "m4-run.png"))
}
