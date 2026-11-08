// Package design's component kit: the small set of surfaces every FoodBox
// page is built from, so the six sections stay visually identical without
// each page re-deriving padding, borders and type sizes.
//
// These helpers only draw; they never touch app state. A page adds children
// to a returned element and reads its own inputs (Clicked, Changed).
package design

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// PaneMaterial is the glass a floating panel sits on. It lets the dimmed
// content read through, so a dialog feels placed over the app rather than
// cut out of it. The tint comes from the palette, so a colored wash never
// greys out behind the glass.
func PaneMaterial(p Palette) ui.Material {
	if p.Dark {
		return glass.Glass{Tint: p.Bg.Alpha(0.50)}
	}
	return glass.Glass{Tint: p.Surface.Alpha(0.30)}
}

// BarMaterial is the glass of a bar that content scrolls under: a little
// more opaque than a pane, so its words stay readable.
func BarMaterial(p Palette) ui.Material {
	if p.Dark {
		return glass.Glass{Tint: p.Bg.Alpha(0.66)}
	}
	return glass.Glass{Tint: p.Surface.Alpha(0.56)}
}

// overlayTint is the veil behind dialogs, as Kero draws it.
func overlayTint(p Palette) ui.Color {
	if p.Dark {
		return ui.RGBA(0, 0, 0, 0.35)
	}
	return ui.RGBA(10, 10, 30, 0.12)
}

// Wash paints a surface's background, moving to hover while the pointer is
// over it. Color needs no spring; 140ms reads as a wash, not a flash.
func Wash(e ui.Element, rest, hover ui.Color) {
	target := rest
	if e.Hovered() {
		target = hover
	}
	e.Background(e.AnimateColor("wash", target, Hover))
}

// Lift raises a surface under the pointer and gives as it is pressed. Both
// are paint-only frames, so a list of cards stays cheap. The caller keeps
// its own rest shadow; this only moves.
func Lift(e ui.Element, lift, press float32) {
	pose := ui.Pose{}
	if e.Hovered() {
		pose.Y = -lift
		pose.Scale = 1 + lift*0.004
	}
	if e.Pressed() {
		pose.Scale = press
	}
	e.AnimatePose(pose, Snappy)
}

// RestShadow and HoverShadow are the two elevations a card may sit at.
func RestShadow(e ui.Element, p Palette)  { e.Shadow(0, 1, 4, -1, p.Shadow) }
func HoverShadow(e ui.Element, p Palette) { e.Shadow(0, 12, 28, -8, p.Shadow) }

// Card is FoodBox's standard surface: white, hairline border, soft shadow.
// Callers add padding, gaps and children.
func Card(c *ui.Context, p Palette) ui.Element {
	return ui.Column(c).FillWidth().Radius(RadiusCard).Background(p.Surface).
		Border(1, p.Border).Shadow(0, 1, 4, -1, p.Shadow)
}

// CardPad is Card with the standard inner padding and row gap.
func CardPad(c *ui.Context, p Palette) ui.Element {
	return Card(c, p).Padding(Space4).Gap(Space3)
}

// SectionHeader is a section title with an optional one-line subtitle.
func SectionHeader(c *ui.Context, p Palette, title, sub string) ui.Element {
	col := ui.Column(c).Gap(2).Margin(0, 0, 10, 0)
	col.Children(func() {
		ui.Text(c, title).FontSize(16).FontWeight(700).TextColor(p.Text)
		if sub != "" {
			ui.Text(c, sub).FontSize(FontSmall).TextColor(p.Muted)
		}
	})
	return col
}

// TileInCard is Tile for a row inside an existing card: it keeps the hover
// wash and the press give, but draws no surface of its own, so cards do not
// nest borders.
func TileInCard(c *ui.Context, p Palette, key any, title, sub string, trail func()) ui.Element {
	row := ui.ButtonBase(c.Key(key)).Row().FillWidth().Height(52).Padding(4, 0).
		Gap(10).AlignItems(ui.Center).Radius(RadiusSmall).Label(title)
	Wash(row, p.Surface.Alpha(0), p.Hover)
	Lift(row, 0, 0.985)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(FontBody).FontWeight(600).TextColor(p.Text).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(FontMeta).TextColor(p.Muted).SingleLine()
			}
		})
		if trail != nil {
			trail()
		}
	})
	return row
}

