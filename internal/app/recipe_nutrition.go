package app

// The dish nutrition cache. Estimates are read by the recipe list's trailing
// column and the detail hero, and they are cheap per dish (a dozen table
// lookups), so they are computed lazily for the dishes actually drawn rather
// than for all 944 on every filter keystroke.

import (
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
)

// dishNutrition is one dish's per-serving estimate plus the purine tier of
// its worst ingredient. The two disagree by design: mg is weighed per
// serving, the tier is the highest band any ingredient reaches.
type dishNutrition struct {
	Nutrients nutrition.Nutrients
	Purine    nutrition.PurineLevel
}

// dishNut returns the estimate, or false while the catalog is still loading.
func (a *App) dishNut(rec domain.Recipe) (dishNutrition, bool) {
	if hit, ok := a.dishNutCache[rec.ID]; ok {
		return hit, true
	}
	cat := a.nutrition.catalog
	if cat == nil {
		return dishNutrition{}, false
	}
	names := make([]string, 0, len(rec.Ingredients))
	for _, in := range rec.Ingredients {
		names = append(names, in.Name)
	}
	got := dishNutrition{Nutrients: cat.Recipe(rec), Purine: cat.DishLevel(names)}
	if a.dishNutCache == nil {
		a.dishNutCache = make(map[int64]dishNutrition, len(a.recipes))
	}
	a.dishNutCache[rec.ID] = got
	return got, true
}

// clearDishNutCache drops the estimates with the dish snapshot they were
// built from, so an edited dish is never shown with its old numbers.
func (a *App) clearDishNutCache() {
	a.dishNutCache = make(map[int64]dishNutrition, len(a.recipes))
}
