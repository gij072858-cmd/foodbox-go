package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// openWriteStore opens a migrated database in the test's temp dir.
func openWriteStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func minutes(v float64) *float64 { return &v }

func sampleRecipe(name, group string) domain.Recipe {
	return domain.Recipe{
		Name:            name,
		Cuisine:         "家常菜",
		Region:          "川菜",
		MainCategory:    "猪肉",
		DurationMinutes: minutes(20),
		Difficulty:      domain.DifficultyMedium,
		BaseServings:    3,
		SeasonMonths:    []int{3, 4, 5},
		Ingredients: []domain.Ingredient{
			{Name: "猪肉", Amount: 300, Unit: "g", Required: true, Grams: minutes(300)},
			{Name: "青椒", Amount: 2, Unit: "个", Required: true},
			{Name: "盐", Required: true, Seasoning: true, Grams: minutes(3)},
		},
		Intro:        "测试用菜式。",
		Source:       &domain.RecipeSource{Platform: "xiaohongshu", Author: "测试", URL: "https://example.com/x"},
		VersionGroup: group,
		IsDish:       true,
		Steps: []domain.Step{
			{Instruction: "切肉。", DurationMinutes: minutes(5), Resources: []string{"board", "hand"}, SafetyTag: "rawMeat"},
			{Instruction: "下锅炒。", DurationMinutes: minutes(8), Resources: []string{"stove", "hand"}, CompletionCue: "肉变色"},
		},
	}
}

func TestCreateRecipeWritesStepsInOneTransaction(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()

	id, err := store.CreateRecipe(ctx, sampleRecipe("青椒肉丝", "青椒肉丝"))
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}
	rec, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "青椒肉丝" || rec.BaseServings != 3 || !rec.IsDish {
		t.Fatalf("recipe = %+v", rec)
	}
	if rec.Source == nil || rec.Source.Author != "测试" {
		t.Fatalf("source = %+v", rec.Source)
	}
	if len(rec.SeasonMonths) != 3 || rec.SeasonMonths[2] != 5 {
		t.Fatalf("season = %v", rec.SeasonMonths)
	}
	if len(rec.Ingredients) != 3 || rec.Ingredients[2].Seasoning == false {
		t.Fatalf("ingredients = %+v", rec.Ingredients)
	}
	if len(rec.Steps) != 2 {
		t.Fatalf("steps = %+v", rec.Steps)
	}
	if rec.Steps[0].OrderIndex != 1 || rec.Steps[1].OrderIndex != 2 {
		t.Fatalf("order = %d, %d", rec.Steps[0].OrderIndex, rec.Steps[1].OrderIndex)
	}
	if rec.Steps[0].SafetyTag != "rawMeat" || rec.Steps[1].CompletionCue != "肉变色" {
		t.Fatalf("step fields = %+v", rec.Steps)
	}
	if len(rec.Steps[0].Resources) != 2 {
		t.Fatalf("resources = %v", rec.Steps[0].Resources)
	}
	count, err := store.RecipeCount(ctx)
	if err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v", count, err)
	}
}

