package app

// M5 nutrition dashboard tests: the target card, the empty-state exits, the
// metric switch and the derived intake report refreshing when records change.

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
)

// nutritionTestProfile is the body profile the tests feed the loader.
func nutritionTestProfile() domain.SichuProfile {
	return domain.SichuProfile{
		HeightCm:       175,
		WeightKg:       70,
		Age:            32,
		Sex:            "male",
		ActivityLevel:  "moderate",
		NutritionGoals: `["均衡"]`,
	}
}

// nutritionTestRecipes gives three fixture dishes real ingredient lines, on
// top of the ones newTestApp already ships.
func nutritionTestRecipes(a *App) {
	line := func(name string, grams float64) domain.Ingredient {
		g := grams
		return domain.Ingredient{Name: name, Grams: &g, Required: true}
	}
	set := func(id int64, servings int, lines ...domain.Ingredient) {
		rec := a.recipeByID[id]
		rec.BaseServings = servings
		rec.Ingredients = lines
		a.recipeByID[id] = rec
	}
	set(2, 2, line("鸡蛋", 100), line("番茄", 200), line("食用油", 15), line("盐", 3))
	set(4, 2, line("豆腐", 400), line("猪肉", 100), line("豆瓣酱", 20),
		line("生抽", 10), line("淀粉", 8), line("食用油", 20), line("蒜", 10))
	set(8, 2, line("香菇", 100), line("胡萝卜", 80), line("食用油", 10), line("盐", 2))
}

// nutritionTestRecordsAt spreads records over the week ending at base.
func nutritionTestRecordsAt(base time.Time) []domain.Record {
	at := func(daysAgo, hour int) time.Time {
		day := base.AddDate(0, 0, -daysAgo)
		return time.Date(day.Year(), day.Month(), day.Day(), hour, 10, 0, 0, day.Location())
	}
	return []domain.Record{
		{ID: 1, RecipeID: 1, CompletedAt: at(0, 12), XPGained: 10},
		{ID: 2, RecipeID: 2, CompletedAt: at(0, 18), XPGained: 10},
		{ID: 3, RecipeID: 2, CompletedAt: at(1, 12), XPGained: 10},
		{ID: 4, RecipeID: 8, CompletedAt: at(1, 18), XPGained: 10},
		{ID: 5, RecipeID: 4, CompletedAt: at(2, 12), XPGained: 10},
		{ID: 6, RecipeID: 1, CompletedAt: at(3, 12), XPGained: 10},
		{ID: 7, RecipeID: 4, CompletedAt: at(3, 18), XPGained: 10},
		{ID: 8, RecipeID: 2, CompletedAt: at(6, 12), XPGained: 10},
	}
}

// nutritionTestRecords spreads records over the last week.
func nutritionTestRecords() []domain.Record { return nutritionTestRecordsAt(time.Now()) }

// newNutritionApp builds a ready app with records and a body profile, loads
// the nutrition state and lands on the dashboard.
func newNutritionApp(t *testing.T, records []domain.Record, profile domain.SichuProfile, hasProfile bool) *App {
	return newNutritionAppAt(t, records, profile, hasProfile, time.Time{})
}

// newNutritionAppAt is newNutritionApp with a pinned clock, so screenshots and
// day-bucketed assertions stay stable.
func newNutritionAppAt(t *testing.T, records []domain.Record, profile domain.SichuProfile, hasProfile bool, now time.Time) *App {
	t.Helper()
	a := newTestApp()
	nutritionTestRecipes(a)
	a.records = records
	a.recordCount = len(records)
	a.mealDays = 4
	old := nutritionProfileReader
	nutritionProfileReader = func(context.Context, repository.Repository) (domain.SichuProfile, bool, error) {
		return profile, hasProfile, nil
	}
	t.Cleanup(func() { nutritionProfileReader = old })
	if err := a.loadNutritionState(context.Background(), nil); err != nil {
		t.Fatalf("loadNutritionState: %v", err)
	}
	if !now.IsZero() {
		a.nutrition.now = now
	}
	a.router.Push("/nutrition")
	return a
}

// The dashboard shows the computed target, today's intake, the week chart and
// the contributor list.
func TestNutritionDashboard(t *testing.T) {
	a := newNutritionApp(t, nutritionTestRecords(), nutritionTestProfile(), true)
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)

	for _, want := range []string{
		"今日目标",
		"基础代谢 BMR · kcal",
		"日常消耗 TDEE · kcal",
		"目标热量 · kcal",
		"蛋白 · 目标",
		"今日摄入",
		"最近 7 天",
		"嘌呤负荷",
		"食材贡献",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q on the dashboard; texts = %q", want, tt.Texts())
		}
	}
	// 175 cm / 70 kg / 32 y male, moderate: BMR 1638.75 → 1639,
	// TDEE 1638.75 × 1.55 = 2540.06 → 2540, 均衡 target = TDEE,
	// protein 70 × 1.2 = 84.
	for _, want := range []string{"1639", "2540", "84 g"} {
		if !tt.HasText(want) {
			t.Errorf("missing computed value %q; texts = %q", want, tt.Texts())
		}
	}
	if a.nutrition.report.Week.Dishes != len(nutritionTestRecords()) {
		t.Errorf("week dishes = %d, want %d", a.nutrition.report.Week.Dishes, len(nutritionTestRecords()))
	}
	if a.nutrition.report.Today.Dishes != 2 {
		t.Errorf("today dishes = %d, want 2", a.nutrition.report.Today.Dishes)
	}
	if a.nutrition.report.Today.Kcal <= 0 {
		t.Error("today intake has no calories")
	}
}

