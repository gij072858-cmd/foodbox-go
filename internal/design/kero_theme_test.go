package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestKeroThemes(t *testing.T) {
	want := []string{"petal", "mist", "meadow", "dusk", "tide", "apricot", "aurora", "dune", "paper", "plain"}
	if len(KeroThemes) != len(want) {
		t.Fatalf("KeroThemes has %d entries, want %d", len(KeroThemes), len(want))
	}
	for i, name := range want {
		th := KeroThemes[i]
		if th.Name != name {
			t.Errorf("KeroThemes[%d] = %q, want %q", i, th.Name, name)
		}
		if !ValidTheme(name) {
			t.Errorf("ValidTheme(%q) = false", name)
		}
		if th.Label == "" || th.English == "" {
			t.Errorf("theme %q misses its labels", name)
		}
	}
	if ValidTheme("rex") || ValidTheme("ghostty") || ValidTheme("") {
		t.Error("only the ten configuration names are valid themes")
	}
	if got := keroID("petal"); got != "rex" {
		t.Errorf("keroID(petal) = %q, want rex", got)
	}
	if got := keroName("rex"); got != "petal" {
		t.Errorf("keroName(rex) = %q, want petal", got)
	}
	if keroLook("nope", false) != keroLook("paper", false) || keroLook("nope", true) != keroLook("paper", true) {
		t.Error("an unknown theme must fall back to paper")
	}
	if got := ThemeLabel("dune"); got != "沙丘" {
		t.Errorf("ThemeLabel(dune) = %q", got)
	}
	if got := ThemeLabel("nope"); got != "nope" {
		t.Errorf("ThemeLabel of an unknown name = %q, want the name itself", got)
	}
}

func TestPaletteForKeroThemes(t *testing.T) {
	accents := map[ui.Color]string{}
	for _, th := range KeroThemes {
		lp := PaletteFor(th.Name, false)
		dp := PaletteFor(th.Name, true)

		ll, _, _ := keroOklch(lp.Surface)
		dl, _, _ := keroOklch(dp.Surface)
		if ll <= dl {
			t.Errorf("%s: light card L=%v, dark card L=%v", th.Name, ll, dl)
		}
		if lp.Dark || !dp.Dark {
			t.Errorf("%s: Dark flag out of sync", th.Name)
		}
		if lp.Ink != lp.Text || dp.Ink != dp.Text {
			t.Errorf("%s: Ink must be the text color", th.Name)
		}
		w := keroWashTable[keroID(th.Name)]
		// Light wears the theme's own wash; dark always wears plain's,
		// whatever the theme says (see PaletteFor).
		plainDark := keroWashTable[keroID("plain")].dark
		if len(lp.Wash.Stops) != len(w.light) || len(dp.Wash.Stops) != len(plainDark) {
			t.Errorf("%s: wash stop counts %d/%d, want %d/%d",
				th.Name, len(lp.Wash.Stops), len(dp.Wash.Stops), len(w.light), len(plainDark))
		}
		if prev, dup := accents[lp.Accent]; dup {
			t.Errorf("%s and %s share the same light accent", th.Name, prev)
		}
		accents[lp.Accent] = th.Name
	}

	// plain keeps Kero's static palettes.
	if p := PaletteFor("plain", false); p.Surface != ui.Hex("#ffffff") || p.Accent != ui.Hex("#3b6cf6") {
		t.Error("plain light is not the static kero palette")
	}
	if p := PaletteFor("plain", true); p.Surface != ui.Hex("#232328") || p.Accent != ui.Hex("#6b93ff") {
		t.Error("plain dark is not the static kero palette")
	}

	// paper is the fallback, in both appearances.
	for _, dark := range []bool{false, true} {
		if PaletteFor("nope", dark).Bg != PaletteFor("paper", dark).Bg {
			t.Errorf("unknown theme (dark=%v) does not fall back to paper", dark)
		}
	}
	if got := DefaultTheme; got != "paper" {
		t.Errorf("DefaultTheme = %q", got)
	}
}

func TestKeroWashGeometry(t *testing.T) {
	for _, th := range KeroThemes {
		for _, dark := range []bool{false, true} {
			p := PaletteFor(th.Name, dark)
			if p.Wash.Angle != 180 || !p.Wash.Oklab {
				t.Errorf("%s (dark=%v): wash is %+v, want a 180° Oklab gradient", th.Name, dark, p.Wash.Angle)
			}
			if p.Glow.Angle != 225 || !p.Glow.Oklab {
				t.Errorf("%s (dark=%v): glow is %+v, want a 225° Oklab gradient", th.Name, dark, p.Glow.Angle)
			}
		}
	}
}

// TestDarkPaletteIsOnePalette pins the settings decision: the ten washes are
// a light-appearance idea, and re-deriving each of them into dark gave ten
// near-identical palettes that all shifted at once. Dark now wears 素/plain
// whatever the theme says, while light still keeps all ten.
func TestDarkPaletteIsOnePalette(t *testing.T) {
	for _, th := range KeroThemes {
		dark := darkColors(PaletteFor(th.Name, true))
		want := darkColors(PaletteFor(DefaultTheme, true))
		if dark != want {
			t.Errorf("%s in dark = %v, want plain's %v", th.Name, dark, want)
		}
	}

	// Light must still differ, or the picker would do nothing at all.
	seen := map[[6]ui.Color]bool{}
	for _, th := range KeroThemes {
		seen[darkColors(PaletteFor(th.Name, false))] = true
	}
	if len(seen) < len(KeroThemes) {
		t.Errorf("light palettes collapsed: %d distinct of %d themes", len(seen), len(KeroThemes))
	}
}

// darkColors picks the fields a theme actually changes, so the comparison
// says "these ten are one" without depending on an uncomparable Palette.
func darkColors(p Palette) [6]ui.Color {
	return [6]ui.Color{p.Surface, p.Accent, p.Text, p.Border, p.Bg, p.Wash.From}
}
