package review

import (
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

var utc = time.UTC

func rec(id int64, recipeID int64, at time.Time) domain.Record {
	return domain.Record{ID: id, RecipeID: recipeID, CompletedAt: at, XPGained: 10}
}

func at(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, utc)
}

func recipes() map[int64]Recipe {
	return map[int64]Recipe{
		1: {Name: "青椒肉丝", Cuisine: "川菜", Region: "四川", MainCategory: "猪肉"},
		2: {Name: "番茄炒蛋", Cuisine: "家常菜", Region: "家常", MainCategory: "蛋类"},
		3: {Name: "红烧肉", Cuisine: "本帮菜", Region: "上海", MainCategory: "猪肉"},
		4: {Name: "清蒸鲈鱼", Cuisine: "粤菜", Region: "广东", MainCategory: "鱼类"},
	}
}

func TestBuildYearReview(t *testing.T) {
	records := []domain.Record{
		rec(1, 1, at(2026, time.November, 3, 18)),
		rec(2, 2, at(2026, time.November, 3, 18)), // same meal, same day
		rec(3, 1, at(2026, time.November, 4, 12)),
		rec(4, 1, at(2026, time.November, 5, 12)),
		rec(5, 3, at(2026, time.October, 1, 12)),
		rec(6, 4, at(2025, time.December, 31, 12)), // another year
	}
	stats := Build(records, recipes(), 2026, utc)
	if stats.Dishes != 5 {
		t.Errorf("Dishes = %d, want 5", stats.Dishes)
	}
	if stats.Days != 4 {
		t.Errorf("Days = %d, want 4", stats.Days)
	}
	if stats.Months[10] != 4 || stats.Months[9] != 1 {
		t.Errorf("Months = %v", stats.Months)
	}
	if len(stats.Top) != 3 {
		t.Fatalf("Top = %v", stats.Top)
	}
	if stats.Top[0].Name != "青椒肉丝" || stats.Top[0].Count != 3 {
		t.Errorf("Top[0] = %+v", stats.Top[0])
	}
	if stats.Top[1].Name != "番茄炒蛋" || stats.Top[2].Name != "红烧肉" {
		t.Errorf("Top order = %+v", stats.Top)
	}
	if len(stats.Cuisines) != 3 || stats.Cuisines[0].Label != "川菜" || stats.Cuisines[0].Count != 3 {
		t.Errorf("Cuisines = %+v", stats.Cuisines)
	}
	if stats.LongestStreak != 3 {
		t.Errorf("LongestStreak = %d, want 3", stats.LongestStreak)
	}
}

func TestBuildEmpty(t *testing.T) {
	stats := Build(nil, nil, 2026, utc)
	if stats.Dishes != 0 || stats.Days != 0 || len(stats.Top) != 0 || len(stats.Cuisines) != 0 {
		t.Errorf("empty review = %+v", stats)
	}
}

func TestTopIsCappedAndTiesSortByName(t *testing.T) {
	records := make([]domain.Record, 0, 8)
	meta := make(map[int64]Recipe, 8)
	for i := int64(1); i <= 8; i++ {
		records = append(records, rec(i, i, at(2026, time.March, 1, 12)))
		meta[i] = Recipe{Name: "菜" + string(rune('0'+i))}
	}
	stats := Build(records, meta, 2026, utc)
	if len(stats.Top) != TopDishes {
		t.Fatalf("Top = %d rows, want %d", len(stats.Top), TopDishes)
	}
	// Every dish has count 1, so the tie breaks by name.
	if stats.Top[0].Name != "菜1" {
		t.Errorf("Top[0] = %q, want 菜1", stats.Top[0].Name)
	}
	if stats.Top[TopDishes-1].Count != 1 {
		t.Errorf("Top = %+v", stats.Top)
	}
}

func TestMonthCells(t *testing.T) {
	records := []domain.Record{
		rec(1, 1, at(2026, time.November, 3, 9)),
		rec(2, 2, at(2026, time.November, 3, 18)),
		rec(3, 1, at(2026, time.November, 3, 20)),
		rec(4, 4, at(2026, time.December, 1, 12)),
	}
	cells := Month(records, recipes(), 2026, 11, utc)
	if len(cells) != 30 {
		t.Fatalf("cells = %d, want 30", len(cells))
	}
	if cells[0].Day != 1 || cells[0].Date != "2026-11-01" || cells[0].Count != 0 {
		t.Errorf("cell 1 = %+v", cells[0])
	}
	third := cells[2]
	if third.Count != 3 {
		t.Errorf("day 3 count = %d, want 3", third.Count)
	}
	if third.Initials != "青番" {
		t.Errorf("day 3 initials = %q, want 青番", third.Initials)
	}
}

func TestCountByDayAndStreak(t *testing.T) {
	now := at(2026, time.November, 5, 20)
	records := []domain.Record{
		rec(1, 1, at(2026, time.November, 5, 12)),
		rec(2, 1, at(2026, time.November, 4, 12)),
		rec(3, 1, at(2026, time.November, 3, 12)),
		rec(4, 1, at(2026, time.November, 1, 12)),
	}
	counts := CountByDay(records, utc)
	if counts["2026-11-05"] != 1 || counts["2026-11-02"] != 0 {
		t.Errorf("counts = %v", counts)
	}
	if got := Streak(records, now, utc); got != 3 {
		t.Errorf("Streak = %d, want 3", got)
	}

	// Today has no record yet: yesterday's run still counts.
	noToday := []domain.Record{
		rec(1, 1, at(2026, time.November, 4, 12)),
		rec(2, 1, at(2026, time.November, 3, 12)),
	}
	if got := Streak(noToday, now, utc); got != 2 {
		t.Errorf("Streak without today = %d, want 2", got)
	}

	// A missing day breaks the run.
	gap := []domain.Record{
		rec(1, 1, at(2026, time.November, 5, 12)),
		rec(2, 1, at(2026, time.November, 3, 12)),
	}
	if got := Streak(gap, now, utc); got != 1 {
		t.Errorf("Streak across a gap = %d, want 1", got)
	}
	if got := Streak(nil, now, utc); got != 0 {
		t.Errorf("Streak of nothing = %d, want 0", got)
	}
}

func TestYearsNewestFirst(t *testing.T) {
	records := []domain.Record{
		rec(1, 1, at(2024, time.January, 2, 12)),
		rec(2, 1, at(2026, time.June, 2, 12)),
		rec(3, 1, at(2025, time.May, 2, 12)),
		rec(4, 1, at(2026, time.July, 2, 12)),
	}
	years := Years(records, utc)
	if len(years) != 3 || years[0] != 2026 || years[1] != 2025 || years[2] != 2024 {
		t.Errorf("Years = %v", years)
	}
}
