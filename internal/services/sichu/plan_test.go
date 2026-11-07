package sichu

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func minutes(v float64) *float64 { return &v }

// testRecipe builds a library dish with required, non-seasoning ingredients.
func testRecipe(id int64, name, cuisine, category string, ingredients ...string) domain.Recipe {
	lines := make([]domain.Ingredient, 0, len(ingredients))
	for _, ing := range ingredients {
		lines = append(lines, domain.Ingredient{Name: ing, Amount: 100, Unit: "g", Required: true})
	}
	return domain.Recipe{
		ID: id, Name: name, Cuisine: cuisine, MainCategory: category,
		DurationMinutes: minutes(20), BaseServings: 2, IsDish: true,
		VersionGroup: name, Ingredients: lines,
	}
}

func testRecipes() []domain.Recipe {
	out := []domain.Recipe{
		testRecipe(1, "青椒肉丝", "家常菜", "猪肉", "猪肉", "青椒"),
		testRecipe(2, "香煎鸡胸", "快手菜", "鸡肉", "鸡胸肉", "黑胡椒"),
		testRecipe(3, "红烧肉", "炖菜", "猪肉", "五花肉", "冰糖"),
		testRecipe(4, "清蒸鲈鱼", "蒸菜", "鱼类", "鲈鱼", "葱"),
		testRecipe(5, "蒜蓉西兰花", "快手菜", "蔬菜", "西兰花", "大蒜"),
		testRecipe(6, "香菇青菜", "快炒", "菌菇", "香菇", "青菜"),
		testRecipe(7, "醋溜白菜", "快手菜", "蔬菜", "白菜", "醋"),
		testRecipe(8, "番茄蛋花汤", "汤羹", "蛋类", "番茄", "鸡蛋"),
		testRecipe(9, "紫菜蛋花汤", "汤羹", "蛋类", "紫菜", "鸡蛋"),
		testRecipe(10, "冬瓜排骨汤", "汤羹", "猪肉", "冬瓜", "排骨"),
		testRecipe(11, "黄金蛋炒饭", "快手菜", "主食", "米饭", "鸡蛋", "胡萝卜"),
		testRecipe(12, "番茄牛肉面", "家常菜", "主食", "面条", "牛肉", "番茄"),
		testRecipe(13, "皮蛋瘦肉粥", "家常菜", "主食", "大米", "皮蛋", "瘦肉"),
		testRecipe(14, "口水鸡（川味）", "凉菜", "鸡肉", "鸡腿", "辣椒"),
		testRecipe(15, "口水鸡（粤味）", "凉菜", "鸡肉", "鸡腿", "芝麻"),
		testRecipe(16, "万能蘸酱", "凉菜", "蔬菜", "酱油", "蒜"),
		testRecipe(17, "辣子鸡", "快手菜", "鸡肉", "鸡腿", "干辣椒"),
		testRecipe(18, "宫保鸡丁", "快手菜", "鸡肉", "鸡腿", "花生米"),
		testRecipe(19, "咸菜炒肉", "快手菜", "猪肉", "咸菜", "猪肉"),
		testRecipe(20, "猪肝汤", "汤羹", "猪肉", "猪肝", "菠菜"),
		testRecipe(21, "番茄豆腐汤", "汤羹", "豆制品", "番茄", "豆腐"),
		testRecipe(22, "冬瓜海带汤", "汤羹", "蔬菜", "冬瓜", "海带"),
		testRecipe(23, "清炒时蔬", "家常菜", "蔬菜", "时蔬"),
		testRecipe(24, "家常豆腐", "家常菜", "豆制品", "豆腐", "青椒"),
	}
	// The sauce is a non-dish entry: the planner must never offer it.
	out[15].IsDish = false
	// The two 口水鸡 entries are variants of one version group.
	out[13].VersionGroup = "口水鸡"
	out[14].VersionGroup = "口水鸡"
	return out
}

func testInput() PlanInput {
	return PlanInput{
		Recipes:  testRecipes(),
		Now:      time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
		Style:    StyleCombo,
		Meals:    []string{"午餐"},
		Servings: 2,
		Profile:  domain.SichuProfile{ExploreRatio: 0.3},
	}
}

// The same input always plans the same meals, whatever order the library
// arrives in.
func TestPlanDeterministic(t *testing.T) {
	in := testInput()
	first, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plan is not deterministic:\n%+v\n%+v", first, second)
	}

	shuffled := testInput()
	reversed := make([]domain.Recipe, len(shuffled.Recipes))
	for i, r := range shuffled.Recipes {
		reversed[len(reversed)-1-i] = r
	}
	shuffled.Recipes = reversed
	third, err := Plan(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, third) {
		t.Fatalf("plan depends on library order:\n%+v\n%+v", first, third)
	}
}

