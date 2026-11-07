// Copied from github.com/nya-owo/cloudy internal/app/ui_cover.go.
// Cloudy is AGPL-3.0-only, Copyright (c) EGOIST.
//
// The glare source (lines 51-77), its compile-once handle (77), the tilt
// maths (184-206) and the depth + glare drawing (223-247) are copied
// verbatim, with only these changes: the surface is a rectangle rather than
// a square, so the pointer is normalised by each axis on its own; the swing
// and the fling-to-next-track that go with Cloudy's swipe are left out; and
// where Cloudy reads its own player state, this takes a motion gate from the
// caller. Do not "tidy" the erf approximation or the sigma ratios — they are
// what makes the glare read as light rather than as a blob.
//
// FoodBox-Go is distributed under AGPL-3.0 because of this copy; see
// LICENSE.md.

package design

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// glareSource paints the glare as the shadow of a point, grown by Core
// and blurred by Sigma, would be: the blur of a square Core from Center
// each way. It rounds the cover's corners itself, where a clip would be
// rasterized anew each frame the cover tilts in depth.
const glareSource = `package main

var Center vec2
var Core float
var Sigma float
var Alpha float

// erf approximates the error function, within 0.0002.
func erf(x float) float {
	x2 := x * x
	return sign(x) * sqrt(1.0-exp(-x2*(1.2732395+0.147*x2)/(1.0+0.147*x2)))
}

// spread is how much of a line of length 2*Core around 0, blurred, is at d.
func spread(d float) float {
	k := 0.70710678 / Sigma
	return 0.5 * (erf((d+Core)*k) - erf((d-Core)*k))
}

func Fragment(dst vec4, pos vec2) vec4 {
	d := pos - Center
	a := Alpha * spread(d.x) * spread(d.y)
	return vec4(a, a, a, a)
}
`

var glareShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(glareSource) })

// How far the surface leans and how long it takes to get there: Cloudy's
// numbers (ui_cover.go:194-196). The shine fades on its own curve, apart
// from the lean.
var (
	tiltSpring  = ui.Animation{Spring: ui.SpringOptions{Duration: 500 * time.Millisecond, Bounce: 0.2}}
	shineSpring = ui.Animation{Duration: 300 * time.Millisecond}
)

const tiltDegrees = 7

// GlarePeak is Cloudy's strongest glare alpha over artwork (ui_cover.go:244).
// A surface that is a flat colour gets nothing out of white light over it,
// and would pay a shader pass to find that out, so its caller passes 0.
const GlarePeak = float32(0.26)

// Tilt turns box into a thing to touch: it leans toward the pointer in
// depth, and a glare follows the pointer over glare as light over a card
// held up. Call it after building box's own children — the glare is added
// on top of glare's own children.
//
// hit is the element the pointer is read from, and must not be the one being
// leaned. Asking a transformed element where the pointer is returns the
// answer through the transform it is about to cause, so the target moves as
// the answer moves, the spring never settles, and a hovered card draws
// frames forever. Cloudy keeps the same split between its wrap and its box
// (ui_cover.go:212-228); here a card hits and the cover inside it leans, or
// a stage hits and the card inside it leans.
//
// glare is where the light lands; it is usually box, but a whole card can
// lean while only its artwork catches the light. The zero Element draws
// none, and a flat-colour surface should pass 0 as peak for the same
// reason.
//
// Leaving the pointer needs no reset: over goes false, the target becomes
// flat, and the spring carries it back. That is why this is a spring.
//
// box must not be .Clip()ed — a clip under a 3D transform is re-rasterized
// on the CPU for every frame of the lean (Cloudy says so at ui_cover.go:48-50)
// — so radius rounds both the surface and the glare instead.
//
// allow is the app's motion gate: flat unless the page is on full motion.
// The system's own Reduce Motion is honoured here too, as Cloudy's does, so
// a caller that forgets the gate still cannot animate against it.
func Tilt(c *ui.Context, hit, box, glare ui.Element, radius, peak float32, allow bool) {
	hr := hit.LayoutBounds()
	br := box.LayoutBounds()
	px, py, over := hit.PointerPosition()
	tx, ty := float32(0), float32(0)
	if over && allow && br.W > 0 && br.H > 0 {
		// Where the pointer is inside the surface, which sits at its own
		// offset within the element that is hit.
		tx = clamp1((px-(br.X-hr.X))/br.W*2 - 1)
		ty = clamp1((py-(br.Y-hr.Y))/br.H*2 - 1)
	}
	if c.Preferences().ReduceMotion {
		tx, ty = 0, 0
	}
	ry := hit.AnimateTo("ry", tx*tiltDegrees, tiltSpring)
	rx := hit.AnimateTo("rx", -ty*tiltDegrees, tiltSpring)
	shine := float32(0)
	if over && allow {
		shine = 1
	}
	shine = hit.AnimateTo("shine", shine, shineSpring)
	// The glare's centre only follows the pointer while the pointer is
	// over. Cloudy animates it always, but it has one cover in a
	// full-screen view; here a page of virtualized cards would each spring
	// the centre from wherever the pointer happens to be — a long way off,
	// and asking for frames over a second after the page settled, to draw a
	// glare that is not drawn at all.
	if math.Abs(float64(rx))+math.Abs(float64(ry)) > 0.05 {
		// In depth only while tilted: flat, it draws as an image does.
		box.RotateX(rx).RotateY(ry).Perspective(900)
	}
	if peak > 0 && glare.Valid() {
		gr := glare.LayoutBounds()
		gx, gy := px, py
		if over {
			gx = hit.AnimateTo("gx", px-(gr.X-hr.X), tiltSpring)
			gy = hit.AnimateTo("gy", py-(gr.Y-hr.Y), tiltSpring)
		}
		glare.Children(func() {
			if shine <= 0.01 {
				return
			}
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
				glareShader().Draw(p, r, shader.Options{Radius: radius, Uniforms: map[string]any{
					"Center": [2]float32{gx, gy}, "Core": r.W * 0.16, "Sigma": r.W * 0.225, "Alpha": peak * shine,
				}})
			})
		})
	}
}

func clamp1(v float32) float32 { return max(-1, min(v, 1)) }
