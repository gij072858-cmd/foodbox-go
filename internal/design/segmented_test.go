package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSegmentedSizesToItsLabels pins the one behaviour the wrapper exists
// for: MyGo lays a column's children out with Stretch across the cross
// axis, so a bare Segmented becomes a card-wide track with its choices
// pushed to one end. AlignSelf(Start) is what stops that.
//
// The check is that the track no longer follows the column, rather than an
// absolute width: any pixel bound would be a bet on the font's metrics.
func TestSegmentedSizesToItsLabels(t *testing.T) {
	// narrow and wide give the same control in columns twice as far apart.
	narrow, wide := measure(600), measure(1200)

	// Guards that the frame really exercises Stretch: if a bare Segmented
	// were already content-sized, the assertions below would pass for the
	// wrong reason.
	if narrow.bare < 590 || wide.bare < 1190 {
		t.Fatalf("a bare Segmented did not fill its column: %.0f at 600, %.0f at 1200", narrow.bare, wide.bare)
	}
	if wrapped := narrow.wrapped; wrapped > wide.wrapped+1 {
		t.Errorf("the wrapped Segmented changed width with the column: %.0f then %.0f", wrapped, wide.wrapped)
	}
	if wide.wrapped >= wide.bare {
		t.Errorf("the wrapped Segmented is %.0f wide in a %.0f column, no narrower than the bare one", wide.wrapped, wide.bare)
	}
}

// A disabled choice is greyed and takes neither pointer nor keyboard input.
func TestSegmentedDisabledCannotBeChosen(t *testing.T) {
	selected := 0
	tt := ui.NewTester(func(c *ui.Context) {
		SegmentedDisabled(c, &selected, []bool{false, true, false}, "甲", "乙", "丙")
	}, 400, 120)
	tt.Frame()
	if err := tt.Click("乙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if selected != 0 {
		t.Fatalf("a disabled segment was chosen: %d", selected)
	}
	if err := tt.Click("丙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if selected != 2 {
		t.Fatalf("an enabled segment did not take the choice: %d", selected)
	}
}

type segmentedWidths struct {
	bare    float32
	wrapped float32
}

// measure lays both controls in a column of the given width and returns
// their track widths, read through the accessibility label because neither
// track draws its own text.
func measure(column float32) segmentedWidths {
	sel := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(column).Gap(20).Children(func() {
			ui.Segmented(c, &sel, "拉伸甲", "拉伸乙").Label("track-bare")
			Segmented(c, &sel, "自适应甲", "自适应乙").Label("track-wrapped")
		})
	}, 1400, 400)
	bare, ok := tt.Find("track-bare")
	if !ok {
		panic("a segmented control is not findable by its label")
	}
	wrapped, ok := tt.Find("track-wrapped")
	if !ok {
		panic("the wrapped segmented control is not findable by its label")
	}
	return segmentedWidths{bare: bare.W, wrapped: wrapped.W}
}
