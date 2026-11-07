package nutrition

import (
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// intakeFixture builds two recipes and a week of records around now.
func intakeFixture(t *testing.T) (*Catalog, map[int64]domain.Recipe, []domain.Record, time.Time) {
	t.Helper()
	c := testCatalog(t)
	now := time.Now()
	at := func(daysAgo, hour int) time.Time {
		day := now.AddDate(0, 0, -daysAgo)
		return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
	}
	gpork, gpepper := 300.0, 60.0
	gegg, gtomato := 100.0, 200.0
	recipes := map[int64]domain.Recipe{
		1: {ID: 1, Name: "青椒肉丝", BaseServings: 2, Ingredients: []domain.Ingredient{
			{Name: "猪肉", Grams: &gpork},
			{Name: "青椒", Grams: &gpepper},
		}},
		2: {ID: 2, Name: "番茄炒蛋", BaseServings: 1, Ingredients: []domain.Ingredient{
			{Name: "鸡蛋", Grams: &gegg},
			{Name: "番茄", Grams: &gtomato},
		}},
	}
	records := []domain.Record{
		{ID: 1, RecipeID: 1, CompletedAt: at(0, 12)},
		{ID: 2, RecipeID: 2, CompletedAt: at(0, 18)},
		{ID: 3, RecipeID: 2, CompletedAt: at(1, 12)},
		{ID: 4, RecipeID: 99, CompletedAt: at(2, 12)}, // deleted recipe
		{ID: 5, RecipeID: 2, CompletedAt: at(9, 12)},  // outside the window
	}
	return c, recipes, records, now
}

// Records land in the right local day, oldest first, with today last.
func TestAnalyzeIntakeBuckets(t *testing.T) {
	c, recipes, records, now := intakeFixture(t)
	report := AnalyzeIntake(records, recipes, c, now)
	if len(report.Days) != RecentDays {
		t.Fatalf("Days = %d, want %d", len(report.Days), RecentDays)
	}
	today := report.Days[RecentDays-1]
	if today.Date.Format("2006-01-02") != now.Format("2006-01-02") {
		t.Errorf("last day = %s, want today", today.Date.Format("2006-01-02"))
	}
	if today.Dishes != 2 {
		t.Errorf("today dishes = %d, want 2", today.Dishes)
	}
	if report.Today.Dishes != 2 || report.Today.Kcal <= 0 {
		t.Errorf("Today = %+v, want two estimated dishes", report.Today)
	}
	yesterday := report.Days[RecentDays-2]
	if yesterday.Dishes != 1 || yesterday.PurineMissing != 0 {
		t.Errorf("yesterday = %+v, want one covered dish", yesterday)
	}
	// The record outside the window must not be counted anywhere.
	totalDishes := 0
	for _, day := range report.Days {
		totalDishes += day.Dishes
	}
	if totalDishes != 4 {
		t.Errorf("window dishes = %d, want 4", totalDishes)
	}
	if report.Week.Dishes != 4 {
		t.Errorf("Week.Dishes = %d, want 4", report.Week.Dishes)
	}
	// The deleted recipe has no estimate and is reported, not silently zeroed.
	if report.Days[RecentDays-3].Uncovered != 1 {
		t.Errorf("day with deleted recipe = %+v, want one uncovered", report.Days[RecentDays-3])
	}
}

// The focus day is today when it has records, otherwise the newest day with
// any, and its top rows are per-serving.
func TestAnalyzeIntakeTopContributors(t *testing.T) {
	c, recipes, records, now := intakeFixture(t)
	report := AnalyzeIntake(records, recipes, c, now)
	if report.Focus.Format("2006-01-02") != now.Format("2006-01-02") {
		t.Fatalf("Focus = %s, want today", report.Focus.Format("2006-01-02"))
	}
	if len(report.TopKcal) == 0 {
		t.Fatal("TopKcal is empty")
	}
	// 猪肉 150 g per serving dominates both the calories and the protein.
	if report.TopKcal[0].Name != "猪肉" {
		t.Errorf("TopKcal[0] = %q, want 猪肉", report.TopKcal[0].Name)
	}
	if report.TopProtein[0].Name != "猪肉" {
		t.Errorf("TopProtein[0] = %q, want 猪肉", report.TopProtein[0].Name)
	}
	// 150 g pork at 331 kcal/100 g × 0.91 editable ≈ 452 kcal.
	approx(t, "pork kcal", report.TopKcal[0].Kcal, 331*0.91*1.5)
	approx(t, "pork grams", report.TopKcal[0].ServedGrams, 150)
	// Purine list only holds ingredients the appendix covers: 番茄 has 17 mg
	// per 100 g and 200 g in the dish, 猪肉 has no purine value at all.
	if len(report.TopPurine) == 0 || report.TopPurine[0].Name != "番茄" {
		t.Errorf("TopPurine = %+v, want 番茄 first", report.TopPurine)
	}
}

// With no record today the contributors describe the newest day that has one.
func TestAnalyzeIntakeFocusFallsBack(t *testing.T) {
	c, recipes, records, now := intakeFixture(t)
	var onlyYesterday []domain.Record
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	for _, r := range records {
		if r.CompletedAt.Format("2006-01-02") == yesterday {
			onlyYesterday = append(onlyYesterday, r)
		}
	}
	report := AnalyzeIntake(onlyYesterday, recipes, c, now)
	if report.Today.Dishes != 0 {
		t.Errorf("Today.Dishes = %d, want 0", report.Today.Dishes)
	}
	if report.Focus.Format("2006-01-02") != yesterday {
		t.Errorf("Focus = %s, want yesterday", report.Focus.Format("2006-01-02"))
	}
	if len(report.TopKcal) == 0 {
		t.Error("TopKcal is empty on the fallback day")
	}
}

// Empty records still produce a full, zeroed week so the chart has bars.
func TestAnalyzeIntakeEmpty(t *testing.T) {
	c := testCatalog(t)
	report := AnalyzeIntake(nil, nil, c, time.Now())
	if len(report.Days) != RecentDays {
		t.Fatalf("Days = %d, want %d", len(report.Days), RecentDays)
	}
	for _, day := range report.Days {
		if day.Dishes != 0 || day.Kcal != 0 {
			t.Errorf("empty report day = %+v", day)
		}
	}
	if report.TopKcal != nil || report.Focus.IsZero() == false {
		t.Errorf("empty report = %+v, want no focus and no tops", report)
	}
}

// Coverage counters split dishes into full / partial / uncovered.
func TestAnalyzeIntakeCoverageCounts(t *testing.T) {
	c := testCatalog(t)
	g := 100.0
	recipes := map[int64]domain.Recipe{
		1: {ID: 1, BaseServings: 1, Ingredients: []domain.Ingredient{{Name: "鸡蛋", Grams: &g}}},
		2: {ID: 2, BaseServings: 1, Ingredients: []domain.Ingredient{{Name: "鸡蛋", Grams: &g}, {Name: "未知食材", Grams: &g}}},
		3: {ID: 3, BaseServings: 1, Ingredients: []domain.Ingredient{{Name: "未知食材", Grams: &g}}},
	}
	now := time.Now()
	records := []domain.Record{
		{RecipeID: 1, CompletedAt: now},
		{RecipeID: 2, CompletedAt: now},
		{RecipeID: 3, CompletedAt: now},
	}
	report := AnalyzeIntake(records, recipes, c, now)
	if report.Today.Full != 1 || report.Today.Partial != 1 || report.Today.Uncovered != 1 {
		t.Errorf("coverage = %d/%d/%d, want 1/1/1", report.Today.Full, report.Today.Partial, report.Today.Uncovered)
	}
	// 鸡蛋 carries no purine value in the table, so all three dishes are
	// counted as missing purine data.
	if report.Today.PurineMissing != 3 {
		t.Errorf("PurineMissing = %d, want 3", report.Today.PurineMissing)
	}
}
