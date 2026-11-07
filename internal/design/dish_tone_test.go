package design

import (
	"fmt"
	"testing"
)

// The dish tones must stay distinguishable in every theme and appearance:
// they are the only thing telling a meat dish from a fish dish in the list.
func TestDishTonesSurviveEveryTheme(t *testing.T) {
	tones := []DishTone{DishToneMeat, DishToneSea, DishToneVeg, DishToneEgg, DishToneGrain}
	for _, theme := range KeroThemes {
		name := theme.Name
		for _, dark := range []bool{false, true} {
			p := PaletteFor(name, dark)
			seen := map[string]DishTone{}
			for _, tone := range tones {
				ink := DishInk(p, tone)
				if ink.A == 0 {
					t.Fatalf("%s dark=%v: tone %v is transparent", name, dark, tone)
				}
				key := fmt.Sprintf("%02x%02x%02x", ink.R, ink.G, ink.B)
				if other, ok := seen[key]; ok {
					t.Fatalf("%s dark=%v: tones %v and %v collapse to %s", name, dark, other, tone, key)
				}
				seen[key] = tone
				// The wash must read as a tile without stealing the text.
				if DishWash(p, tone).A == 0 {
					t.Fatalf("%s dark=%v: wash %v is invisible", name, dark, tone)
				}
			}
			if DishInk(p, DishToneOther) != p.Accent {
				t.Fatalf("%s dark=%v: the fallback tone should be the theme accent", name, dark)
			}
		}
	}
}
