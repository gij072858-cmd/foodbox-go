package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Level is how much the app moves.
type Level int

const (
	MotionOff Level = iota
	MotionLite
	MotionFull
)

// The app moves on three clocks, borrowed from Cloudy's discipline: what
// follows the pointer springs; what comes over the app and what changes the
// view travel on the same curve, so nothing lands with speed left over.
var (
	Ease = ui.CubicBezier(0.22, 0.8, 0.24, 1)

	// Fast follows the pointer: hover washes, arrow turns.
	Fast = ui.Animation{Spring: ui.SpringOptions{Duration: 200 * time.Millisecond}}
	// Snappy is the lift and press of a surface under the pointer.
	Snappy = ui.Animation{Spring: ui.SpringOptions{Duration: 320 * time.Millisecond, Bounce: 0.12}}
	// Hover is a color wash; color needs no spring.
	Hover = ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}

	Pane = ui.Animation{Duration: 280 * time.Millisecond, Ease: Ease}
	Page = ui.Animation{Duration: 340 * time.Millisecond, Ease: Ease}
)

// Stagger brings the first screenful of a list in, one row behind the next.
// The cap matters: a long list that animates every row reads as slow.
const (
	StaggerRows = 14
	StaggerStep = 24 * time.Millisecond
	StaggerTime = 280 * time.Millisecond
)

// LevelFor returns how much to move, following the system's Reduce Motion
// and the setting ("full", "lite" or "off").
func LevelFor(c *ui.Context, setting string) Level {
	if c.Preferences().ReduceMotion || setting == "off" {
		return MotionOff
	}
	if setting == "lite" {
		return MotionLite
	}
	return MotionFull
}

// Moves reports whether the level animates hover and press at all.
func (l Level) Moves() bool { return l != MotionOff }

// Full reports whether the level may slide pages, fly elements and stagger
// lists. Lite keeps fades only.
func (l Level) Full() bool { return l == MotionFull }

// Apply settles the frame when motion is off. It must run before the view
// builds anything that animates.
func (l Level) Apply(c *ui.Context) {
	if l == MotionOff {
		c.DisableAnimations()
	}
}

// PageEnter moves a page as it arrives: 34 DIP in from the right going
// deeper, 24 back out the way it came, a 10 DIP rise across, fading from
// nothing, never playing an exit — two pages showing through each other
// read as neither. The values are Cloudy's pageMotion (ui_expand.go:196-215).
//
// This is time-driven rather than an ElementTransition.Enter because MyGo
// runs Enter only for an element born this frame under a parent that was
// not (ui/transition.go:446-458), while a Back or Forward reuses the page
// state the router kept for the history (ui/router.go:705-729), so Enter
// would silently not play on the way back. A time curve plays every time,
// and asks for frames only while it is still moving.
func PageEnter(c *ui.Context, level Level, at time.Time, dir int, page ui.Element) {
	if level == MotionOff || at.IsZero() {
		return
	}
	t := c.Now().Sub(at)
	if t >= Page.Duration {
		return
	}
	c.AnimationFrame()
	shown := Ease(float32(t) / float32(Page.Duration))
	left := 1 - shown
	if level.Full() {
		switch {
		case dir > 0:
			page.Translate(34*left, 0)
		case dir < 0:
			page.Translate(-24*left, 0)
		default:
			page.Translate(0, 10*left)
		}
	}
	page.Opacity(shown)
}