func TestUpdateRecipeReplacesSteps(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	id, err := store.CreateRecipe(ctx, sampleRecipe("青椒肉丝", "青椒肉丝"))
	if err != nil {
		t.Fatal(err)
	}

	rec, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rec.Name = "青椒肉丝（改良版）"
	rec.Steps = []domain.Step{{Instruction: "只剩一步。", Resources: []string{"hand"}}}
	if err := store.UpdateRecipe(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "青椒肉丝（改良版）" {
		t.Fatalf("name = %q", got.Name)
	}
	if len(got.Steps) != 1 || got.Steps[0].Instruction != "只剩一步。" {
		t.Fatalf("steps = %+v", got.Steps)
	}
	if got.Steps[0].OrderIndex != 1 {
		t.Fatalf("order = %d", got.Steps[0].OrderIndex)
	}
}

func TestUpdateRecipeKeepsStepsWhenNil(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	id, err := store.CreateRecipe(ctx, sampleRecipe("青椒肉丝", "青椒肉丝"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Execute(ctx, "UPDATE recipes SET name = '改个名' WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
	rec, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rec.Steps = nil
	rec.Intro = "只改简介。"
	if err := store.UpdateRecipe(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("steps were erased: %+v", got.Steps)
	}
}

func TestReplaceSteps(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	id, err := store.CreateRecipe(ctx, sampleRecipe("青椒肉丝", "青椒肉丝"))
	if err != nil {
		t.Fatal(err)
	}
	steps := []domain.Step{
		{Instruction: "一。", DurationMinutes: minutes(1.5)},
		{Instruction: "二。", Resources: []string{"stove", "waiting"}},
		{Instruction: "三。", CompletionCue: "出锅"},
	}
	if err := store.ReplaceSteps(ctx, id, steps); err != nil {
		t.Fatal(err)
	}
	got, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 3 {
		t.Fatalf("steps = %d", len(got.Steps))
	}
	if got.Steps[0].DurationMinutes == nil || *got.Steps[0].DurationMinutes != 1.5 {
		t.Fatalf("duration = %v", got.Steps[0].DurationMinutes)
	}
}

func TestDeleteRecipeCascadesSteps(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	id, err := store.CreateRecipe(ctx, sampleRecipe("青椒肉丝", "青椒肉丝"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRecipe(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecipeByID(ctx, id); err == nil {
		t.Fatal("recipe still readable after delete")
	}
	rows, err := store.Query(ctx, "SELECT COUNT(*) FROM steps WHERE recipe_id = ?", id)
	if err != nil {
		t.Fatal(err)
	}
	if asInt(rows.Values[0][0]) != 0 {
		t.Fatalf("steps left behind: %v", rows.Values)
	}
}

func TestRecipesByVersionGroup(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	first, err := store.CreateRecipe(ctx, sampleRecipe("口水鸡（家常版）", "口水鸡"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateRecipe(ctx, sampleRecipe("口水鸡（整鸡版）", "口水鸡"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRecipe(ctx, sampleRecipe("白切鸡", "")); err != nil {
		t.Fatal(err)
	}
	group, err := store.RecipesByVersionGroup(ctx, "口水鸡")
	if err != nil {
		t.Fatal(err)
	}
	if len(group) != 2 || group[0].ID != first || group[1].ID != second {
		t.Fatalf("group = %+v", group)
	}
	if len(group[0].Steps) != 0 {
		t.Fatalf("version group must not load steps: %+v", group[0].Steps)
	}
}

// A save that fails inside the transaction leaves no partial dish behind.
// The bad step (invalid UTF-8) fails after the recipe row was written, so
// the rollback must take the recipe row with it.
func TestCreateRecipeRollsBackOnStepFailure(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	rec := sampleRecipe("坏菜", "坏菜")
	rec.Steps = append(rec.Steps, domain.Step{Instruction: string([]byte{0xff, 0xfe})})
	if _, err := store.CreateRecipe(ctx, rec); err == nil {
		t.Fatal("expected the transaction to fail")
	}
	count, err := store.RecipeCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("recipe row survived a failed transaction: %d", count)
	}
}

// The same rule for an edit: the old dish and its old steps stay readable.
func TestUpdateRecipeRollsBackOnStepFailure(t *testing.T) {
	store := openWriteStore(t)
	ctx := context.Background()
	id, err := store.CreateRecipe(ctx, sampleRecipe("青椒肉丝", "青椒肉丝"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	rec.Name = "被改坏的名字"
	rec.Steps = []domain.Step{{Instruction: string([]byte{0xff})}}
	if err := store.UpdateRecipe(ctx, rec); err == nil {
		t.Fatal("expected the transaction to fail")
	}
	got, err := store.RecipeByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "青椒肉丝" {
		t.Fatalf("name changed despite rollback: %q", got.Name)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("steps changed despite rollback: %+v", got.Steps)
	}
}

func TestUpdateMissingRecipe(t *testing.T) {
	store := openWriteStore(t)
	if err := store.UpdateRecipe(context.Background(), sampleRecipe("无处安放", "")); err == nil {
		t.Fatal("expected not-found error")
	}
}