// SettingsGroup is one block of the settings page: a small, muted group
// label over a card of rows. Small is the point — the row titles below carry
// the weight, so a section label the same size would read as another row.
// The 34 DIP lead after the card is Cloudy's section rhythm (see SectionLead).
func SettingsGroup(c *ui.Context, p Palette, title string, fn func()) ui.Element {
	group := ui.Column(c).FillWidth().Margin(0, 0, SectionLead(c), 0)
	group.Children(func() {
		ui.Text(c, title).FontSize(FontSmall).FontWeight(600).TextColor(p.Muted).
			LetterSpacing(0.6).Role(ui.RoleHeading).Margin(0, 0, 8, 0)
		card := ui.Column(c).FillWidth().Gap(2).Padding(6).Radius(RadiusCard).
			Background(p.Surface).Border(1, p.Border)
		card.Children(fn)
	})
	return group
}

// SettingsRow is one setting that is read and changed in place: a bold
// title, a quiet line under it, then the control. The title is the same
// size and weight as a SettingsLinkRow's, so a block reads down one
// column whether its rows navigate or not.
func SettingsRow(c *ui.Context, p Palette, title, sub string, control func()) ui.Element {
	row := ui.Column(c).FillWidth().Gap(Sp(c, 2)).Padding(12, 14)
	row.Children(func() {
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			ui.Text(c, title).FontSize(FontSubhead).FontWeight(600).TextColor(p.Text)
			if sub != "" {
				ui.Text(c, sub).FontSize(FontSmall).TextColor(p.Muted).LineHeight(1.4)
			}
		})
		if control != nil {
			control()
		}
	})
	return row
}

// SettingsLinkRow is one destination: the same title and subtitle as a
// SettingsRow, a hover wash over the whole row and a trailing mark the
// caller draws (the settings page gives it a chevron). The row itself is
// the link, so the whole line — not only the words — is the target.
func SettingsLinkRow(c *ui.Context, p Palette, key any, title, sub string, trail func()) ui.Element {
	row := ui.ButtonBase(c.Key(key)).Row().FillWidth().Height(56).Padding(12, 14).
		Gap(10).AlignItems(ui.Center).Radius(RadiusSmall).Label(title)
	Wash(row, p.Surface.Alpha(0), p.Hover)
	Lift(row, 0, 0.99)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(FontSubhead).FontWeight(600).
				TextColor(p.Text).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(FontSmall).TextColor(p.Muted).SingleLine()
			}
		})
		if trail != nil {
			trail()
		}
	})
	return row
}

// Chip is a selectable pill, built on the framework's toggle button: it
// keeps the toggle's focus, press and accessibility, and only overrides
// the face when it is on.
func Chip(c *ui.Context, p Palette, key any, label string, selected bool) ui.Element {
	on := selected
	chip := ui.Toggle(c.Key(key), &on, label).Padding(5, 12).Radius(RadiusPill)
	if selected {
		chip.Background(p.AccentPale).TextColor(p.Accent)
	}
	return chip
}

// Segmented is a segmented control as AppKit draws one: the choices sit in
// a quiet track and the chosen one rides a capsule that slides to the next
// as it changes. It is Cloudy's segmentedControl (internal/app/ui_kit.go:
// 284-352, AGPL-3.0) with the widths measured per label rather than made
// equal, because this app's labels carry counts (全部(15)) and modes
// (按人数智能配比) of very different lengths.
//
// It is still the framework's SegmentedBase underneath: the radio group,
// focus, keyboard arrows, press feedback and Changed all come from MyGo;
// only the face is drawn here. The capsule is a paint-only AnimateTo, the
// same primitive Cloudy uses, so motion-off settles it in one frame.
//
// MyGo lays a column's children out on the cross axis with Stretch
// (ui/layout.go), so the track pins itself to Start: dropped into a
// Column, Field or modal pane it otherwise fills the whole card width.
// Only use it where the parent is vertical: inside a Row, Start is the top
// edge, and it would un-center the control.
func Segmented(c *ui.Context, selected *int, labels ...string) ui.Element {
	return segmentedControl(c, selected, nil, labels...)
}

