package sichu

import (
	"strings"
	"testing"
)

func mustPlan(t *testing.T, in PlanInput) MealPlan {
	t.Helper()
	plan, err := Plan(in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return plan
}

// A meal with a missing slot, an extra dish or the wrong role is rejected
// whole, not patched.
func TestValidateStructure(t *testing.T) {
	in := testInput()
	plan := mustPlan(t, in)

	t.Run("缺一道", func(t *testing.T) {
		broken := clonePlan(plan)
		broken.Meals[0].Dishes = broken.Meals[0].Dishes[:2]
		if err := Validate(broken, in); err == nil {
			t.Fatal("expected an error for a two-dish combo")
		}
	})

	t.Run("多一道", func(t *testing.T) {
		broken := clonePlan(plan)
		broken.Meals[0].Dishes = append(broken.Meals[0].Dishes,
			broken.Meals[0].Dishes[1])
		if err := Validate(broken, in); err == nil {
			t.Fatal("expected an error for a four-dish combo")
		}
	})

	t.Run("汤位被蔬菜顶替", func(t *testing.T) {
		broken := clonePlan(plan)
		for _, rec := range in.Recipes {
			if rec.Name == "醋溜白菜" {
				broken.Meals[0].Dishes[2] = Dish{RecipeID: rec.ID, Name: rec.Name, Slot: SlotSoup}
				break
			}
		}
		err := Validate(broken, in)
		if err == nil || !strings.Contains(err.Error(), "汤") {
			t.Fatalf("expected a soup-slot error, got %v", err)
		}
	})
}

// Duplicates across meals are rejected.
func TestValidateDuplicateDish(t *testing.T) {
	in := testInput()
	in.Meals = []string{"午餐", "晚餐"}
	plan := mustPlan(t, in)
	plan.Meals[1].Dishes[0] = plan.Meals[0].Dishes[0]
	if err := Validate(plan, in); err == nil {
		t.Fatal("expected a duplicate error")
	}
}

// A hard-limit violation smuggled into a plan is caught even when the local
// planner would never produce it.
func TestValidateConstraintViolation(t *testing.T) {
	in := testInput()
	in.Profile.Restrictions = "halal"
	plan := mustPlan(t, in)
	for _, rec := range in.Recipes {
		if rec.Name == "红烧肉" {
			plan.Meals[0].Dishes[0] = Dish{RecipeID: rec.ID, Name: rec.Name, Slot: SlotMain}
			break
		}
	}
	err := Validate(plan, in)
	if err == nil || !strings.Contains(err.Error(), "清真") {
		t.Fatalf("expected a halal error, got %v", err)
	}
}

// A sauce is not a dish and cannot fill a slot.
func TestValidateRejectsNonDish(t *testing.T) {
	in := testInput()
	plan := mustPlan(t, in)
	for _, rec := range in.Recipes {
		if rec.Name == "万能蘸酱" {
			plan.Meals[0].Dishes[1] = Dish{RecipeID: rec.ID, Name: rec.Name, Slot: SlotVeg}
			break
		}
	}
	err := Validate(plan, in)
	if err == nil || !strings.Contains(err.Error(), "不是菜式") {
		t.Fatalf("expected a non-dish error, got %v", err)
	}
}

// A proposed new dish is accepted only when it is complete and safe.
func TestValidateDraft(t *testing.T) {
	in := testInput()
	in.Style = StyleOneBowl
	draft := &Draft{
		Name: "蒜蓉秋葵", Cuisine: "家常菜", MainCategory: "蔬菜",
		BaseServings: 2, DurationMinutes: 10,
		Ingredients: []DraftIngredient{{Name: "秋葵"}, {Name: "大蒜", Seasoning: true}},
		Steps:       []DraftStep{{Instruction: "焯水后快炒"}},
	}
	plan := MealPlan{Meals: []Meal{{
		Title: "午餐", Style: StyleOneBowl,
		Dishes: []Dish{{Name: "蒜蓉秋葵盖饭", Draft: draft}},
	}}}
	// A draft still has to fit the one-bowl shape, so name it like one.
	plan.Meals[0].Dishes[0].Name = "蒜蓉秋葵盖饭"
	enriched := Enrich(plan, in)
	if err := Validate(enriched, in); err != nil {
		t.Fatalf("valid draft rejected: %v", err)
	}

	broken := Enrich(plan, in)
	broken.Meals[0].Dishes[0].Draft = &Draft{Name: "蒜蓉秋葵"}
	if err := Validate(broken, in); err == nil {
		t.Fatal("expected an error for a draft without ingredients and steps")
	}
}

// Enrich fills an AI answer's bare ids with library data.
func TestEnrichFillsLibraryFields(t *testing.T) {
	in := testInput()
	plan := MealPlan{Meals: []Meal{{
		Title: "午餐",
		Dishes: []Dish{
			{RecipeID: 1}, {RecipeID: 5}, {RecipeID: 8},
		},
	}}}
	got := Enrich(plan, in)
	for _, dish := range got.Meals[0].Dishes {
		if dish.Name == "" {
			t.Errorf("name not filled: %+v", dish)
		}
		if dish.DurationMinutes == 0 {
			t.Errorf("duration not filled: %+v", dish)
		}
		if dish.Slot == "" {
			t.Errorf("slot not filled: %+v", dish)
		}
		if dish.VersionGroup == "" {
			t.Errorf("version group not filled: %+v", dish)
		}
	}
	if got.Meals[0].Style != StyleCombo {
		t.Errorf("style not filled: %q", got.Meals[0].Style)
	}
	if err := Validate(got, in); err != nil {
		t.Fatalf("enriched plan should validate: %v", err)
	}
}

// A one-bowl meal rejects a second non-soup dish.
func TestValidateOneBowlSecondDish(t *testing.T) {
	in := testInput()
	in.Style = StyleOneBowl
	plan := mustPlan(t, in)
	if len(plan.Meals[0].Dishes) != 2 {
		t.Skip("library planned a core without soup")
	}
	plan.Meals[0].Dishes[1] = plan.Meals[0].Dishes[0]
	err := Validate(plan, in)
	if err == nil || !strings.Contains(err.Error(), "汤") {
		t.Fatalf("expected a one-bowl soup error, got %v", err)
	}
}

// Validate reports an empty plan clearly.
func TestValidateEmptyPlan(t *testing.T) {
	if err := Validate(MealPlan{}, testInput()); err == nil {
		t.Fatal("expected an error for an empty plan")
	}
}
