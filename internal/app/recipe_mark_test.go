package app

// Tests for the dish mark and the detail hero: the category collapse, the
// per-serving column, the hero's readings, and the cache that keeps them off
// the frame path.

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func TestMarkToneCoversEverySeedCategory(t *testing.T) {
	cases := map[string]design.DishTone{
		"猪肉": design.DishToneMeat, "牛肉": design.DishToneMeat, "羊肉": design.DishToneMeat,
		"鸡肉": design.DishToneMeat, "鸭肉": design.DishToneMeat, "禽肉": design.DishToneMeat,
		"鱼类": design.DishToneSea, "虾蟹贝": design.DishToneSea, "其他海鲜": design.DishToneSea,
		"蔬菜": design.DishToneVeg, "菌菇": design.DishToneVeg,
		"蛋类": design.DishToneEgg, "豆制品": design.DishToneEgg,
		"主食": design.DishToneGrain, "面点": design.DishToneGrain,
		"":    design.DishToneOther,
		"腌制品": design.DishToneOther,
	}
	for category, want := range cases {
		if got := markTone(domain.Recipe{MainCategory: category}); got != want {
			t.Errorf("markTone(%q) = %v, want %v", category, got, want)
		}
	}
}

// Every tone must have an icon, or a dish renders as an empty tile.
func TestMarkIconNeverNil(t *testing.T) {
	for _, tone := range []design.DishTone{
		design.DishToneOther, design.DishToneMeat, design.DishToneSea,
		design.DishToneVeg, design.DishToneEgg, design.DishToneGrain,
	} {
		if markIcon(tone) == nil {
			t.Errorf("markIcon(%v) is nil", tone)
		}
	}
}

// withCatalog loads the nutrition catalog the way bootstrap does, so the
// pages can show their estimates.
func withCatalog(t testing.TB, a *App) {
	t.Helper()
	if err := a.loadNutritionState(context.Background(), nil); err != nil {
		t.Fatalf("load nutrition catalog: %v", err)
	}
}

// The recipe row closes with the per-serving estimate once the catalog is
// read, and shows nothing rather than a wrong number before that.
func TestRecipeRowKcalColumn(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	if hasTextContaining(tt, "458") {
		t.Fatal("the row showed an estimate before the catalog was read")
	}

	withCatalog(t, a)
	tt.Frame()
	settle(tt)
	// 青椒肉丝 is the only fixture dish with weighed ingredients.
	if !hasTextContaining(tt, "458") {
		t.Fatalf("the row lost its estimate: %q", tt.Texts())
	}

	// An edit re-seeds the snapshot, so the cache must not answer from the
	// old ingredient lines.
	before := len(a.dishNutCache)
	rec := a.recipeByID[1]
	rec.Ingredients = append(rec.Ingredients, domain.Ingredient{Name: "土豆", Amount: 200, Unit: "g"})
	recipes := append([]domain.Recipe{}, a.recipes...)
	for i, r := range recipes {
		if r.ID == 1 {
			recipes[i] = rec
		}
	}
	a.setRecipes(recipes, a.steps)
	if len(a.dishNutCache) != 0 || before == 0 {
		t.Fatalf("snapshot change left %d cached dishes (was %d)", len(a.dishNutCache), before)
	}
}

// The detail hero states what it is estimating, how well the table covered
// the dish, and what the pantry still owes.
func TestRecipeHeroCard(t *testing.T) {
	a := newTestApp()
	withCatalog(t, a)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	for _, want := range []string{"营养估算 · 每 1 人份", "食材表全覆盖", "必需食材", "1 / 2 在库", "嘌呤", "蛋白", "脂肪", "碳水"} {
		if !tt.HasText(want) {
			t.Errorf("hero missing %q: %q", want, tt.Texts())
		}
	}
	// The pills that moved out of the card stay beside the title.
	for _, want := range []string{"4 步", "难度 简单", "全年"} {
		if !tt.HasText(want) {
			t.Errorf("detail missing %q: %q", want, tt.Texts())
		}
	}
	// A settled hero asks for no frames: the ring is painted, not animated.
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("the settled recipe hero keeps asking for frames")
	}
}

// Before the catalog arrives the hero must not invent numbers.
func TestRecipeHeroWithoutCatalog(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("营养目录读取中") {
		t.Fatalf("hero did not say the catalog is loading: %q", tt.Texts())
	}
	if tt.HasText("kcal") {
		t.Errorf("hero showed an estimate with no catalog: %q", tt.Texts())
	}
}
