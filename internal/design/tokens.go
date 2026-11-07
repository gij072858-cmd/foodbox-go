// Package design holds the visual tokens and shared motion of FoodBox-Go.
// It depends on MyGo's ui package only, never on app state.
package design

import "github.com/egoist/mygo/ui"

// Layout metrics. PagePadding is the least margin a page ever gets: 32 is
// Cloudy's breathable page edge (ui_pages.go:14 used 40 for its denser
// desktop layout), and it is what lets a page of stacked blocks read as
// blocks without pinning content to the window.
//
// TitleBand is the height of the strip the window controls are drawn over in
// a hidden title bar. 36 is a pure drag strip, as Cloudy keeps it
// (ui_main.go:295-299): no page title or actions ride at the window's top
// edge. It is an int because mygo.WindowOptions.TitleBarHeight is one.
const (
	SidebarWidth = 232
	PageMaxWidth = 1100
	PagePadding  = 32
	TitleBand    = 36
	// RunBoardWidth is the cooking run's left rail: narrow enough that the
	// lyric flow keeps the width, tall enough to read as a board.
	RunBoardWidth = 300
	// RunControlSize is one icon-only transport control.
	RunControlSize = 38
)

// Type scale. The app reads at a desktop distance, so body sits at 14 and
// metadata never goes below 10. Cloudy keeps the same narrow ladder; the
// point is that a page uses three sizes, not seven.
const (
	FontDisplay = 30
	FontHero    = 26 // the home greeting and the run clock
	FontTitle   = 24
	FontHeading = 18
	FontSubhead = 15
	FontBody    = 14
	FontSmall   = 12
	FontMeta    = 11
	FontCaption = 10
)

// Corner radii. Every surface picks one of these; a page that invents 9 or
// 16 is a page that will drift.
const (
	RadiusSmall  = 8
	RadiusCard   = 14
	RadiusPane   = 20
	RadiusDialog = 26
	RadiusPill   = 999
)

// Spacing steps. Four is the base unit, as in the theme.
const (
	Space1 = 4
	Space2 = 8
	Space3 = 12
	Space4 = 16
	Space5 = 24
	Space6 = 32
)

// Sp is n units of the theme's spacing scale, the framework's own lever:
// ui.Theme.Spacing is 4 by default and its docs say 3 is compact and 5 is
// roomy, so a page sized through Sp moves with the theme instead of with
// whatever number a page happened to type. The consts above cannot do this
// — a const cannot read the theme — so new layout code goes through Sp.
func Sp(c *ui.Context, n float32) float32 { return c.Theme().Space(n) }

// SectionGap is the room between a section's title and its own content,
// and SectionLead the air before the next section. Both are Cloudy's
// section rhythm (ui_pages.go:33, Margin(34, 0, 14, 0)): the break between
// blocks is more than twice the break within one, which is what lets a page
// of many blocks be read as blocks rather than as one long column.
func SectionGap(c *ui.Context) float32 { return Sp(c, 3.5) }

func SectionLead(c *ui.Context) float32 { return Sp(c, 8.5) }

// Palette is the app's colors for one theme in one appearance. They all
// derive from the Kero theme engine (kero_theme.go); a page never picks a
// raw hex.
type Palette struct {
	Dark       bool
	Bg         ui.Color // solid fallback for the wash, never a page background
	Surface    ui.Color
	SurfaceAlt ui.Color
	Sidebar    ui.Color
	Text       ui.Color
	Muted      ui.Color
	Faint      ui.Color
	Border     ui.Color
	Hover      ui.Color
	Press      ui.Color
	Accent     ui.Color
	AccentPale ui.Color
	OnAccent   ui.Color
	Warning    ui.Color
	Success    ui.Color
	Danger     ui.Color
	Shadow     ui.Color
	CoverStart ui.Color
	CoverEnd   ui.Color
	Ink        ui.Color          // every icon: near-black in the light, near-white in the dark
	Wash       ui.LinearGradient // the window's base gradient (180°, Oklab)
	Glow       ui.LinearGradient // the top-right glow (225°, Oklab)
}

// PaletteFor returns the palette of one Kero theme in the light or the
// dark appearance. An unknown theme falls back to paper.
//
// The ten washes are a light-appearance idea. In the dark one, they had to
// be re-derived through their own Oklch lightness curves, which produced
// ten nearly-indistinguishable dark palettes that each shifted the window
// wash, the cards and the accent a little — colour churn that read as
// noise. So dark wears one simple palette (素/plain) whatever the theme, and
// the picker only has an effect in light.
func PaletteFor(theme string, dark bool) Palette {
	if dark {
		theme = "plain"
	}
	look := keroLook(keroID(theme), dark)
	th, kp := look.term, look.pal
	onAccent := ui.Hex("#ffffff")
	shadow := kp.cardShadow
	cs, ce := float32(0.70), float32(0.88)
	if dark {
		onAccent = kp.card
		shadow = kp.floatShadow
		cs, ce = 0.55, 0.80
	}
	return Palette{
		Dark:       dark,
		Bg:         keroMidColor(look.base),
		Surface:    kp.card,
		SurfaceAlt: kp.popover,
		Sidebar:    kp.sidebar,
		Text:       kp.text,
		Muted:      kp.textMuted,
		Faint:      kp.textFaint,
		Border:     kp.cardBorder,
		Hover:      kp.rowHover,
		Press:      kp.text.Alpha(0.08),
		Accent:     kp.accent,
		AccentPale: kp.accent.Alpha(0.14),
		OnAccent:   onAccent,
		Warning:    kp.attention,
		Success:    th.Palette[2],
		Danger:     kp.danger,
		Shadow:     shadow,
		CoverStart: kp.accent.Mix(kp.card, cs),
		CoverEnd:   kp.accent.Mix(kp.card, ce),
		Ink:        kp.text,
		Wash:       look.base,
		Glow:       look.glow,
	}
}
