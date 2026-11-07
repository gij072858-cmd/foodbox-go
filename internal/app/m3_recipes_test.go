package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// newStoreApp builds the ready state the production bootstrap reaches, but
// on a throwaway database seeded with the fixture dishes. Views then save
// into a real SQLite file, which is what the M3 tests assert against.
func newStoreApp(t *testing.T) (*App, testRepo) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "m3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	steps := fixtureSteps()
	for _, rec := range fixtureRecipes() {
		rec.Steps = steps[rec.ID]
		if _, err := store.CreateRecipe(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	a := newTestApp()
	a.store = store
	recipes, err := store.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	allSteps, err := store.AllSteps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.setRecipes(recipes, allSteps)
	return a, store
}

// minutesPtr is a fixture helper for the optional duration fields.
func minutesPtr(v float64) *float64 { return &v }

// showcaseRecipes is a small, realistic library for the M3 view tests and
// screenshots: a two-version group, a spring dish and a few all-year
// favourites with different durations.
func showcaseRecipes() ([]domain.Recipe, map[int64][]domain.Step) {
	recipes := []domain.Recipe{
		{
			ID: 1, Name: "口水鸡（家常版）", Cuisine: "凉菜", Region: "川菜",
			MainCategory: "鸡肉", DurationMinutes: minutesPtr(40),
			Difficulty: domain.DifficultyMedium, BaseServings: 2,
			SeasonMonths: []int{6, 7, 8}, VersionGroup: "口水鸡", IsDish: true,
			Ingredients: []domain.Ingredient{
				{Name: "鸡腿肉", Amount: 400, Unit: "g", Required: true, Grams: minutesPtr(400)},
				{Name: "辣椒油", Amount: 2, Unit: "勺", Required: true, Seasoning: true, Grams: minutesPtr(30)},
				{Name: "花椒粉", Required: false, Seasoning: true, Grams: minutesPtr(2)},
			},
			Intro: "家常做法，鸡腿煮熟后浇红油。",
		},
		{
			ID: 2, Name: "口水鸡（整鸡版）", Cuisine: "凉菜", Region: "川菜",
			MainCategory: "鸡肉", DurationMinutes: minutesPtr(60),
			Difficulty: domain.DifficultyMedium, BaseServings: 4,
			SeasonMonths: []int{6, 7, 8}, VersionGroup: "口水鸡", IsDish: true,
			Ingredients: []domain.Ingredient{
				{Name: "整鸡", Amount: 1, Unit: "只", Required: true, Grams: minutesPtr(1200)},
				{Name: "辣椒油", Amount: 3, Unit: "勺", Required: true, Seasoning: true},
			},
			Intro: "整鸡浸熟，皮更紧实。",
		},
		{
			ID: 3, Name: "春笋炒腊肉", Cuisine: "快手菜", Region: "湘菜",
			MainCategory: "猪肉", DurationMinutes: minutesPtr(25),
			Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{3, 4, 5}, VersionGroup: "春笋炒腊肉", IsDish: true,
			Ingredients: []domain.Ingredient{
				{Name: "春笋", Amount: 300, Unit: "g", Required: true, Grams: minutesPtr(300)},
				{Name: "腊肉", Amount: 150, Unit: "g", Required: true, Grams: minutesPtr(150)},
			},
			Intro: "春天才有的脆嫩。",
		},
		{
			ID: 4, Name: "番茄炒蛋", Cuisine: "家常菜", Region: "家常菜",
			MainCategory: "蛋类", DurationMinutes: minutesPtr(15),
			Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			VersionGroup: "番茄炒蛋", IsDish: true,
			Ingredients: []domain.Ingredient{
				{Name: "番茄", Amount: 2, Unit: "个", Required: true, Grams: minutesPtr(300)},
				{Name: "鸡蛋", Amount: 3, Unit: "个", Required: true, Grams: minutesPtr(150)},
			},
		},
		{
			ID: 5, Name: "红烧肉", Cuisine: "炖菜", Region: "本帮菜",
			MainCategory: "猪肉", DurationMinutes: minutesPtr(90),
			Difficulty: domain.DifficultyHard, BaseServings: 3,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			VersionGroup: "红烧肉", IsDish: true,
			Ingredients: []domain.Ingredient{
				{Name: "五花肉", Amount: 500, Unit: "g", Required: true},
			},
		},
		{
			ID: 6, Name: "清蒸鲈鱼", Cuisine: "蒸菜", Region: "粤菜",
			MainCategory: "鱼类", DurationMinutes: minutesPtr(30),
			Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			VersionGroup: "清蒸鲈鱼", IsDish: true,
		},
		{
			ID: 7, Name: "麻婆豆腐", Cuisine: "家常菜", Region: "川菜",
			MainCategory: "豆制品", DurationMinutes: minutesPtr(25),
			Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			VersionGroup: "麻婆豆腐", IsDish: true,
		},
		{
			ID: 8, Name: "宫保鸡丁", Cuisine: "快炒", Region: "川菜",
			MainCategory: "禽肉", DurationMinutes: minutesPtr(25),
			Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			VersionGroup: "宫保鸡丁", IsDish: true,
		},
	}
	steps := make(map[int64][]domain.Step, len(recipes))
	for _, rec := range recipes {
		rows := []string{
			"备料：把主料洗净切好。",
			"下锅：按顺序加热，注意火候。",
			"出锅：调味装盘，趁热吃。",
		}
		for i, text := range rows {
			steps[rec.ID] = append(steps[rec.ID], domain.Step{
				RecipeID: rec.ID, OrderIndex: i + 1, Instruction: rec.Name + "：" + text,
				DurationMinutes: minutesPtr(float64(3 + i*2)),
				Resources:       []string{"hand"},
			})
		}
	}
	return recipes, steps
}

// newShowcaseApp is the in-memory app the version, season, sort and
// screenshot tests use.
func newShowcaseApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp()
	recipes, steps := showcaseRecipes()
	a.setRecipes(recipes, steps)
	withCatalog(t, a)
	a.recipeEdit.seasonMonth = 4
	a.recomputeRecipeView()
	return a
}

