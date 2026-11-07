package sichu_test

// End-to-end checks against the real library and the real nutrition table:
// the private kitchen plan, its structure validation and the per-dish energy
// estimate are the three packages that only meet here.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

func loadRecipes(t *testing.T) []domain.Recipe {
	t.Helper()
	b, err := assets.Data("recipes.json")
	if err != nil {
		t.Fatal(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(b, &raws); err != nil {
		t.Fatal(err)
	}
	out := make([]domain.Recipe, len(raws))
	for i := range raws {
		if err := json.Unmarshal(raws[i], &out[i]); err != nil {
			t.Fatalf("recipes.json[%d]: %v", i, err)
		}
		out[i].ID = int64(i + 1)
		// isDish defaults to true: only an explicit false marks a seasoning.
		var probe struct {
			IsDish *bool `json:"isDish"`
		}
		if err := json.Unmarshal(raws[i], &probe); err == nil {
			out[i].IsDish = probe.IsDish == nil || *probe.IsDish
		}
	}
	return out
}

func realCatalog(t *testing.T) *nutrition.Catalog {
	t.Helper()
	cat, err := nutrition.NewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if cat.Size() < 100 {
		t.Fatalf("nutrition catalog has %d entries", cat.Size())
	}
	return cat
}

func comboInput(t *testing.T) sichu.PlanInput {
	return sichu.PlanInput{
		Recipes:  loadRecipes(t),
		Catalog:  realCatalog(t),
		Now:      time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local),
		Style:    sichu.StyleCombo,
		Meals:    []string{"午餐"},
		Servings: 2,
	}
}

// A two-dish-one-soup plan over the real 944 dishes validates and carries the
// energy estimate the nutrition table computes.
func TestRealLibraryPlanHasNutrition(t *testing.T) {
	in := comboInput(t)
	plan, err := sichu.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals) != 1 {
		t.Fatalf("meals = %d", len(plan.Meals))
	}
	if n := len(plan.Meals[0].Dishes); n != 3 {
		t.Fatalf("dishes = %d, want 3", n)
	}
	plan = sichu.Enrich(plan, in)
	if err := sichu.Validate(plan, in); err != nil {
		t.Fatalf("plan does not validate: %v\n%s", err, sichu.Describe(plan))
	}
	total := 0
	for _, d := range plan.Meals[0].Dishes {
		if d.RecipeID == 0 {
			t.Errorf("local policy produced a draft: %q", d.Name)
		}
		if d.Kcal <= 0 {
			t.Errorf("%q has no energy estimate", d.Name)
		}
		if d.ProteinG <= 0 {
			t.Errorf("%q has no protein estimate", d.Name)
		}
		total += d.Kcal
	}
	t.Logf("plan: %s (合计约 %d kcal)", sichu.Describe(plan), total)
}

// 清真 removes pork from the pool: no dish may list pork among its
// ingredients.
func TestRealLibraryHalalPlanHasNoPork(t *testing.T) {
	in := comboInput(t)
	in.Profile = domain.SichuProfile{Restrictions: "清真"}
	plan, err := sichu.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	plan = sichu.Enrich(plan, in)
	if err := sichu.Validate(plan, in); err != nil {
		t.Fatalf("halal plan does not validate: %v\n%s", err, sichu.Describe(plan))
	}
	byID := make(map[int64]domain.Recipe, len(in.Recipes))
	for _, r := range in.Recipes {
		byID[r.ID] = r
	}
	for _, d := range plan.Meals[0].Dishes {
		rec, ok := byID[d.RecipeID]
		if !ok {
			t.Fatalf("dish %q references missing recipe %d", d.Name, d.RecipeID)
		}
		for _, ing := range rec.Ingredients {
			if strings.Contains(ing.Name, "猪") {
				t.Errorf("halal plan picked %q with 猪 ingredient %q", rec.Name, ing.Name)
			}
		}
	}
}

// A one-bowl plan is one self-sufficient core, optionally one soup.
func TestRealLibraryOneBowlPlan(t *testing.T) {
	in := comboInput(t)
	in.Style = sichu.StyleOneBowl
	plan, err := sichu.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals) != 1 {
		t.Fatalf("meals = %d", len(plan.Meals))
	}
	dishes := plan.Meals[0].Dishes
	if len(dishes) < 1 || len(dishes) > 2 {
		t.Fatalf("one-bowl dishes = %d, want 1..2", len(dishes))
	}
	plan = sichu.Enrich(plan, in)
	if err := sichu.Validate(plan, in); err != nil {
		t.Fatalf("one-bowl plan does not validate: %v\n%s", err, sichu.Describe(plan))
	}
	t.Logf("one bowl: %s", sichu.Describe(plan))
}

// The planner is deterministic: the same input twice gives the same plan.
func TestRealLibraryPlanDeterministic(t *testing.T) {
	in := comboInput(t)
	first, err := sichu.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sichu.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Errorf("plans differ:\n%s\n%s", a, b)
	}
}