// SegmentedDisabled is Segmented with per-choice availability. A disabled
// choice is greyed and takes no pointer or keyboard input; callers should
// also keep *selected on an enabled index.
func SegmentedDisabled(c *ui.Context, selected *int, disabled []bool, labels ...string) ui.Element {
	return segmentedControl(c, selected, disabled, labels...)
}

func segmentedControl(c *ui.Context, selected *int, disabled []bool, labels ...string) ui.Element {
	if len(labels) == 0 {
		return ui.Segmented(c, selected).AlignSelf(ui.Start)
	}
	t := c.Theme()
	const (
		height = 30
		inset  = 2
		padX   = 12
	)
	n := len(labels)
	*selected = max(0, min(*selected, n-1))
	widths := make([]float32, n)
	prefix := make([]float32, n) // where each segment starts inside the track
	total := float32(0)
	for i, label := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: label, Size: 13})
		widths[i] = w + 2*padX
		prefix[i] = total
		total += widths[i]
	}
	trackColor, capsule, divider := t.Surface.Mix(t.Text, 0.06), t.Surface, t.Border
	if t.Dark {
		trackColor = t.Text.Alpha(0.08)
		capsule = t.Text.Alpha(0.18)
		divider = t.Text.Alpha(0.10)
	}
	parts := ui.SegmentedBase(c, selected, n)
	track := parts.Track.Width(total+2*inset).Height(height).
		Padding(inset).Radius(RadiusSmall).Background(trackColor).
		Border(1, t.Border).AlignSelf(ui.Start)
	// The capsule's left edge and width both interpolate, so a slide
	// between two labels of different lengths grows or shrinks on the way.
	offset := float32(0)
	width := widths[*selected]
	if c.Instant() || c.Preferences().ReduceMotion {
		offset = prefix[*selected]
	} else {
		at := track.AnimateTo("at", float32(*selected),
			ui.Animation{Spring: ui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0.08}})
		i := int(at)
		if i < 0 {
			i = 0
		}
		if i >= n-1 {
			offset, width = prefix[n-1], widths[n-1]
		} else {
			f := at - float32(i)
			offset = prefix[i] + f*widths[i]
			width = widths[i] + f*(widths[i+1]-widths[i])
		}
	}
	radius := float32(RadiusSmall - inset)
	track.Draw(func(p *ui.Painter, r ui.Rect) {
		pill := ui.Rect{X: r.X + inset + offset, Y: r.Y + inset, W: width, H: r.H - 2*inset}
		if !t.Dark {
			p.Shadow(pill, radius, 0, 1, 2, 0, t.Text.Alpha(0.12))
		}
		p.Fill(pill, capsule, radius)
	})
	track.Children(func() {
		for i, label := range labels {
			off := i < len(disabled) && disabled[i]
			seg := parts.Segment(i).Width(widths[i]).Height(height - 2*inset).
				Center().Radius(radius)
			if off {
				seg.Disabled(true)
			}
			if seg.Pressed() && i != *selected && !off {
				seg.Background(divider)
			}
			textColor := t.Text
			if off {
				textColor = t.Text.Alpha(0.35)
			}
			seg.Children(func() {
				ui.Text(c, label).FontSize(13).TextColor(textColor).SingleLine()
			})
		}
	})
	return track
}

// Stepper is one integrated [ − 2 人 + ] field: the two buttons and the
// value share a capsule, so the number is changed where it is read rather
// than through a pair of floating arrows (the framework's ui.Stepper) or a
// wide number field. It reports a change after clamping to lo..hi.
func Stepper(c *ui.Context, p Palette, key string, value *int, lo, hi int, unit string, width float32) (ui.Element, bool) {
	return StepperNamed(c, p, key, unit, value, lo, hi, unit, width)
}

