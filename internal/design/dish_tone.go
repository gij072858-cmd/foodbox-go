// Dish tones are the five hues a dish is filed under. They exist because the
// recipe list and hero mark a dish by its category, and a pastel is invisible
// at 3pt: the hues here are saturated enough to survive as a hairline, and
// DishInk / DishWash adapt them to the appearance.

package design

import "github.com/egoist/mygo/ui"

// DishTone is one dish group. The app maps MainCategory values onto these;
// the mapping is data, the colour is a token.
type DishTone int

const (
	DishToneOther DishTone = iota
	DishToneMeat
	DishToneSea
	DishToneVeg
	DishToneEgg
	DishToneGrain
)

// dishHues are the five group hues. They keep the hue of the old cover
// tints, saturated so a 3pt stripe reads as a colour and not as a smudge.
var dishHues = map[DishTone]ui.Color{
	DishToneMeat:  ui.Hex("#c2603a"),
	DishToneSea:   ui.Hex("#3d86a2"),
	DishToneVeg:   ui.Hex("#4f8f52"),
	DishToneEgg:   ui.Hex("#b08a2e"),
	DishToneGrain: ui.Hex("#7a68a8"),
}

// DishInk is the colour of the bar: the hue itself in the light, lifted
// toward the ink of the dark so it keeps its hue without glowing.
func DishInk(p Palette, tone DishTone) ui.Color {
	if tone == DishToneOther {
		return p.Accent
	}
	hue := dishHues[tone]
	if p.Dark {
		return hue.Mix(p.Text, 0.28)
	}
	return hue
}

// DishWash is the tile the icon sits on.
func DishWash(p Palette, tone DishTone) ui.Color {
	if tone == DishToneOther {
		return p.AccentPale
	}
	if p.Dark {
		return DishInk(p, tone).Alpha(0.30)
	}
	return DishInk(p, tone).Alpha(0.16)
}