// Without a body profile the target card asks for one instead of inventing
// numbers, and the button lands on the body profile page.
func TestNutritionMissingProfile(t *testing.T) {
	a := newNutritionApp(t, nutritionTestRecords(), domain.SichuProfile{}, false)
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	if !tt.HasText("目标热量还不可用") {
		t.Fatalf("missing-profile state absent: %q", tt.Texts())
	}
	if err := tt.Click("去填身体档案"); err != nil {
		t.Fatalf("click 去填身体档案: %v", err)
	}
	settle(tt)
	if got := a.router.Path(); got != "/sichu/body" {
		t.Errorf("path = %q, want /sichu/body", got)
	}
}

// An empty history shows the exit states.
func TestNutritionEmptyState(t *testing.T) {
	a := newNutritionApp(t, nil, nutritionTestProfile(), true)
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	if !tt.HasText("还没有开饭记录") {
		t.Fatalf("empty state absent: %q", tt.Texts())
	}
	if err := tt.Click("去挑一道菜"); err != nil {
		t.Fatalf("click 去挑一道菜: %v", err)
	}
	settle(tt)
	if got := a.router.Path(); got != "/recipes" {
		t.Errorf("path = %q, want /recipes", got)
	}
}

// The contributor list switches metric without recomputing the report.
func TestNutritionMetricSwitch(t *testing.T) {
	a := newNutritionApp(t, nutritionTestRecords(), nutritionTestProfile(), true)
	// Tall enough that the contributor card is on screen to click.
	tt := ui.NewTester(a.View, 1280, 1500)
	settle(tt)
	if a.nutrition.metric != 0 {
		t.Fatalf("metric = %d, want 0", a.nutrition.metric)
	}
	if err := tt.Click("按嘌呤"); err != nil {
		t.Fatalf("click 按嘌呤: %v", err)
	}
	settle(tt)
	if a.nutrition.metric != 2 {
		t.Errorf("metric = %d, want 2", a.nutrition.metric)
	}
	if !tt.HasText(" mg") {
		t.Errorf("purine values missing: %q", tt.Texts())
	}
	if len(a.nutrition.report.TopPurine) == 0 {
		t.Error("TopPurine is empty")
	}
	if err := tt.Click("按蛋白"); err != nil {
		t.Fatalf("click 按蛋白: %v", err)
	}
	settle(tt)
	if a.nutrition.metric != 1 {
		t.Errorf("metric = %d, want 1", a.nutrition.metric)
	}
	if a.nutrition.report.TopKcal[0].Name != "猪肉" {
		t.Errorf("top calorie contributor = %q, want 猪肉", a.nutrition.report.TopKcal[0].Name)
	}
}

// A settlement prepends records; the dashboard rebuilds its report from the
// new snapshot (no recompute when nothing changed).
func TestNutritionReportRefreshes(t *testing.T) {
	a := newNutritionApp(t, nutritionTestRecords(), nutritionTestProfile(), true)
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	before := a.nutrition.report.Today.Kcal
	beforeSig := a.nutrition.sig

	a.records = append([]domain.Record{{
		ID: 99, RecipeID: 4, CompletedAt: time.Now(), XPGained: 10,
	}}, a.records...)
	tt.Frame()
	if a.nutrition.sig == beforeSig {
		t.Error("signature did not change after a new record")
	}
	if a.nutrition.report.Today.Kcal <= before {
		t.Errorf("today kcal = %v, want more than %v", a.nutrition.report.Today.Kcal, before)
	}
	// A second frame with the same records must not recompute.
	sig := a.nutrition.sig
	tt.Frame()
	if a.nutrition.sig != sig {
		t.Error("report recomputed without a records change")
	}
}

// The page copes with an old record whose recipe was deleted.
func TestNutritionDeletedRecipe(t *testing.T) {
	records := []domain.Record{{ID: 1, RecipeID: 4242, CompletedAt: time.Now(), XPGained: 10}}
	a := newNutritionApp(t, records, nutritionTestProfile(), true)
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	if a.nutrition.report.Today.Uncovered != 1 {
		t.Errorf("Uncovered = %d, want 1", a.nutrition.report.Today.Uncovered)
	}
	if !tt.HasText("缺成分数据") {
		t.Errorf("coverage line missing: %q", tt.Texts())
	}
}
