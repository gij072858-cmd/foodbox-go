package app

// The redesign's evidence frames: the seven pages named by
// docs/方案-对齐egoist原生桌面改造.md §6 stage 0, at the three sizes the
// acceptance table uses. PLAN_SHOT_DIR picks the folder, so the same test
// writes before/ and after/ and the pair is comparable pixel for pixel.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
	"github.com/gij072858-cmd/foodbox-go/internal/services/review"
)

func planShotApp(t *testing.T) *App {
	t.Helper()
	a := newShowcaseApp(t)
	if packs, err := loadStarterPacks(); err == nil && len(packs) > 0 {
		a.packs = packs
		if len(packs) > 1 {
			a.packID = packs[1].ID
		}
	}

	a.sichu.meals = map[string]bool{"lunch": true}
	a.sichu.servings = 2
	a.sichu.questionnaire = domain.UserProfile{Spicy: "mild", Saltiness: "light"}
	a.sichu.members = []domain.MemberProfile{
		{ID: 1, Name: "本人", Role: "adult", IsSelf: true, HeightCm: 175, WeightKg: 70},
		{ID: 2, Name: "小满", Role: "kid", Age: 8, Allergens: "peanut"},
	}
	a.setSichuProfile(domain.SichuProfile{
		HeightCm: 175, WeightKg: 70, Age: 30, Sex: "male",
		ActivityLevel: "moderate", ExploreRatio: 0.3,
	})
	if catalog, err := nutrition.NewCatalog(); err == nil {
		a.sichu.catalog = catalog
	}

	now := time.Now().In(time.Local)
	day := func(offset, hour, minute int) time.Time {
		d := now.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, time.Local)
	}
	seconds := 2400
	a.records = []domain.Record{
		{ID: 1, RecipeID: 1, CompletedAt: day(0, 12, 0), XPGained: 10, SessionSeconds: &seconds},
		{ID: 2, RecipeID: 2, CompletedAt: day(0, 18, 30), XPGained: 10, SessionSeconds: &seconds},
		{ID: 3, RecipeID: 1, CompletedAt: day(-1, 18, 30), XPGained: 10},
		{ID: 4, RecipeID: 3, CompletedAt: day(-2, 19, 0), XPGained: 10},
		{ID: 5, RecipeID: 7, CompletedAt: day(-4, 12, 0), XPGained: 10},
	}
	a.recordCount = len(a.records)
	a.mealDays = 4
	a.feed.recipes = map[int64]review.Recipe{}
	for _, rec := range a.recipes {
		a.feed.recipes[rec.ID] = review.Recipe{
			Name: rec.Name, Cuisine: rec.Cuisine, Region: rec.Region,
			MainCategory: rec.MainCategory,
		}
	}
	return a
}

func TestPlanScreens(t *testing.T) {
	name := os.Getenv("PLAN_SHOT_DIR")
	if name == "" {
		t.Skip("set PLAN_SHOT_DIR=before or after to write the evidence frames")
	}
	dir := filepath.Join("..", "..", "test", "screenshots", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sizes := []struct {
		tag  string
		w, h int
		dark bool
	}{
		{"1280x860", 1280, 860, false},
		{"1024x720", 1024, 720, false},
		{"1280x860-dark", 1280, 860, true},
	}
	for _, s := range sizes {
		s := s
		t.Run(s.tag, func(t *testing.T) {
			a := planShotApp(t)
			tt := ui.NewTester(a.View, s.w, s.h)
			tt.SetLocale("zh-Hans")
			if s.dark {
				tt.SetDark(true)
			}
			settle(tt)
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-pantry.png", s.tag)))

			clickText(t, tt, "新手包")
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-starter.png", s.tag)))
			if err := tt.Click("回到仓库"); err == nil {
				settle(tt)
			}

			clickText(t, tt, "私厨")
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-sichu.png", s.tag)))

			clickText(t, tt, "菜式")
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-recipes.png", s.tag)))

			clickText(t, tt, a.recipes[0].Name)
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-recipe-detail.png", s.tag)))

			clickText(t, tt, "食历")
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-feed.png", s.tag)))
		})
	}

	// The run page needs a live session, which m4TestApp already knows how to
	// start.
	for _, s := range sizes {
		s := s
		t.Run("run-"+s.tag, func(t *testing.T) {
			a := m4TestApp()
			tt := ui.NewTester(a.View, s.w, s.h)
			tt.SetLocale("zh-Hans")
			if s.dark {
				tt.SetDark(true)
			}
			if err := openSchedule(tt); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("开始掌序"); err != nil {
				t.Fatal(err)
			}
			settle(tt)
			a.runStepTo(2)
			tt.Frame()
			settle(tt)
			capture(t, tt, filepath.Join(dir, fmt.Sprintf("%s-run.png", s.tag)))
		})
	}
}