// openRecipes navigates from wherever the app starts to the recipe list.
func openRecipes(t *testing.T, tt *ui.Tester) {
	t.Helper()
	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
}

// TestRecipeEditorCreateFlow walks the editor the way a user does: open the
// new-dish form, try to save it empty, then fill the name and one step.
func TestRecipeEditorCreateFlow(t *testing.T) {
	a, store := newStoreApp(t)
	tt := ui.NewTester(a.View, 1280, 1500)
	openRecipes(t, tt)

	if err := tt.Click("新建菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("名称必填，至少写一步步骤") {
		t.Fatalf("editor did not open: %q", tt.Texts())
	}

	// An empty form must not write anything, and says why.
	if err := tt.Click("保存菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("请填写菜式名称") {
		t.Fatalf("missing name validation: %q", tt.Texts())
	}
	if count, err := store.RecipeCount(context.Background()); err != nil || count != 8 {
		t.Fatalf("a rejected save wrote rows: count=%d err=%v", count, err)
	}

	// A name without a single step is still rejected.
	if err := tt.Click("菜式名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("酱爆茄子")
	if err := tt.Click("保存菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("至少写一步步骤") {
		t.Fatalf("missing step validation: %q", tt.Texts())
	}
	if count, _ := store.RecipeCount(context.Background()); count != 8 {
		t.Fatalf("a rejected save wrote rows: count=%d", count)
	}

	// Fill the first step and save: the list then shows the new dish.
	if err := tt.Click("第 1 步说明"); err != nil {
		t.Fatal(err)
	}
	tt.Type("茄子切条，下锅煸软。")
	if err := tt.Click("保存菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("酱爆茄子") {
		t.Fatalf("list did not refresh: %q", tt.Texts())
	}
	count, err := store.RecipeCount(context.Background())
	if err != nil || count != 9 {
		t.Fatalf("count after save = %d, err = %v", count, err)
	}
	var saved domain.Recipe
	for _, rec := range a.recipes {
		if rec.Name == "酱爆茄子" {
			saved = rec
		}
	}
	if saved.ID == 0 {
		t.Fatalf("saved dish missing from the snapshot: %+v", a.recipes)
	}
	steps, err := store.RecipeByID(context.Background(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps.Steps) != 1 || !strings.Contains(steps.Steps[0].Instruction, "茄子切条") {
		t.Fatalf("steps = %+v", steps.Steps)
	}
	if _, ok := a.steps[saved.ID]; !ok {
		t.Fatal("step snapshot not refreshed after save")
	}
}

// TestRecipeEditorEditFlow opens a seeded dish, renames it and saves; the
// steps the form never showed must survive the replace.
func TestRecipeEditorEditFlow(t *testing.T) {
	a, store := newStoreApp(t)
	tt := ui.NewTester(a.View, 1280, 1500)
	openRecipes(t, tt)

	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("编辑"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("菜式名称"); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type("青椒肉丝（改良）")
	if err := tt.Click("保存菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("已保存「青椒肉丝（改良）」") {
		t.Fatalf("save notice missing: %q", tt.Texts())
	}
	rec, err := store.RecipeByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "青椒肉丝（改良）" {
		t.Fatalf("name = %q", rec.Name)
	}
	if len(rec.Steps) != 4 {
		t.Fatalf("steps lost on edit: %+v", rec.Steps)
	}
	if a.recipeByID[1].Name != "青椒肉丝（改良）" {
		t.Fatalf("snapshot not refreshed: %+v", a.recipeByID[1])
	}
}

// TestRecipeDeleteFlow deletes a dish through its confirmation dialog.
func TestRecipeDeleteFlow(t *testing.T) {
	a, store := newStoreApp(t)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("删除菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("删除「青椒肉丝」？") {
		t.Fatalf("confirmation missing: %q", tt.Texts())
	}
	// Cancelling keeps the dish.
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if _, err := store.RecipeByID(context.Background(), 1); err != nil {
		t.Fatalf("cancel deleted the dish: %v", err)
	}

	if err := tt.Click("删除菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if _, err := store.RecipeByID(context.Background(), 1); err == nil {
		t.Fatal("dish still in the database")
	}
	if _, ok := a.recipeByID[1]; ok {
		t.Fatal("snapshot still has the deleted dish")
	}
	if _, ok := a.steps[1]; ok {
		t.Fatal("step snapshot still has the deleted dish")
	}
	if tt.HasText("青椒肉丝") {
		t.Fatalf("list did not refresh: %q", tt.Texts())
	}
}

// TestRecipeVersionSwitcher checks the version group listing and switching.
func TestRecipeVersionSwitcher(t *testing.T) {
	a := newShowcaseApp(t)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	if err := tt.Click("口水鸡（家常版）"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("2 种做法") {
		t.Fatalf("version section missing: %q", tt.Texts())
	}
	if err := tt.Click("家常版"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("整鸡版") {
		t.Fatalf("sibling version missing: %q", tt.Texts())
	}
	if err := tt.Click("整鸡版"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("整鸡浸熟，皮更紧实。") {
		t.Fatalf("did not switch to the other version: %q", tt.Texts())
	}
	if a.versionCount(a.recipeByID[2]) != 2 {
		t.Fatalf("version index = %d", a.versionCount(a.recipeByID[2]))
	}
}

// TestRecipeSeasonFilter checks the 「当季」 chip against the injected month.
func TestRecipeSeasonFilter(t *testing.T) {
	a := newShowcaseApp(t)
	a.recipeEdit.seasonMonth = 4
	a.recomputeRecipeView()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	if err := tt.Click("当季"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.recipeView) != 1 || a.recipeView[0].Name != "春笋炒腊肉" {
		t.Fatalf("April seasonal view = %+v", namesOf(a.recipeView))
	}

	// Summer brings the version group in; the all-year dishes stay out.
	a.recipeEdit.seasonMonth = 7
	a.recomputeRecipeView()
	if len(a.recipeView) != 2 {
		t.Fatalf("July seasonal view = %+v", namesOf(a.recipeView))
	}
	for _, rec := range a.recipeView {
		if rec.VersionGroup != "口水鸡" {
			t.Fatalf("all-year dish leaked into the seasonal filter: %s", rec.Name)
		}
	}

	// Turning the chip off restores the whole list.
	if err := tt.Click("当季"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.recipeView) != 8 {
		t.Fatalf("cleared filter view = %d", len(a.recipeView))
	}
}

func namesOf(recipes []domain.Recipe) []string {
	out := make([]string, 0, len(recipes))
	for _, rec := range recipes {
		out = append(out, rec.Name)
	}
	return out
}

// TestRecipeSort checks that the segmented sort reorders the list without
// touching the stored rows.
func TestRecipeSort(t *testing.T) {
	a := newShowcaseApp(t)
	a.recomputeRecipeView()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	if err := tt.Click("默认排序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("最快"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.recipeEdit.sortFilter != 1 {
		t.Fatalf("sort mode = %d", a.recipeEdit.sortFilter)
	}
	last := -1.0
	for _, rec := range a.recipeView {
		d := recipeDuration(rec)
		if d < last {
			t.Fatalf("fastest sort out of order: %+v", namesOf(a.recipeView))
		}
		last = d
	}
	if a.recipeView[0].Name != "番茄炒蛋" {
		t.Fatalf("fastest dish = %q", a.recipeView[0].Name)
	}

	if err := tt.Click("最快"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("最新"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.recipeView[0].ID != 8 {
		t.Fatalf("newest dish = %+v", a.recipeView[0])
	}

	if err := tt.Click("最新"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("默认排序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.recipeView[0].ID != 1 {
		t.Fatalf("default order starts at %d", a.recipeView[0].ID)
	}
}

// TestRecipeEditorPreloadsExistingFields verifies the form reads a dish back
// so an edit does not start from blanks.
func TestRecipeEditorPreloadsExistingFields(t *testing.T) {
	a := newShowcaseApp(t)
	a.router.Push("/recipes/1/edit")
	tt := ui.NewTester(a.View, 1280, 1500)
	settle(tt)
	if !tt.HasText("编辑菜式") {
		t.Fatalf("editor did not open: %q", tt.Texts())
	}
	s := a.recipeEdit
	if s.name != "口水鸡（家常版）" || s.region != "川菜" || s.versionGroup != "口水鸡" {
		t.Fatalf("form = %+v", s)
	}
	if !s.months[5] || !s.months[6] || !s.months[7] {
		t.Fatalf("season months = %v", s.months)
	}
	if len(s.ingredients) != 3 || len(s.steps) != 3 {
		t.Fatalf("rows = %d ingredients, %d steps", len(s.ingredients), len(s.steps))
	}
	if !tt.HasText("夏 · 6–8 月") {
		t.Fatalf("season label missing: %q", tt.Texts())
	}
}

// One filter row: the main-ingredient Select re-derives the list and keeps
// the stock counts in the segment labels.
func TestRecipeFilterOneRow(t *testing.T) {
	a := newShowcaseApp(t)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	if err := tt.Click("主料不限"); err != nil {
		t.Fatalf("%v; texts=%q", err, tt.Texts())
	}
	settle(tt)
	if err := tt.Click("猪肉"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.recipeView) == 0 {
		t.Fatal("the filter emptied the list")
	}
	for _, rec := range a.recipeView {
		if rec.MainCategory != "猪肉" {
			t.Fatalf("filter leaked %q (%s)", rec.Name, rec.MainCategory)
		}
	}
	if a.recipeEdit.mainFilter != "猪肉" {
		t.Fatalf("filter value = %q", a.recipeEdit.mainFilter)
	}
}

// The table's header sorts the rows; a second click reverses them, and the
// first row is the fastest or the slowest dish.
func TestRecipeTableSort(t *testing.T) {
	a := newShowcaseApp(t)
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	// The list is a table; no switch is needed before sorting.
	if err := tt.Click("耗时"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	fastest := recipeDuration(a.recipeView[0])
	for _, rec := range a.recipeView {
		if recipeDuration(rec) < fastest {
			t.Fatalf("ascending sort is out of order: %+v", namesOf(a.recipeView))
		}
	}
	if err := tt.Click("耗时"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	slowest := recipeDuration(a.recipeView[0])
	for _, rec := range a.recipeView {
		if recipeDuration(rec) > slowest {
			t.Fatalf("descending sort is out of order: %+v", namesOf(a.recipeView))
		}
	}
	if slowest <= fastest {
		t.Fatalf("the two directions give the same first row (%v)", slowest)
	}
}

// The whole table row opens the dish, not only the name cell.
func TestRecipeTableWholeRowClick(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	rec := a.recipeView[0]
	pos, ok := tt.Find(recipeMeta(rec))
	if !ok {
		t.Fatalf("the row has no meta cell: %q", tt.Texts())
	}
	tt.ClickAt(pos.X+pos.W/2, pos.Y+pos.H/2)
	settle(tt)
	if want := fmt.Sprintf("/recipes/%d", rec.ID); a.router.Path() != want {
		t.Fatalf("path after clicking the meta cell = %q, want %q", a.router.Path(), want)
	}
}

// Adding a dish to 掌序 keeps the cook on the dish page instead of throwing
// them into the cooking screen.
func TestAddToScheduleStaysOnRecipesPage(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	rec := a.recipeView[0]
	if err := tt.Click(rec.Name); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("加入掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if want := fmt.Sprintf("/recipes/%d", rec.ID); a.router.Path() != want {
		t.Fatalf("path after adding = %q, want %q", a.router.Path(), want)
	}
	if len(a.schedule) != 1 {
		t.Fatalf("schedule after adding = %+v", a.schedule)
	}
	if !strings.Contains(a.notice, "已加入掌序") {
		t.Errorf("notice = %q", a.notice)
	}
	if err := tt.Click("加入掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.schedule) != 1 || !strings.Contains(a.notice, "已经在掌序里") {
		t.Errorf("a second add made %d dishes (notice %q)", len(a.schedule), a.notice)
	}
}

// Both recipe layouts must stay virtualized. An auto-height page column once
// made the table build all 944 rows on every frame (~40 ms), which is what
// made the sidebar pill stutter on the way to 菜式. The text count is the
// observable: a virtualized list builds only the rows on screen, far fewer
// text nodes than there are rows.
func TestRecipeListVirtualizes(t *testing.T) {
	a := newTestApp()
	a.setRecipes(realRecipes(t), map[int64][]domain.Step{})
	a.recomputeRecipeView()
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)

	rows := len(a.recipeView)
	if rows < 500 {
		t.Fatalf("the real seed did not load: %d rows", rows)
	}
	if n := len(tt.Texts()); n > rows {
		t.Fatalf("the table built %d texts for %d rows; it is not virtualizing", n, rows)
	}
}

// The table's first screen holds at least twelve dishes at 1280×860: the
// dense list is the only recipe view left.
func TestRecipeFirstScreenDensity(t *testing.T) {
	a := newTestApp()
	recipes := make([]domain.Recipe, 0, 40)
	minutes := 20.0
	for i := 0; i < 40; i++ {
		recipes = append(recipes, domain.Recipe{
			ID: int64(100 + i), Name: fmt.Sprintf("菜式%02d", i+1),
			Cuisine: "家常菜", Region: "川菜", MainCategory: "猪肉",
			DurationMinutes: &minutes, BaseServings: 2,
		})
	}
	a.setRecipes(recipes, map[int64][]domain.Step{})
	tt := ui.NewTester(a.View, 1280, 860)
	openRecipes(t, tt)
	shown := 0
	for _, rec := range recipes {
		if _, ok := tt.Find(rec.Name); ok {
			shown++
		}
	}
	if shown < 12 {
		t.Fatalf("first screen shows %d of 12 dishes", shown)
	}
}