// StepperNamed is Stepper with a label for its two buttons, so several
// steppers on one page can be told apart by assistive technology and the
// UI tests ("增加成人人数" beside "增加小孩人数").
func StepperNamed(c *ui.Context, p Palette, key, label string, value *int, lo, hi int, unit string, width float32) (ui.Element, bool) {
	changed := false
	row := ui.Row(c).Width(width).Height(32).Shrink(0).Padding(2).Radius(RadiusPill).
		Background(p.SurfaceAlt).Border(1, p.Border).AlignItems(ui.Center)
	row.Children(func() {
		minus := ui.ButtonBase(c.Key(key+"-minus")).Size(28, 28).Radius(RadiusPill).
			Label("减少" + label)
		Wash(minus, p.SurfaceAlt.Alpha(0), p.Hover)
		minus.Children(func() {
			ui.Text(c, "−").FontSize(16).FontWeight(600).TextColor(p.Ink)
		})
		if minus.Clicked() && *value > lo {
			*value--
			changed = true
		}
		ui.Text(c, fmt.Sprintf("%d %s", *value, unit)).FontSize(FontSmall).
			FontWeight(600).TextColor(p.Text).Grow(1).TextAlign(ui.Center).SingleLine()
		plus := ui.ButtonBase(c.Key(key+"-plus")).Size(28, 28).Radius(RadiusPill).
			Label("增加" + label)
		Wash(plus, p.SurfaceAlt.Alpha(0), p.Hover)
		plus.Children(func() {
			ui.Text(c, "+").FontSize(16).FontWeight(600).TextColor(p.Ink)
		})
		if plus.Clicked() && *value < hi {
			*value++
			changed = true
		}
	})
	return row, changed
}

// Pill is a static, neutral tag: the framework's badge, restyled as a
// quiet chip.
func Pill(c *ui.Context, p Palette, label string) ui.Element {
	return ui.Badge(c, label).FontSize(FontMeta).TextColor(p.Muted).Padding(4, 10).
		Background(p.Surface).Border(1, p.Border)
}

// Tag is a small coloured state tag (success / warning / danger / accent),
// again the framework's badge with the state's color.
func Tag(c *ui.Context, p Palette, label string, color ui.Color) ui.Element {
	return ui.Badge(c, label).FontSize(FontCaption).FontWeight(700).TextColor(color).
		Padding(2, 8).Background(color.Alpha(0.14))
}

// Stat is one number with its label in the line, for the many places that
// need a count but not a card: a page's numbers are a sentence, not four
// white boxes.
func Stat(c *ui.Context, p Palette, value, label string) ui.Element {
	row := ui.Row(c).Gap(Space1).AlignItems(ui.Baseline)
	row.Children(func() {
		ui.Text(c, value).FontSize(20).FontWeight(700).TextColor(p.Text).
			ContentTransition(ui.ContentTransition{Numeric: true})
		ui.Text(c, label).FontSize(FontMeta).TextColor(p.Muted)
	})
	return row
}

// StatCard is one number with its label, for the top of a page. The number
// rolls when it changes, so a count that ticks reads as a count.
func StatCard(c *ui.Context, p Palette, value, label string) ui.Element {
	card := Card(c, p).Grow(1).Padding(Space4).Gap(2)
	card.Children(func() {
		ui.Text(c, value).FontSize(26).FontWeight(700).TextColor(p.Text).
			ContentTransition(ui.ContentTransition{Numeric: true})
		ui.Text(c, label).FontSize(FontSmall).TextColor(p.Muted)
	})
	return card
}

// TintSurface washes a surface with the theme's accent: a whisper at one
// corner deepening to the pale accent at the other. It is painted between
// the surface's own background and its children, so the text keeps its
// contrast and the border and shadow stay the card's own.
func TintSurface(e ui.Element, p Palette, radius float32) {
	e.Draw(func(pnt *ui.Painter, r ui.Rect) {
		pnt.FillGradient(r, ui.LinearGradient{Angle: 100, Oklab: true,
			Stops: []ui.GradientStop{
				{Color: p.Accent.Alpha(0.10), At: 0},
				{Color: p.AccentPale, At: 1},
			}}, radius)
	})
}

// EmptyState is the shared "nothing here yet" panel. Callers add actions.
// It wears a faint theme tint so an empty page still belongs to the theme
// instead of reading as a blank white box.
func EmptyState(c *ui.Context, p Palette, icon *ui.SVG, title, body string) ui.Element {
	box := Card(c, p).Padding(36).Gap(8)
	TintSurface(box, p, RadiusCard)
	box.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(34, 34).TextColor(p.Ink)
		}
		ui.Text(c, title).FontSize(20).FontWeight(700).TextColor(p.Text)
		ui.Text(c, body).FontSize(13).TextColor(p.Muted).LineHeight(1.5)
	})
	return box
}

