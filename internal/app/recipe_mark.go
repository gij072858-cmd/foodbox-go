package app

// The dish mark replaces the gradient cover. The fourteen MainCategory values
// the seed carries collapse into five design tones, and a dish reads as its
// tone: a 3pt bar down the card edge and an icon tile. Colour is meaning now,
// not decoration, so two dishes of one category look alike on purpose.

import (
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func markTone(rec domain.Recipe) design.DishTone {
	switch rec.MainCategory {
	case "猪肉", "牛肉", "羊肉", "鸡肉", "鸭肉", "禽肉":
		return design.DishToneMeat
	case "鱼类", "虾蟹贝", "其他海鲜":
		return design.DishToneSea
	case "蔬菜", "菌菇":
		return design.DishToneVeg
	case "蛋类", "豆制品":
		return design.DishToneEgg
	case "主食", "面点":
		return design.DishToneGrain
	}
	return design.DishToneOther
}

// markIcon reuses the navigation set wherever its shape already says the
// tone: a flame for hot dishes' meat, a bowl for staples, a leaf for greens.
// Only the fish and the egg are drawn for this.
func markIcon(tone design.DishTone) *ui.SVG {
	switch tone {
	case design.DishToneMeat:
		return &icons.Flame
	case design.DishToneSea:
		return &icons.Fish
	case design.DishToneVeg:
		return &icons.Leaf
	case design.DishToneEgg:
		return &icons.Egg
	case design.DishToneGrain:
		return &icons.Bowl
	}
	return &icons.Recipe
}

// recipeMarkTile is the icon tile: the dish tone behind, the mark in ink.
// It is what a card and its detail page share.
func (a *App) recipeMarkTile(c *ui.Context, p design.Palette, rec domain.Recipe, size float32) ui.Element {
	tone := markTone(rec)
	tile := ui.Box(c).Size(size, size).Radius(design.RadiusSmall).
		Background(design.DishWash(p, tone)).Center().Shrink(0)
	tile.Children(func() {
		ui.Icon(c, markIcon(tone)).Size(size*0.6, size*0.6).TextColor(p.Ink)
	})
	return tile
}
