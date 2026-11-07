package sichu

import (
	"strings"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func TestPreferencesFromMergesDiners(t *testing.T) {
	profile := domain.SichuProfile{
		HealthTags:   "diabetes",
		Allergens:    "peanut",
		Restrictions: "halal",
		ExploreRatio: 0.5,
	}
	questionnaire := domain.UserProfile{
		Spicy: "mild", Saltiness: "light",
	}
	diners := []domain.MemberProfile{
		{Name: "小满", HealthTags: "gout", Allergens: "shellfish"},
	}
	prefs := PreferencesFrom(profile, questionnaire, diners)
	if !prefs.Halal {
		t.Error("halal not merged")
	}
	if got := strings.Join(prefs.HealthTags, ","); got != "diabetes,gout" {
		t.Errorf("health tags = %q", got)
	}
	if got := strings.Join(prefs.Allergens, ","); got != "peanut,shellfish" {
		t.Errorf("allergens = %q", got)
	}
	if prefs.Spicy != "mild" || prefs.Salt != "light" {
		t.Errorf("taste = %q/%q", prefs.Spicy, prefs.Salt)
	}
}

func TestPreferencesFallsBackToQuestionnaire(t *testing.T) {
	q := domain.UserProfile{Restrictions: "vegetarian", AllergyNote: "芒果、香菜"}
	prefs := PreferencesFrom(domain.SichuProfile{}, q, nil)
	if !prefs.Vegetarian {
		t.Error("questionnaire restriction not used")
	}
	if got := strings.Join(prefs.AllergyNote, ","); got != "芒果,香菜" {
		t.Errorf("allergy note = %q", got)
	}
}

// Hard limits are safety boundaries: an obvious ingredient is enough to
// reject the dish.
func TestDishViolation(t *testing.T) {
	cases := []struct {
		name     string
		prefs    Preferences
		dish     string
		ings     []string
		wantPart string
	}{
		{
			name: "清真见猪肉", prefs: Preferences{Halal: true},
			dish: "青椒肉丝", ings: []string{"猪肉", "青椒"}, wantPart: "清真",
		},
		{
			name: "清真见五花", prefs: Preferences{Halal: true},
			dish: "红烧肉", ings: []string{"五花肉", "冰糖"}, wantPart: "清真",
		},
		{
			name: "吃斋见鸡蛋", prefs: Preferences{Vegetarian: true},
			dish: "番茄炒蛋", ings: []string{"番茄", "鸡蛋"}, wantPart: "吃斋",
		},
		{
			name: "吃斋放行豆腐", prefs: Preferences{Vegetarian: true},
			dish: "家常豆腐", ings: []string{"豆腐", "青椒"},
		},
		{
			name: "糖尿病见糖醋", prefs: Preferences{HealthTags: []string{"diabetes"}},
			dish: "糖醋排骨", ings: []string{"排骨", "糖"},
			wantPart: "糖尿病",
		},
		{
			name: "糖尿病放行青菜", prefs: Preferences{HealthTags: []string{"diabetes"}},
			dish: "蒜蓉青菜", ings: []string{"青菜", "大蒜"},
		},
		{
			name: "痛风见猪肝", prefs: Preferences{HealthTags: []string{"gout"}},
			dish: "猪肝汤", ings: []string{"猪肝"}, wantPart: "痛风",
		},
		{
			name: "高血压见咸菜", prefs: Preferences{HealthTags: []string{"hypertension"}},
			dish: "咸菜炒肉", ings: []string{"咸菜", "猪肉"}, wantPart: "高血压",
		},
		{
			name: "高血脂见油炸", prefs: Preferences{HealthTags: []string{"hyperlipidemia"}},
			dish: "炸鸡", ings: []string{"鸡腿", "食用油"}, wantPart: "高血脂",
		},
		{
			name: "过敏源按调料也算", prefs: Preferences{Allergens: []string{"soy"}},
			dish: "清炒青菜", ings: []string{"青菜", "酱油"}, wantPart: "过敏",
		},
		{
			name: "自填过敏源", prefs: Preferences{AllergyNote: []string{"香菜"}},
			dish: "凉拌黄瓜", ings: []string{"黄瓜", "香菜"}, wantPart: "过敏",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.prefs.DishViolation(tc.dish, tc.ings)
			if tc.wantPart == "" {
				if got != "" {
					t.Errorf("expected no violation, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantPart) {
				t.Errorf("expected %q in reason, got %q", tc.wantPart, got)
			}
		})
	}
}

func TestPreferencesSummary(t *testing.T) {
	prefs := Preferences{
		Halal: true, HealthTags: []string{"gout"},
		Allergens: []string{"peanut"}, Spicy: "hot",
		Goals: []string{"fatLoss"},
	}
	got := strings.Join(prefs.Summary(), "|")
	for _, want := range []string{"清真", "痛风", "花生", "重辣", "减脂"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
}

func TestSplitAndJoinList(t *testing.T) {
	got := SplitList("halal，vegetarian、 halal")
	if len(got) != 2 || got[0] != "halal" || got[1] != "vegetarian" {
		t.Fatalf("SplitList = %v", got)
	}
	if JoinList([]string{"a", "b"}) != "a,b" {
		t.Errorf("JoinList = %q", JoinList([]string{"a", "b"}))
	}
}

func TestNameOfAcceptsLabels(t *testing.T) {
	if got := NameOf(HealthTagOptions, "糖尿病"); got != "diabetes" {
		t.Errorf("label lookup = %q", got)
	}
	if got := NameOf(AllergenOptions, "peanut"); got != "peanut" {
		t.Errorf("name lookup = %q", got)
	}
}

func TestNormalizeProfile(t *testing.T) {
	p := domain.SichuProfile{
		HealthTags:     "糖尿病,unknownTag",
		Allergens:      "花生",
		Restrictions:   "halal",
		NutritionGoals: `["减脂","fatLoss"]`,
		TargetMeals:    "午餐",
	}
	got := NormalizeProfile(p)
	if got.HealthTags != "diabetes" {
		t.Errorf("health tags = %q", got.HealthTags)
	}
	if got.Allergens != "peanut" {
		t.Errorf("allergens = %q", got.Allergens)
	}
	if got.TargetMeals != "lunch" {
		t.Errorf("target meals = %q", got.TargetMeals)
	}
	if got.NutritionGoals != `["fatLoss"]` {
		t.Errorf("goals = %q", got.NutritionGoals)
	}
	// 0% explore is a valid choice; only an out-of-range value resets.
	if got := NormalizeProfile(domain.SichuProfile{ExploreRatio: 2}); got.ExploreRatio != 0.3 {
		t.Errorf("invalid explore ratio = %v", got.ExploreRatio)
	}
	if got := NormalizeProfile(domain.SichuProfile{ExploreRatio: 0}); got.ExploreRatio != 0 {
		t.Errorf("zero explore ratio = %v", got.ExploreRatio)
	}
}

func TestMealTitles(t *testing.T) {
	if got := strings.Join(MealTitles([]string{"dinner", "breakfast"}), ","); got != "早餐,晚餐" {
		t.Errorf("MealTitles = %q", got)
	}
}