// KV is one label/value row. Wrap several in a Card for a parameter grid.
func KV(c *ui.Context, p Palette, label, value string) ui.Element {
	row := ui.Row(c).FillWidth().Padding(12, 9).Gap(12).AlignItems(ui.Center)
	row.Children(func() {
		ui.Text(c, label).FontSize(FontSmall).TextColor(p.Muted).Width(104).Shrink(0)
		ui.Text(c, value).FontSize(13).TextColor(p.Text).Grow(1).Wrap()
	})
	return row
}

// growIn is a one-shot 0→1 for a chart's first appearance. It returns 1
// when motion is off or the system asks for less.
func growIn(c *ui.Context, e ui.Element) float32 {
	if c.Instant() || c.Preferences().ReduceMotion {
		return 1
	}
	return e.Keyframes("grow", []ui.Keyframe{
		{Value: 0},
		{At: 500 * time.Millisecond, Value: 1, Ease: ui.EaseOut},
	}, ui.KeyframeOptions{Run: 1})
}

// BarChart draws labelled bars with MyGo's headless chart: a band scale
// places the bars, a linear scale their heights, and the plot owns the
// axis labels, so the page gets the framework's layout and accessibility
// rather than another hand-painted chart.
func BarChart(c *ui.Context, p Palette, labels []string, values []float64, height float32) ui.Element {
	x := chart.NewBand(labels...).Padding(0.35, 0.2)
	_, high := chart.Extent(values...)
	if high <= 0 {
		high = 1
	}
	y := chart.NewLinear(0, high)
	label := func(_ chart.Side, tick chart.Tick) {
		ui.Text(c, tick.Label).NoWrap().FontSize(FontCaption).TextColor(p.Faint)
	}
	return chart.Plot(c, chart.Axes{Bottom: x.Ticks(), Label: label}, func(area ui.Element) {
		grow := growIn(c, area)
		for i, v := range values {
			if v <= 0 {
				continue
			}
			chart.Rect(c, x.Band(i), y.Span(0, v*float64(grow))).Radius(4).Background(p.Accent)
		}
	}).Height(height + 18).Gap(6)
}

// modalPane is the standard dialog surface. Full motion grows it from the
// trigger; lite only fades.
func modalPane(c *ui.Context, p Palette, width float32, level Level) ui.Element {
	pane := ui.Column(c).Width(width).Padding(22).Gap(12).Radius(RadiusPane).
		Material(PaneMaterial(p)).Border(0.5, p.Border).
		Shadow(0, 24, 60, -20, p.Shadow)
	if level.Moves() {
		enter, exit := &ui.Motion{Y: 12, Scale: 0.9}, &ui.Motion{Scale: 0.97}
		if !level.Full() {
			enter, exit = &ui.Motion{}, &ui.Motion{}
		}
		pane.Transition(ui.ElementTransition{Animation: &Pane, Enter: enter, Exit: exit})
	}
	return pane
}

// ModalDialog is the app's dialog: DialogBase with the dimmed backdrop and
// the glass pane every page uses, so Escape, focus return and the backdrop
// stay the framework's job.
func ModalDialog(c *ui.Context, p Palette, open *bool, width float32, level Level, fn func()) {
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(overlayTint(p))
		panel.Children(func() {
			modalPane(c, p, width, level).Children(fn)
		})
	})
}

// FieldLabel is the small label above a form field.
func FieldLabel(c *ui.Context, p Palette, label string) ui.Element {
	return ui.Text(c, label).FontSize(FontMeta).FontWeight(700).TextColor(p.Muted)
}

// HeaderAction is a row that puts a section title on the left and arbitrary
// controls on the right.
func HeaderAction(c *ui.Context, p Palette, title, sub string, actions func()) ui.Element {
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).
		Margin(0, 0, 10, 0)
	row.Children(func() {
		SectionHeader(c, p, title, sub)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(actions)
	})
	return row
}

// ButtonKind is a button's place in the hierarchy: one primary action per
// view, secondary for the rest, danger for what cannot be undone.
type ButtonKind int

const (
	ButtonPrimary ButtonKind = iota
	ButtonSecondary
	ButtonDanger
	ButtonGhost
)

