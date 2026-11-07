// Adapted from github.com/egoist/kero theme.go.
// MIT License, Copyright (c) 2026 EGOIST.
//
// The theme algorithm (washTable, toneTheme, tinted, oklch, glowOf) is
// copied verbatim from Kero with only name adaptation: package design and
// a kero prefix on every symbol. Do not rewrite it — tune through the
// washTable entries.

package design

import (
	"math"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// abs is Kero's own float32 helper (controls.go), needed by keroToneTheme.
func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// keroTone is the hue of a kero theme, how much of it its cards and text
// take, how vivid the programs' colors are, which lean to its hue, and
// the hue of its accent: the cursor, selection and controls.
type keroTone struct{ hue, tint, vivid, accent float32 }

// keroWash is one theme's light and dark window gradients, their corner
// glows, and the tone that tints the cards and controls above them.
type keroWash struct {
	light, dark         []string
	lightGlow, darkGlow ui.Color
	tone                keroTone
}

var keroWashTable = map[string]keroWash{
	// Rex's: pink lavender above, rose at the top right, butter cream below.
	"rex": {
		light: []string{"#ebe0f3", "#f3e7f1", "#f7eee4", "#f6efcf"}, lightGlow: ui.RGBA(247, 220, 241, 0.75),
		dark: []string{"#240f1d", "#26111a", "#271610"}, darkGlow: ui.RGBA(142, 58, 110, 0.3),
		tone: keroTone{350, 1, 0.95, 350},
	},
	"mist": {
		light: []string{"#e3e9f4", "#ebeff6", "#f1f2f3", "#f2f1ec"}, lightGlow: ui.RGBA(210, 225, 248, 0.75),
		dark: []string{"#0c1628", "#0d1827", "#111b23"}, darkGlow: ui.RGBA(46, 91, 166, 0.3),
		tone: keroTone{255, 1, 0.8, 255},
	},
	"meadow": {
		light: []string{"#e2eee5", "#ebf2e7", "#f3f2e2", "#f5f0d6"}, lightGlow: ui.RGBA(214, 240, 222, 0.75),
		dark: []string{"#081c0d", "#101d0c", "#1b1c0b"}, darkGlow: ui.RGBA(6, 113, 50, 0.3),
		tone: keroTone{140, 1, 0.9, 140},
	},
	"dusk": {
		light: []string{"#e4dbf3", "#efdeee", "#f6e3dc", "#f7e6cf"}, lightGlow: ui.RGBA(246, 206, 222, 0.8),
		dark: []string{"#190f2e", "#260f24", "#2d140b"}, darkGlow: ui.RGBA(125, 65, 138, 0.3),
		tone: keroTone{315, 1, 1.1, 315},
	},
	// Sea glass: aqua above, a pale tide line below.
	"tide": {
		light: []string{"#dbebef", "#e3eff1", "#ecf3f0", "#f2f2e8"}, lightGlow: ui.RGBA(194, 232, 238, 0.75),
		dark: []string{"#001b22", "#001e1f", "#0b1e1b"}, darkGlow: ui.RGBA(0, 112, 128, 0.3),
		tone: keroTone{205, 1, 0.85, 205},
	},
	"apricot": {
		light: []string{"#f5e1d6", "#f8e7dc", "#f8eedf", "#f6f0da"}, lightGlow: ui.RGBA(252, 212, 194, 0.78),
		dark: []string{"#2a1207", "#291507", "#25190a"}, darkGlow: ui.RGBA(151, 62, 22, 0.3),
		tone: keroTone{50, 1, 1.05, 50},
	},
	// Mint at the top, fading through blue to violet.
	"aurora": {
		light: []string{"#dbefe8", "#e1ebf2", "#e8e4f4", "#f0e6f1"}, lightGlow: ui.RGBA(198, 238, 224, 0.75),
		dark: []string{"#001f13", "#06192b", "#1e122a"}, darkGlow: ui.RGBA(0, 115, 75, 0.3),
		tone: keroTone{170, 1, 1.15, 170},
	},
	"dune": {
		light: []string{"#ede4d8", "#f1e9de", "#f4eee4", "#f3efe8"}, lightGlow: ui.RGBA(242, 222, 194, 0.7),
		dark: []string{"#22190b", "#211a0e", "#201b12"}, darkGlow: ui.RGBA(135, 79, 0, 0.3),
		tone: keroTone{80, 0.7, 0.55, 80},
	},
	// White paper on a desk of light gray, in ink, flat and quiet.
	"paper": {
		light: []string{"#f6f6f7", "#f7f7f8", "#f8f8f8"}, lightGlow: ui.RGBA(255, 255, 255, 0),
		dark: []string{"#141416", "#151517", "#161618"}, darkGlow: ui.RGBA(90, 120, 200, 0.1),
		tone: keroTone{260, 0, 0.9, 262},
	},
	"plain": {
		light: []string{"#eceef4", "#eceef4"}, lightGlow: ui.RGBA(0, 0, 0, 0),
		dark: []string{"#141418", "#141418"}, darkGlow: ui.RGBA(0, 0, 0, 0),
	},
}

// keroPalette is kero's colors, for the light and the dark appearance.
type keroPalette struct {
	dark bool

	sidebar      ui.Color // under the sidebar where no material shows
	text         ui.Color
	textMuted    ui.Color // project headers, times
	textFaint    ui.Color // counts, hints
	rowHover     ui.Color
	rowSelected  ui.Color
	rowSelBorder ui.Color
	rowShadow    ui.Color

	card        ui.Color // the terminal's card
	cardBorder  ui.Color
	cardShadow  ui.Color
	cardShadow2 ui.Color
	floatShadow ui.Color
	cardDim     ui.Color // a pane the user is not in
	// edge and edgeHigh are the hairlines around surfaces in the dark:
	// cards and rows, and what floats or takes a click.
	edge     ui.Color
	edgeHigh ui.Color
	// veil fades a pane the user is not in, its text too, as Ghostty's
	// unfocused-split-opacity.
	veil     ui.Color
	buttonBg ui.Color
	// Controls' faces: buttons, fields, selects, a segmented control's
	// choice. Light ones stand on a soft shadow; dark ones on their
	// lighter fill alone, with a hint of light along their top, as macOS
	// draws them.
	control       ui.Color
	controlHover  ui.Color
	controlShadow bool
	controlLight  ui.Color
	divider       ui.Color
	chip          ui.Color

	attention ui.Color // needs you
	attnSoft  ui.Color
	done      ui.Color // the filled check
	onDone    ui.Color
	danger    ui.Color
	accent    ui.Color

	overlay   ui.Color // behind dialogs
	popover   ui.Color
	kbd       ui.Color
	kbdBorder ui.Color
}

var keroLightPalette = keroPalette{
	sidebar:      ui.Hex("#eceef4"),
	text:         ui.Hex("#1d1d1f"),
	textMuted:    ui.Hex("#6e6e78"),
	textFaint:    ui.Hex("#a3a3ad"),
	rowHover:     ui.RGBA(0, 0, 0, 0.04),
	rowSelected:  ui.Hex("#ffffff"),
	rowSelBorder: ui.RGBA(0, 0, 0, 0.05),
	rowShadow:    ui.RGBA(20, 20, 50, 0.08),

	card:          ui.Hex("#ffffff"),
	cardBorder:    ui.RGBA(0, 0, 0, 0.07),
	cardShadow:    ui.RGBA(0, 0, 0, 0.07),
	cardShadow2:   ui.RGBA(0, 0, 0, 0.035),
	floatShadow:   ui.RGBA(20, 20, 40, 0.14),
	cardDim:       ui.RGBA(255, 255, 255, 0.58),
	veil:          ui.RGBA(250, 248, 252, 0.28),
	buttonBg:      ui.Hex("#ffffff"),
	control:       ui.Hex("#ffffff"),
	controlHover:  ui.Hex("#f7f7f9"),
	controlShadow: true,
	divider:       ui.RGBA(0, 0, 0, 0.06),
	chip:          ui.RGBA(0, 0, 0, 0.045),

	attention: ui.Hex("#c27b0f"),
	attnSoft:  ui.RGBA(232, 154, 40, 0.13),
	done:      ui.Hex("#1d1d1f"),
	onDone:    ui.Hex("#ffffff"),
	danger:    ui.Hex("#d93a35"),
	accent:    ui.Hex("#3b6cf6"),

	overlay:   ui.RGBA(10, 10, 30, 0.12),
	popover:   ui.Hex("#ffffff"),
	kbd:       ui.RGBA(0, 0, 0, 0.04),
	kbdBorder: ui.RGBA(0, 0, 0, 0.09),
}

var keroDarkPalette = keroPalette{
	dark:         true,
	sidebar:      ui.Hex("#141418"),
	text:         ui.Hex("#ececf0"),
	textMuted:    ui.Hex("#a9a9b4"),
	textFaint:    ui.Hex("#83838f"),
	rowHover:     ui.RGBA(255, 255, 255, 0.05),
	rowSelected:  ui.RGBA(255, 255, 255, 0.10),
	rowSelBorder: ui.RGBA(255, 255, 255, 0.06),
	rowShadow:    ui.RGBA(0, 0, 0, 0.25),

	card:         ui.Hex("#232328"),
	cardBorder:   ui.RGBA(255, 255, 255, 0.08),
	cardShadow:   ui.RGBA(0, 0, 0, 0.35),
	cardShadow2:  ui.RGBA(0, 0, 0, 0.2),
	floatShadow:  ui.RGBA(0, 0, 0, 0.45),
	cardDim:      ui.RGBA(35, 35, 40, 0.5),
	veil:         ui.RGBA(14, 13, 18, 0.42),
	edge:         ui.RGBA(255, 255, 255, 0.075),
	edgeHigh:     ui.RGBA(255, 255, 255, 0.11),
	buttonBg:     ui.Hex("#404047"),
	control:      ui.Hex("#404047"),
	controlHover: ui.Hex("#4a4a52"),
	controlLight: ui.RGBA(255, 255, 255, 0.09),
	divider:      ui.RGBA(255, 255, 255, 0.07),
	chip:         ui.RGBA(255, 255, 255, 0.07),

	attention: ui.Hex("#f0a33a"),
	attnSoft:  ui.RGBA(240, 163, 58, 0.15),
	done:      ui.Hex("#ececf0"),
	onDone:    ui.Hex("#161618"),
	danger:    ui.Hex("#ff6961"),
	accent:    ui.Hex("#6b93ff"),

	overlay:   ui.RGBA(0, 0, 0, 0.35),
	popover:   ui.Hex("#232327"),
	kbd:       ui.RGBA(255, 255, 255, 0.06),
	kbdBorder: ui.RGBA(255, 255, 255, 0.1),
}

// Terminal themes on the card's color, with palettes tuned to read on it:
// Apple's system colors, a little deeper in the light.
var keroLightTermTheme = &terminal.Theme{
	Foreground:    ui.Hex("#1d1d1f"),
	Background:    keroLightPalette.card,
	Cursor:        ui.Hex("#1d1d1f"),
	CursorText:    ui.Hex("#ffffff"),
	Selection:     ui.Hex("#cfdcfb"),
	SelectionText: ui.Hex("#1d1d1f"),
	Palette: [16]ui.Color{
		ui.Hex("#1d1d1f"), ui.Hex("#d3302f"), ui.Hex("#22863a"), ui.Hex("#b08800"),
		ui.Hex("#2f5bd8"), ui.Hex("#a033c4"), ui.Hex("#0f8199"), ui.Hex("#8e8e93"),
		ui.Hex("#5f5f66"), ui.Hex("#ef4444"), ui.Hex("#2fa84f"), ui.Hex("#d4a017"),
		ui.Hex("#3b82f6"), ui.Hex("#c04ee0"), ui.Hex("#14a3bf"), ui.Hex("#b4b4bb"),
	},
}

var keroDarkTermTheme = &terminal.Theme{
	Foreground:    ui.Hex("#e6e6ea"),
	Background:    keroDarkPalette.card,
	Cursor:        ui.Hex("#e6e6ea"),
	CursorText:    ui.Hex("#161618"),
	Selection:     ui.Hex("#2f4170"),
	SelectionText: ui.Hex("#ffffff"),
	Palette: [16]ui.Color{
		ui.Hex("#3a3a40"), ui.Hex("#ff6b64"), ui.Hex("#4fd17a"), ui.Hex("#f2c14e"),
		ui.Hex("#6b9bff"), ui.Hex("#d07cf0"), ui.Hex("#4cc9e0"), ui.Hex("#c9c9cf"),
		ui.Hex("#6c6c78"), ui.Hex("#ff8a84"), ui.Hex("#74e397"), ui.Hex("#ffd479"),
		ui.Hex("#8fb3ff"), ui.Hex("#e09cf7"), ui.Hex("#7fdcec"), ui.Hex("#f5f5f7"),
	},
}

// keroLookDef is a theme in one appearance: a terminal theme whose colors
// tint the cards and controls, with the wash and glow under everything.
type keroLookDef struct {
	term       *terminal.Theme
	pal        *keroPalette
	base, glow ui.LinearGradient
	plain      bool // kero's original palette, no tone applied
}

func keroGlowOf(c ui.Color) ui.LinearGradient {
	return ui.LinearGradient{Angle: 225, Oklab: true, Stops: []ui.GradientStop{{Color: c, At: 0}, {Color: c.Alpha(0), At: 0.5}}}
}

var keroLooks = func() map[string][2]*keroLookDef {
	m := map[string][2]*keroLookDef{}
	for id, w := range keroWashTable {
		var both [2]*keroLookDef
		for i, dark := range []bool{false, true} {
			hexes, glow := w.light, w.lightGlow
			if dark {
				hexes, glow = w.dark, w.darkGlow
			}
			stops := make([]ui.GradientStop, len(hexes))
			for j, h := range hexes {
				stops[j] = ui.GradientStop{Color: ui.Hex(h), At: float32(j) / float32(len(hexes)-1)}
			}
			l := &keroLookDef{base: ui.LinearGradient{Angle: 180, Oklab: true, Stops: stops}, glow: keroGlowOf(glow), plain: id == "plain"}
			if l.plain {
				// kero's colors as they were, on the card's white or gray.
				l.term, l.pal = keroLightTermTheme, &keroLightPalette
				if dark {
					l.term, l.pal = keroDarkTermTheme, &keroDarkPalette
				}
			} else {
				var accent ui.Color
				l.term, accent = keroToneTheme(w.tone, dark)
				l.pal = keroTinted(l.term, accent)
			}
			both[i] = l
		}
		m[id] = both
	}
	return m
}()

// keroID is the wash of one of kero's themes by its name in the
// configuration, where Rex's is "petal"; "" for a name of none of kero's.
func keroID(name string) string {
	if name == "petal" {
		return "rex"
	}
	if _, ok := keroWashTable[name]; ok && name != "rex" {
		return name
	}
	return ""
}

// keroName is a kero theme's name in the configuration (keroID).
func keroName(id string) string {
	if id == "rex" {
		return "petal"
	}
	return id
}

// keroLook is kero's theme of a wash; an unknown id falls back to paper.
func keroLook(id string, dark bool) *keroLookDef {
	l, ok := keroLooks[id]
	if !ok {
		l = keroLooks["paper"]
	}
	if dark {
		return l[1]
	}
	return l[0]
}

// keroTinted is kero's palette for the light or the dark, as a terminal
// theme's background is, on the theme's colors: the cards its background,
// the text its foreground, with the accent.
func keroTinted(th *terminal.Theme, accent ui.Color) *keroPalette {
	p := keroLightPalette
	if l, _, _ := keroOklch(th.Background); l < 0.5 {
		p = keroDarkPalette
	}
	bg, fg, white := th.Background, th.Foreground, ui.Hex("#ffffff")
	p.card = bg
	p.cardDim = bg.Alpha(float32(p.cardDim.A) / 255)
	p.text, p.textMuted, p.textFaint = fg, fg.Mix(bg, 0.38), fg.Mix(bg, 0.56)
	p.done, p.onDone = fg, bg
	p.accent = accent
	if p.dark {
		p.popover = bg.Mix(white, 0.04)
	} else {
		p.rowSelected = bg.Mix(white, 0.5)
		p.popover = bg.Mix(white, 0.6)
	}
	return &p
}

// keroToneTheme is a kero theme's terminal colors, and its accent: its// cards and text tinted with its hue, and the programs' colors muted to
// sit with them, the cursor and selection in its hue.
func keroToneTheme(t keroTone, dark bool) (*terminal.Theme, ui.Color) {
	tint := func(l, c float32) ui.Color { return ui.Oklch(l, c*t.tint, t.hue) }
	// Red, green, yellow, blue, magenta and cyan.
	hues := [6]float32{28, 145, 85, 255, 325, 195}
	ls := [6]float32{0.56, 0.56, 0.62, 0.54, 0.54, 0.56}
	cs := [6]float32{0.15, 0.13, 0.12, 0.13, 0.14, 0.09}
	bright := float32(0.05)
	th := &terminal.Theme{}
	var accent ui.Color
	if dark {
		ls = [6]float32{0.70, 0.74, 0.80, 0.70, 0.72, 0.76}
		cs = [6]float32{0.14, 0.13, 0.12, 0.12, 0.13, 0.10}
		bright = 0.06
		th.Background, th.Foreground = tint(0.255, 0.02), tint(0.92, 0.015)
		th.Palette[0], th.Palette[7], th.Palette[8], th.Palette[15] = tint(0.36, 0.02), tint(0.82, 0.012), tint(0.5, 0.02), tint(0.96, 0.01)
		th.Selection = ui.Oklch(0.38, 0.06, t.accent)
		accent = ui.Oklch(0.74, 0.13, t.accent)
	} else {
		// The less tinted, the whiter the paper: an untinted theme's
		// cards are white.
		paper := 0.978 + 0.022*max(0, 1-t.tint/0.5)
		th.Background, th.Foreground = tint(paper, 0.012), tint(0.28, 0.03)
		th.Palette[0], th.Palette[7], th.Palette[8], th.Palette[15] = th.Foreground, tint(0.7, 0.02), tint(0.52, 0.02), tint(0.8, 0.015)
		th.Selection = ui.Oklch(0.9, 0.045, t.accent)
		accent = ui.Oklch(0.55, 0.13, t.accent)
	}
	for i := range 6 {
		// Toward the theme's hue, the less the farther it is, as a far
		// color turned is a muddy one, and more vivid the nearer to it: a
		// theme's own color stands out of its palette.
		d := float32(math.Remainder(float64(t.hue-hues[i]), 360))
		h := hues[i] + d*0.35*(1-abs(d)/180)
		c := cs[i] * t.vivid * (1 + 0.45*max(0, 1-abs(d)/60))
		th.Palette[1+i] = ui.Oklch(ls[i], c, h)
		th.Palette[9+i] = ui.Oklch(ls[i]+bright, c+0.01, h)
	}
	th.Cursor, th.CursorText, th.SelectionText = accent, th.Background, th.Foreground
	return th, accent
}

// keroOklch returns a color's lightness, chroma and hue, as CSS's oklch().
func keroOklch(c ui.Color) (l, ch, h float32) {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.R), lin(c.G), lin(c.B)
	lc := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mc := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sc := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc
	A := 1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc
	B := 0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc
	hue := math.Mod(math.Atan2(B, A)*180/math.Pi+360, 360)
	return float32(L), float32(math.Hypot(A, B)), float32(hue)
}

