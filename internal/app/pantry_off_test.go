package app

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The pantry switch survives a restart, and a database with the pantry off
// opens on 菜式 instead of the hidden home.
func TestPantryOffPrefsRoundTrip(t *testing.T) {
	store := openM6Store(t)
	ctx := context.Background()
	a := newTestApp()
	a.store = store
	a.setPantryOff(true)
	waitFor(t, "pantry_off written", func() bool {
		raw, err := store.MetaGet(ctx, "prefs")
		return err == nil && strings.Contains(raw, `"pantry_off":true`)
	})

	b := New()
	b.phase = PhaseReady
	if err := b.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pantry_off loaded", func() bool { return b.pantryOff })
	if b.router.Path() != "/recipes" {
		t.Errorf("initial route = %q, want /recipes", b.router.Path())
	}
}

// With the pantry off the shelf, the stock filter, the stock column and the
// stock words on a dish page all go away.
func TestPantryOffHidesRecipeStock(t *testing.T) {
	a := newTestApp()
	a.setPantryOff(true)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	for _, gone := range []string{"能做(", "差一点(", "库存"} {
		if tt.HasText(gone) {
			t.Errorf("recipes page still shows %q: %q", gone, tt.Texts())
		}
	}
	rec := a.recipeView[0]
	if err := tt.Click(rec.Name); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	for _, gone := range []string{"在库", "加入购物清单"} {
		if tt.HasText(gone) {
			t.Errorf("dish page still shows %q: %q", gone, tt.Texts())
		}
	}
}

// The private kitchen works from the whole recipe library when there is no
// pantry, and its stock controls disappear.
func TestPantryOffSichuUsesFullLibrary(t *testing.T) {
	a := newSichuTestApp()
	a.setPantryOff(true)
	a.setItems(nil)
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready with an empty pantry: %q", a.sichu.planError)
	}
	total := 0
	for _, meal := range a.sichu.plan.Meals {
		total += len(meal.Dishes)
	}
	if total == 0 {
		t.Fatal("the plan is empty")
	}
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if tt.HasText("库存策略") {
		t.Errorf("sichu still offers a stock policy: %q", tt.Texts())
	}
}

// With the pantry closed, settlement has no deduction preview.
func TestPantryOffSettlePreviewHasNoStock(t *testing.T) {
	a := m4TestApp()
	a.setPantryOff(true)
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	a.openSettleConfirm()
	if len(a.sched.preview.Changes) != 0 {
		t.Fatalf("preview still deducts stock: %+v", a.sched.preview.Changes)
	}
	tt.Frame()
	settle(tt)
	if !tt.HasText("仓库页已关闭") {
		t.Errorf("settlement sheet does not explain the closed pantry: %q", tt.Texts())
	}
}

// A route under /pantry lands on the recipes page instead.
func TestPantryOffRoutesRedirect(t *testing.T) {
	a := newTestApp()
	a.setPantryOff(true)
	a.router.Push("/pantry")
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	if a.router.Path() != "/recipes" {
		t.Fatalf("path = %q, want /recipes", a.router.Path())
	}
	if !tt.HasText("新建菜式") {
		t.Errorf("recipes page did not render: %q", tt.Texts())
	}
}