// Button is the app's action surface. It is the framework's button, so
// focus, hover, press and disabled come from the theme; the kind only
// chooses the accent or the text color.
func Button(c *ui.Context, p Palette, key any, label string, kind ButtonKind) ui.Element {
	ctx := c.Key(key)
	var b ui.Element
	switch kind {
	case ButtonPrimary:
		b = ui.PrimaryButton(ctx, label)
	case ButtonDanger:
		b = ui.Button(ctx, label).TextColor(p.Danger)
	case ButtonGhost:
		b = ui.Button(ctx, label).TextColor(p.Muted)
	default:
		b = ui.Button(ctx, label)
	}
	return b.Label(label)
}

// Skeleton is a placeholder bone. Bones across a view share one clock, so
// the sheen sweeps them together; it sleeps between sweeps instead of
// asking for frames.
func Skeleton(c *ui.Context, p Palette, w, h, radius float32) ui.Element {
	return skeleton(c, p, ui.Box(c).Width(w).Height(h), radius)
}

// SkeletonFill is a bone that takes the width it is given, for a card
// whose width the layout decides.
func SkeletonFill(c *ui.Context, p Palette, h, radius float32) ui.Element {
	return skeleton(c, p, ui.Box(c).FillWidth().Height(h), radius)
}

func skeleton(c *ui.Context, p Palette, box ui.Element, radius float32) ui.Element {
	box.Radius(radius)
	box.Draw(func(pnt *ui.Painter, r ui.Rect) {
		pnt.Fill(r, p.Hover, radius)
		if c.Preferences().ReduceMotion {
			return
		}
		const (
			period = 1600 * time.Millisecond
			sweep  = 1100 * time.Millisecond
			band   = float32(260)
		)
		at := time.Duration(pnt.Now().UnixNano()) % period
		if at >= sweep {
			pnt.After(period - at)
			return
		}
		x := -band + (r.W+band*2)*float32(at)/float32(sweep)
		sheen := ui.RGBA(255, 255, 255, 0.55)
		if p.Dark {
			sheen = ui.RGBA(255, 255, 255, 0.09)
		}
		pnt.Clip(r, radius, func() {
			pnt.FillGradient(ui.Rect{X: r.X + x, Y: r.Y, W: band, H: r.H}, ui.LinearGradient{
				Angle: 90,
				Stops: []ui.GradientStop{
					{Color: p.Hover.Alpha(0), At: 0},
					{Color: sheen, At: 0.5},
					{Color: p.Hover.Alpha(0), At: 1},
				},
			}, 0)
		})
		pnt.AnimationFrameRate(60)
	})
	return box
}

// ImageIn advances an image's fade. While bm is nil the fade sits at zero;
// once it arrives it travels to one in 320ms. el must be the stable parent
// element. Split from DrawImage so a caller can fade its placeholder out
// before the image is drawn over it.
func ImageIn(el ui.Element, bm *ui.Bitmap) float32 {
	if bm == nil {
		return el.Jump("art", 0)
	}
	return el.AnimateTo("art", 1, ui.Animation{Duration: 320 * time.Millisecond})
}

// DrawImage fills the parent with bm at the given fade.
func DrawImage(c *ui.Context, bm *ui.Bitmap, radius, in float32) {
	if bm == nil || in <= 0 {
		return
	}
	img := ui.Image(c, bm).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Fit(ui.Cover).Radius(radius)
	if in < 1 {
		img.Opacity(in)
	}
}

// Stagger returns how far in row i is, counted from the moment its page
// opened. Only the first screenful moves; the rest is already there. While
// a row is still arriving the helper asks for the next frame.
func Stagger(c *ui.Context, level Level, opened time.Time, i int) float32 {
	if !level.Full() || i >= StaggerRows {
		return 1
	}
	t := float32(c.Now().Sub(opened)-time.Duration(i)*StaggerStep) / float32(StaggerTime)
	if t >= 1 {
		return 1
	}
	c.AnimationFrame()
	if t < 0 {
		t = 0
	}
	t = 1 - t
	return 1 - t*t*t
}

// StaggerRow applies a row's arrival: ten pixels up and a fade, both done
// when shown reaches one.
func StaggerRow(e ui.Element, shown float32) {
	if shown >= 1 {
		return
	}
	e.Translate(0, 10*(1-shown)).Opacity(shown)
}