// KeroTheme is one selectable Kero theme. Name is the stored id; Label is
// shown in Chinese and English in the settings picker.
type KeroTheme struct{ Name, Label, English string }

// KeroThemes are the ten washes, in Kero's settings order.
var KeroThemes = []KeroTheme{
	{"petal", "花瓣", "petal"}, {"mist", "薄雾", "mist"}, {"meadow", "草甸", "meadow"},
	{"dusk", "黄昏", "dusk"}, {"tide", "潮汐", "tide"}, {"apricot", "杏子", "apricot"},
	{"aurora", "极光", "aurora"}, {"dune", "沙丘", "dune"}, {"paper", "纸", "paper"},
	{"plain", "素", "plain"},
}

// DefaultTheme is Kero's default: white paper, quiet.
const DefaultTheme = "paper"

// ValidTheme reports whether name is one of the ten theme names.
func ValidTheme(name string) bool { return keroID(name) != "" }

// ThemeLabel is the Chinese label of a theme name.
func ThemeLabel(name string) string {
	for _, t := range KeroThemes {
		if t.Name == name {
			return t.Label
		}
	}
	return name
}

// ThemeOptions are the Chinese labels of the ten themes, in KeroThemes
// order, for a picker that shows names rather than colour previews.
func ThemeOptions() []string {
	out := make([]string, 0, len(KeroThemes))
	for _, t := range KeroThemes {
		out = append(out, t.Label)
	}
	return out
}

// ThemeNameForLabel is ThemeLabel reversed, or "" for a label that names no
// theme — a picker's value is the label the user reads, while what is stored
// is the id.
func ThemeNameForLabel(label string) string {
	for _, t := range KeroThemes {
		if t.Label == label {
			return t.Name
		}
	}
	return ""
}

// keroMidColor is the gradient's color at its midpoint, for a solid
// fallback where a wash cannot be drawn.
func keroMidColor(g ui.LinearGradient) ui.Color {
	if len(g.Stops) == 0 {
		return ui.Color{}
	}
	mid := len(g.Stops) / 2
	if len(g.Stops)%2 == 1 {
		return g.Stops[mid].Color
	}
	return g.Stops[mid-1].Color.Mix(g.Stops[mid].Color, 0.5)
}

// WashBackdrop paints Kero's window base: the theme's wash on the root,
// its glow from the top right on row, which holds everything else.
func WashBackdrop(c *ui.Context, row ui.Element, p Palette) {
	c.Root().LinearGradient(p.Wash)
	row.LinearGradient(p.Glow)
}