// A two-dish soup meal has exactly the three required slots and passes
// validation.
func TestPlanStructure(t *testing.T) {
	plan, err := Plan(testInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals) != 1 {
		t.Fatalf("meals = %d", len(plan.Meals))
	}
	meal := plan.Meals[0]
	if meal.Title != "午餐" {
		t.Errorf("title = %q", meal.Title)
	}
	if len(meal.Dishes) != 3 {
		t.Fatalf("dishes = %d: %+v", len(meal.Dishes), meal.Dishes)
	}
	if meal.Dishes[0].Slot != SlotMain || meal.Dishes[1].Slot != SlotVeg ||
		meal.Dishes[2].Slot != SlotSoup {
		t.Errorf("slots = %v %v %v",
			meal.Dishes[0].Slot, meal.Dishes[1].Slot, meal.Dishes[2].Slot)
	}
	if err := Validate(plan, testInput()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// A one-bowl meal is a core dish plus an optional soup.
func TestPlanOneBowl(t *testing.T) {
	in := testInput()
	in.Style = StyleOneBowl
	plan, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	meal := plan.Meals[0]
	if len(meal.Dishes) < 1 || len(meal.Dishes) > 2 {
		t.Fatalf("one-bowl dishes = %d", len(meal.Dishes))
	}
	if !IsOneBowlName(meal.Dishes[0].Name) {
		t.Errorf("core is not a one-bowl dish: %q", meal.Dishes[0].Name)
	}
	if len(meal.Dishes) == 2 && !strings.Contains(meal.Dishes[1].Name, "汤") {
		t.Errorf("second dish should be a soup: %q", meal.Dishes[1].Name)
	}
	if err := Validate(plan, in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// Hard limits really remove dishes, not just rank them lower.
func TestPlanHardConstraints(t *testing.T) {
	cases := []struct {
		name string
		set  func(*PlanInput)
		bad  []string
	}{
		{
			name: "清真", bad: []string{"猪", "排骨", "五花"},
			set: func(in *PlanInput) {
				in.Profile.Restrictions = "halal"
			},
		},
		{
			name: "吃斋", bad: []string{"肉", "鸡", "鱼", "蛋", "虾"},
			set: func(in *PlanInput) {
				in.Profile.Restrictions = "vegetarian"
			},
		},
		{
			name: "痛风", bad: []string{"猪肝", "浓汤"},
			set: func(in *PlanInput) {
				in.Profile.HealthTags = "gout"
			},
		},
		{
			name: "过敏", bad: []string{"花生"},
			set: func(in *PlanInput) {
				in.Profile.Allergens = "peanut"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := testInput()
			in.Profile.ExploreRatio = 0.3
			tc.set(&in)
			for _, rec := range LegalRecipes(in) {
				for _, bad := range tc.bad {
					if strings.Contains(rec.Name, bad) {
						t.Errorf("candidate pool contains %q: %q", bad, rec.Name)
					}
				}
			}
			plan, err := Plan(in)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if err := Validate(plan, in); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

// A dish the user is allergic to via categories never appears.
func TestPlanAllergenByIngredient(t *testing.T) {
	in := testInput()
	// 花生 is in 宫保鸡丁's ingredient list; it can never be a soup or veg,
	// so force the main slot pool to matter: give every protein dish peanuts
	// except one.
	for i := range in.Recipes {
		if strings.Contains(in.Recipes[i].Name, "鸡") {
			in.Recipes[i].Ingredients = append(in.Recipes[i].Ingredients,
				domain.Ingredient{Name: "花生米", Required: true})
		}
	}
	in.Profile.Allergens = "peanut"
	plan, err := Plan(in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, dish := range plan.Meals[0].Dishes {
		if strings.Contains(dish.Name, "宫保鸡丁") || strings.Contains(dish.Name, "辣子鸡") {
			t.Errorf("allergen dish planned: %q", dish.Name)
		}
	}
}

// Variants of one version group never both appear, and dishes never repeat
// across meals.
func TestPlanNoDuplicateVersions(t *testing.T) {
	in := testInput()
	in.Meals = []string{"午餐", "晚餐"}
	plan, err := Plan(in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	seen := make(map[string]bool)
	for _, meal := range plan.Meals {
		for _, dish := range meal.Dishes {
			group := dish.VersionGroup
			if group == "" {
				group = NormalizeVersionGroup(dish.Name)
			}
			if seen[group] {
				t.Errorf("version group %q appears twice", group)
			}
			seen[group] = true
			if strings.Contains(dish.Name, "口水鸡") && dish.VersionGroup != "口水鸡" {
				t.Errorf("version group not filled: %+v", dish)
			}
		}
	}
}

// Non-dish entries (sauces) never enter a plan.
func TestPlanSkipsNonDishes(t *testing.T) {
	in := testInput()
	in.Style = StyleOneBowl
	plan, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, meal := range plan.Meals {
		for _, dish := range meal.Dishes {
			if dish.Name == "万能蘸酱" {
				t.Fatalf("non-dish planned: %+v", dish)
			}
		}
	}
}

// The explore share changes which dishes are picked, visibly.
func TestPlanExploreRatio(t *testing.T) {
	safe := testInput()
	safe.ExploreRatio = 0
	safe.Favorites = map[int64]bool{2: true, 5: true, 8: true}
	safePlan, err := Plan(safe)
	if err != nil {
		t.Fatal(err)
	}
	favoritePicks := 0
	for _, dish := range safePlan.Meals[0].Dishes {
		if safe.Favorites[dish.RecipeID] {
			favoritePicks++
		}
	}
	if favoritePicks != 3 {
		t.Errorf("safe plan should favour favorites, got %d of 3: %v",
			favoritePicks, dishNames(safePlan))
	}

	explore := testInput()
	explore.ExploreRatio = 1
	explore.Favorites = safe.Favorites
	explorePlan, err := Plan(explore)
	if err != nil {
		t.Fatal(err)
	}
	for _, dish := range explorePlan.Meals[0].Dishes {
		if explore.Favorites[dish.RecipeID] {
			t.Errorf("explore plan kept a favorite: %q", dish.Name)
		}
	}
	if reflect.DeepEqual(safePlan, explorePlan) {
		t.Fatal("explore ratio had no effect")
	}
}

// Replace swaps one dish without touching the others.
func TestReplaceDish(t *testing.T) {
	in := testInput()
	plan, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	before := dishNames(plan)
	next, err := Replace(in, plan, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	after := dishNames(next)
	if after[0] == before[0] {
		t.Errorf("dish did not change: %v", after)
	}
	if after[1] != before[1] || after[2] != before[2] {
		t.Errorf("other dishes changed: %v -> %v", before, after)
	}
	if err := Validate(next, in); err != nil {
		t.Fatalf("Validate after replace: %v", err)
	}
}

// The daily line count and slot shape are stable for a two-meal plan.
func TestPlanTwoMeals(t *testing.T) {
	in := testInput()
	in.Meals = []string{"午餐", "晚餐"}
	plan, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Meals) != 2 {
		t.Fatalf("meals = %d", len(plan.Meals))
	}
	if got := Describe(plan); !strings.Contains(got, "2 餐") || !strings.Contains(got, "6 道菜") {
		t.Errorf("Describe = %q", got)
	}
}

// A new batch excludes the previous batch's dishes and version groups.
func TestPlanRegenerateChangesDishes(t *testing.T) {
	in := testInput()
	first, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	in.ExcludeIDs = make(map[int64]bool)
	in.ExcludeGroups = make(map[string]bool)
	for _, meal := range first.Meals {
		for _, dish := range meal.Dishes {
			if dish.RecipeID != 0 {
				in.ExcludeIDs[dish.RecipeID] = true
			}
			in.ExcludeGroups[dish.VersionGroup] = true
		}
	}
	second, err := Plan(in)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	seen := make(map[string]bool)
	for _, name := range dishNames(first) {
		seen[NormalizeVersionGroup(name)] = true
	}
	for _, meal := range second.Meals {
		for _, dish := range meal.Dishes {
			if seen[NormalizeVersionGroup(dish.Name)] {
				t.Errorf("new batch repeats %q", dish.Name)
			}
		}
	}
	if err := Validate(second, in); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func dishNames(plan MealPlan) []string {
	out := make([]string, 0, 6)
	for _, meal := range plan.Meals {
		for _, dish := range meal.Dishes {
			out = append(out, dish.Name)
		}
	}
	return out
}

// A custom structure is the diner's counts, slot for slot: two proteins, one
// vegetable and no soup is a legal meal, and the contract checks the same
// numbers.
func TestPlanCustomCounts(t *testing.T) {
	in := testInput()
	in.Style = StyleCustom
	in.Counts = Counts{Mains: 2, Vegs: 1}
	plan, err := Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	meal := plan.Meals[0]
	if len(meal.Dishes) != 3 {
		t.Fatalf("dishes = %d: %+v", len(meal.Dishes), meal.Dishes)
	}
	slots := []Slot{meal.Dishes[0].Slot, meal.Dishes[1].Slot, meal.Dishes[2].Slot}
	want := []Slot{SlotMain, SlotMain, SlotVeg}
	for i := range want {
		if slots[i] != want[i] {
			t.Fatalf("slots = %v, want %v", slots, want)
		}
	}
	if err := Validate(plan, in); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// A plan whose counts do not match is rejected whole.
	bad := plan
	bad.Meals = append([]Meal(nil), plan.Meals...)
	bad.Meals[0].Dishes = bad.Meals[0].Dishes[:2]
	if err := Validate(bad, in); err == nil {
		t.Fatal("a custom plan with the wrong dish count passed validation")
	}
}

// An empty custom structure is refused rather than silently filled.
func TestPlanCustomNeedsADish(t *testing.T) {
	in := testInput()
	in.Style = StyleCustom
	in.Counts = Counts{}
	if _, err := Plan(in); err == nil {
		t.Fatal("an empty custom meal was planned")
	}
}
