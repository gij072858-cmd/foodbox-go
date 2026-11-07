// Copied from github.com/nya-owo/cloudy internal/app/ui_expand.go (with one
// helper from ui_stage.go).
// Cloudy is AGPL-3.0-only, Copyright (c) EGOIST.
//
// What is copied verbatim: lerp (ui_stage.go:46), lerpRect
// (ui_expand.go:50-53), or14 (390-395), coverFlightTime and
// coverFlightWait (310-313), the shape of coverFlight (262-270) and
// cardOrigin (272-278), flyFrom (280-286), landCover (328-343) and the
// k-curve and drawing of coverFlightView (345-388).
//
// What is different, and why it has to be:
//   - Cloudy keys a flight by an album's picture URL and takes the bitmap
//     out of its own artwork cache. Here the thing flying is named by a key
//     the caller picks and its surface is drawn by a callback, because a
//     pantry cover is a bitmap while a dish's is a coloured mark.
//   - heldCover, which keeps an album's art on screen while the page loads
//     it from the network, has no counterpart: this app's images come off
//     the disk.
//   - The way back starts from what Cloudy calls headAt, the rect the
//     page's own cover reports. That is kept here as landed, refreshed by
//     LandCover, rather than read from a header that scrolls away — this
//     detail page's sticky header has different numbers.
//   - easeRange, flightEnd and flightTouch are not copied: they belong to
//     Cloudy's growing Now-Playing layer, which this is not.
//
// FoodBox-Go is distributed under AGPL-3.0 because of this copy; see
// LICENSE.md.

package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

// lerpRect is the rectangle k of the way from a to b.
func lerpRect(a, b ui.Rect, k float32) ui.Rect {
	return ui.Rect{X: lerp(a.X, b.X, k), Y: lerp(a.Y, b.Y, k), W: lerp(a.W, b.W, k), H: lerp(a.H, b.H, k)}
}

// How long a cover takes, and how long it waits for a place to land: a page
// without one, as a podcast's, leaves its cover where it is.
const (
	coverFlightTime = 460 * time.Millisecond
	coverFlightWait = 300 * time.Millisecond
)

// A flight is a cover on its way from a card to the page it opens, or back.
// It is drawn over the window, for a Match doesn't fly to an element of a
// new page.
type flight struct {
	key        any
	from, to   ui.Rect
	rFrom, rTo float32   // its corners there, 14 if unset
	back       bool      // it is going back to its card
	asked      time.Time // when it was clicked
	at         time.Time // when its way began, the page laid out
	here       bool      // this frame's page has a place for it
}

// A cardOrigin is the card a page was opened from, for going back to it.
type cardOrigin struct {
	path   string
	key    any
	from   ui.Rect // where the card's cover was, the way back lands on it
	y      float32 // how far its page was scrolled
	radius float32
}

// Flight carries one cover between a card and the page it opens. The App
// owns one; the pages tell it what left and what it lands on, and it is
// drawn over everything else once per frame.
type Flight struct {
	flight   flight
	cameFrom cardOrigin
	// landed is where the page's own cover sits, kept up to date by
	// LandCover, so the way back has somewhere to start from.
	landed       ui.Rect
	landedRadius float32
	// RestoreY is how far the list was scrolled when its card was opened.
	RestoreY float32
}

// FlyFrom sends the cover named by key from its card, at from, to the page
// about to open at path. allow is the app's motion gate: without full
// motion the cover stays where it is and the page only changes.
func (f *Flight) FlyFrom(c *ui.Context, path string, key any, from ui.Rect, radius, scrolled float32, allow bool) {
	f.cameFrom = cardOrigin{path: path, key: key, from: from, y: scrolled, radius: radius}
	if allow {
		f.flight = flight{key: key, from: from, rFrom: radius, asked: c.Now()}
	}
}

// FlyBack sends the cover of the page being left back to the card that
// opened it. It does nothing unless path is the page that card led to.
func (f *Flight) FlyBack(path string, now time.Time, allow bool) {
	o := f.cameFrom
	if o.path != path || o.key == nil {
		return
	}
	f.cameFrom, f.RestoreY = cardOrigin{}, o.y
	if allow && f.landed.W > 0 {
		f.flight = flight{key: o.key, from: f.landed, to: o.from,
			rFrom: f.landedRadius, rTo: o.radius, back: true, at: now, here: true}
	}
}

// LandCover gives the flight of a cover, if there is one, where it lands: e,
// wherever e is by now, which it hides until it has.
func (f *Flight) LandCover(c *ui.Context, e ui.Element) {
	if f.flight.key == nil {
		return
	}
	to := e.LayoutBounds()
	if to.W <= 0 {
		return
	}
	if f.flight.to.W == 0 {
		f.flight.at = c.Now()
	}
	f.flight.to = to
	f.flight.here = true
	f.landed, f.landedRadius = to, or14(f.flight.rTo)
	e.Opacity(0)
}

// Landing reports whether a cover is on its way, so a route's own slide can
// stand aside while it flies: Cloudy suppresses the page motion for exactly
// this reason (ui_expand.go:196-214).
func (f *Flight) Landing() bool { return f.flight.key != nil }

// Key is what the cover flying is: a page's own element calls LandCover only
// for the thing it holds.
func (f *Flight) Key() any { return f.flight.key }

// ReturningTo reports whether the cover of key is on its way back to the
// card it came from. That card must then LandCover it every frame, as the
// page opening does: a flight that finds nowhere to land is taken for a page
// that has gone, and dropped.
func (f *Flight) ReturningTo(key any) bool {
	return f.flight.back && f.flight.key == key
}

// Draw paints the cover on its way, if one is. It paints the frame it lands
// on too, which still hides what it lands on.
//
// shade is the colour of a card's shadow, and surface what to draw inside
// the flying box at the corner it has at that moment — a pantry item's
// bitmap, a dish's coloured mark.
func (f *Flight) Draw(c *ui.Context, shade ui.Color, surface func(c *ui.Context, corner float32)) {
	ly := &f.flight
	if ly.key == nil {
		return
	}
	c.AnimationFrame()
	now := c.Now()
	if ly.asked.IsZero() {
		ly.asked = now
	}
	k, shown, done := float32(0), float32(1), false
	switch {
	case ly.to.W > 0 && !ly.here:
		// Its page has gone.
		*ly = flight{}
		return
	case ly.to.W > 0:
		t := min(float32(now.Sub(ly.at))/float32(coverFlightTime), 1)
		k, done = 1-(1-t)*(1-t)*(1-t), t == 1
	case !ly.here:
		// Nowhere to land yet: it fades where it was, if that stays so.
		t := min(float32(now.Sub(ly.asked))/float32(coverFlightWait), 1)
		shown, done = 1-t, t == 1
	}
	ly.here = false
	r := lerpRect(ly.from, ly.to, k)
	if ly.to.W == 0 {
		r = ly.from
	}
	corner := lerp(or14(ly.rFrom), or14(ly.rTo), k)
	box := ui.Box(c.Key("cover-flight")).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
		Radius(corner).PassThrough().Opacity(shown).
		Shadow(0, lerp(12, 18, k), lerp(28, 40, k), lerp(-6, -10, k), shade)
	box.Children(func() { surface(c, corner) })
	if done {
		*ly = flight{}
	}
}

func or14(r float32) float32 {
	if r == 0 {
		return 14
	}
	return r
}
