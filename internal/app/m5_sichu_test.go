package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/services/ai"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
	"github.com/gij072858-cmd/foodbox-go/internal/services/nutrition"
	"github.com/gij072858-cmd/foodbox-go/internal/services/sichu"
)

func sichuMinutes(v float64) *float64 { return &v }

// sichuFixtureRecipes adds the dishes a plan needs: protein, vegetable, soup
// and one-bowl shapes, plus a peanut dish so member allergies matter.
func sichuFixtureRecipes() []domain.Recipe {
	mk := func(id int64, name, cuisine, category string, duration float64, ings ...string) domain.Recipe {
		lines := make([]domain.Ingredient, 0, len(ings))
		for _, in := range ings {
			lines = append(lines, domain.Ingredient{Name: in, Amount: 100, Unit: "g", Required: true})
		}
		return domain.Recipe{
			ID: id, Name: name, Cuisine: cuisine, MainCategory: category,
			DurationMinutes: sichuMinutes(duration), BaseServings: 2, IsDish: true,
			VersionGroup: name, Ingredients: lines,
		}
	}
	return []domain.Recipe{
		mk(101, "青椒肉丝", "家常菜", "猪肉", 20, "猪肉", "青椒"),
		mk(102, "香煎鸡胸", "快手菜", "鸡肉", 20, "鸡胸肉"),
		mk(103, "清蒸鲈鱼", "蒸菜", "鱼类", 25, "鲈鱼"),
		mk(104, "红烧肉", "炖菜", "猪肉", 60, "五花肉"),
		mk(105, "蒜蓉西兰花", "快手菜", "蔬菜", 15, "西兰花"),
		mk(106, "清炒时蔬", "家常菜", "蔬菜", 15, "时蔬"),
		mk(107, "香菇青菜", "快炒", "菌菇", 15, "香菇", "青菜"),
		mk(108, "番茄蛋花汤", "汤羹", "蛋类", 10, "番茄", "鸡蛋"),
		mk(109, "紫菜蛋花汤", "汤羹", "蛋类", 10, "紫菜", "鸡蛋"),
		mk(110, "冬瓜排骨汤", "汤羹", "猪肉", 40, "冬瓜", "排骨"),
		mk(111, "黄金蛋炒饭", "快手菜", "主食", 10, "米饭", "鸡蛋"),
		mk(112, "番茄牛肉面", "家常菜", "主食", 20, "面条", "牛肉"),
		mk(113, "宫保鸡丁", "快手菜", "鸡肉", 20, "鸡腿", "花生米"),
	}
}

// newSichuTestApp is the ready app with a filled body profile and one child
// member, plus the dishes a plan needs.
func newSichuTestApp() *App {
	a := newTestApp()
	for _, rec := range sichuFixtureRecipes() {
		a.recipes = append(a.recipes, rec)
		a.recipeByID[rec.ID] = rec
		if a.steps == nil {
			a.steps = map[int64][]domain.Step{}
		}
		a.steps[rec.ID] = []domain.Step{{
			RecipeID: rec.ID, OrderIndex: 1, Instruction: "做熟",
			DurationMinutes: rec.DurationMinutes, Resources: []string{"stove"},
		}}
	}
	a.sched.stoveCount = 2
	a.computeRecipeMatches()
	a.sichu.meals = map[string]bool{"lunch": true}
	a.sichu.servings = 2
	// The nutrition table is what makes the result page show kcal/protein;
	// production loads it in loadSichuState, the fixture loads it here.
	if catalog, err := nutrition.NewCatalog(); err == nil {
		a.sichu.catalog = catalog
	}
	a.sichu.questionnaire = domain.UserProfile{Spicy: "mild", Saltiness: "light"}
	a.sichu.members = []domain.MemberProfile{
		{ID: 1, Name: "本人", Role: "adult", IsSelf: true, HeightCm: 175, WeightKg: 70},
		{ID: 2, Name: "小满", Role: "kid", Age: 8, Allergens: "peanut"},
	}
	a.setSichuProfile(domain.SichuProfile{
		HeightCm: 175, WeightKg: 70, Age: 30, Sex: "male",
		ActivityLevel: "moderate", Restrictions: "halal", ExploreRatio: 0.3,
	})
	return a
}

// openSichuStore opens a migrated database for the tests that write.
func openSichuStore(t *testing.T) testRepo {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "m5.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

// The entry page generates a plan, the result page adds it to the schedule
// and swaps one dish.
func TestSichuGenerateFlow(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)

	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("生成配餐") {
		t.Fatalf("entry page missing: %q", tt.Texts())
	}
	if err := tt.Click("生成配餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	if a.router.Path() != "/sichu" {
		t.Errorf("path = %q", a.router.Path())
	}
	if !tt.HasText("换一批") || !tt.HasText("推入「掌序」开做") {
		t.Fatalf("result column missing actions: %q", tt.Texts())
	}
	if len(a.sichu.plan.Meals) != 1 || len(a.sichu.plan.Meals[0].Dishes) != 3 {
		t.Fatalf("plan = %+v", a.sichu.plan)
	}

	before := a.sichu.plan.Meals[0].Dishes[0].Name
	if err := tt.Click("重抽"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	after := a.sichu.plan.Meals[0].Dishes[0].Name
	if before == after {
		t.Errorf("重抽 did not change the dish: %q", after)
	}
	if a.sichu.resultNotice != "已换一道。" {
		t.Errorf("replace notice = %q", a.sichu.resultNotice)
	}

	if err := tt.Click("推入「掌序」开做"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.router.Path() != "/schedule" {
		t.Errorf("path after push = %q", a.router.Path())
	}
	if !strings.Contains(a.sichu.resultNotice, "已加入掌序 3 道") {
		t.Errorf("notice = %q", a.sichu.resultNotice)
	}
}

// A plan with lunch and dinner reaches 掌序 as two meal groups, and pressing
// the push button again does not enqueue them twice.
func TestSichuPushMealsGrouped(t *testing.T) {
	a := newSichuTestApp()
	a.sichu.meals = map[string]bool{"lunch": true, "dinner": true}
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	if len(a.sichu.plan.Meals) != 2 {
		t.Fatalf("plan meals = %d, want 2", len(a.sichu.plan.Meals))
	}
	if !a.addPlanToSchedule() {
		t.Fatal("the plan was not pushed")
	}
	groups := a.scheduleMealGroups()
	if len(groups) != 2 {
		t.Fatalf("meal groups = %+v, want lunch and dinner", groups)
	}
	if groups[0].Meal != "午餐" || groups[1].Meal != "晚餐" {
		t.Errorf("groups = %q, %q", groups[0].Meal, groups[1].Meal)
	}
	if !strings.Contains(a.sichu.resultNotice, "午餐") || !strings.Contains(a.sichu.resultNotice, "晚餐") {
		t.Errorf("notice = %q", a.sichu.resultNotice)
	}
	before := len(a.schedule)
	if !a.addPlanToSchedule() {
		t.Fatal("a pushed plan should stay pushed")
	}
	if len(a.schedule) != before {
		t.Errorf("a second push grew the queue from %d to %d", before, len(a.schedule))
	}
}

// The plan respects the declared limits: a halal diner never gets pork.
func TestSichuPlanRespectsHalal(t *testing.T) {
	a := newSichuTestApp()
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			rec, ok := a.recipeByID[dish.RecipeID]
			if !ok {
				continue
			}
			if rec.MainCategory == "猪肉" {
				t.Errorf("halal plan contains pork: %q", dish.Name)
			}
		}
	}
}

// Switching the diner narrows the constraints: the child's peanut allergy
// removes peanut dishes.
func TestSichuPlanRespectsDinerAllergy(t *testing.T) {
	a := newSichuTestApp()
	a.sichu.whoID = 2
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			rec, ok := a.recipeByID[dish.RecipeID]
			if !ok {
				continue
			}
			for _, ing := range rec.Ingredients {
				if strings.Contains(ing.Name, "花生") {
					t.Errorf("allergen dish planned: %q", dish.Name)
				}
			}
		}
	}
}

// A one-bowl plan has a core dish and at most one soup, and passes the local
// contract.
func TestSichuOneBowlFlow(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("一人食简餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("生成配餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	meal := a.sichu.plan.Meals[0]
	if len(meal.Dishes) < 1 || len(meal.Dishes) > 2 {
		t.Fatalf("one-bowl dishes = %d", len(meal.Dishes))
	}
	if !sichu.IsOneBowlName(meal.Dishes[0].Name) {
		t.Errorf("core is not a one-bowl dish: %q", meal.Dishes[0].Name)
	}
	if err := sichu.Validate(a.sichu.plan, a.buildSichuInput()); err != nil {
		t.Fatalf("one-bowl plan invalid: %v", err)
	}
}

// An exhausted daily quota silently falls back to the offline plan.
func TestSichuQuotaFallsBackLocally(t *testing.T) {
	a := newSichuTestApp()
	a.store = openSichuStore(t)
	a.ai.configured = true
	a.ai.usage = ai.Usage{Date: ai.Today(time.Now()), Used: ai.DailyLimit}
	a.ai.remaining = 0
	a.sichu.engine = sichuEngineAI
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	if a.sichu.planSource != "本地" {
		t.Errorf("source = %q", a.sichu.planSource)
	}
	if !strings.Contains(a.sichu.engineNotice, "配额") {
		t.Errorf("engine notice = %q", a.sichu.engineNotice)
	}
}

// Saving the body page writes sichu_profiles and the self member together.
func TestSichuBodySaveSyncsSelfMember(t *testing.T) {
	a := newSichuTestApp()
	store := openSichuStore(t)
	a.store = store
	a.sichu.body.height = 181
	a.sichu.body.weight = 71

	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("去完善"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("基础数据") {
		t.Fatalf("body page missing: %q", tt.Texts())
	}
	if err := tt.Click("保存身体档案"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("身体档案已保存") {
		t.Errorf("save notice missing: %q", tt.Texts())
	}

	profile, _, err := store.GetSichuProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.HeightCm != 181 || profile.WeightKg != 71 {
		t.Errorf("profile = %+v", profile)
	}
	members, err := store.ListMembers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || !members[0].IsSelf {
		t.Fatalf("members = %+v", members)
	}
	if members[0].HeightCm != 181 || members[0].WeightKg != 71 {
		t.Errorf("self member not synced: %+v", members[0])
	}
}

// Adding and deleting a member goes through the page and the database.
func TestSichuMembersCRUDView(t *testing.T) {
	a := newSichuTestApp()
	store := openSichuStore(t)
	a.store = store

	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("管理成员"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("谁吃这一餐") {
		t.Fatalf("members page missing: %q", tt.Texts())
	}
	if err := tt.Click("添加成员"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	a.sichu.memberEdit.name = "小满"
	a.sichu.memberEdit.role = "小孩"
	a.sichu.memberEdit.age = 8
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	settle(tt)

	members, err := store.ListMembers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members after add = %+v", members)
	}

	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("确认删除"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	members, err = store.ListMembers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("members after delete = %+v", members)
	}
	if a.sichu.whoID != 0 {
		t.Errorf("diner selection not cleared: %d", a.sichu.whoID)
	}
}

// The AI settings page shows the state and refuses an empty key without
// touching the credential store.
func TestAISettingsPage(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("AI 设置"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("未配置") {
		t.Fatalf("ai page missing state: %q", tt.Texts())
	}
	if !tt.HasText("今日剩余 10 / 10 次") {
		t.Errorf("quota line missing: %q", tt.Texts())
	}
	if err := tt.Click("保存密钥"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.ai.notice != "请先粘贴密钥" {
		t.Errorf("notice = %q", a.ai.notice)
	}
	if !tt.HasText("请先粘贴密钥") {
		t.Errorf("notice not shown: %q", tt.Texts())
	}
}

// Every M5 route renders without the app state being loaded: the player can
// walk in before the database finishes.
func TestSichuRoutesRender(t *testing.T) {
	a := newSichuTestApp()
	for _, path := range []string{"/sichu", "/sichu/body", "/sichu/members", "/settings/ai", "/sichu/result"} {
		a.router.Push(path)
		tt := ui.NewTester(a.View, 1280, 860)
		if !tt.HasText("食匣") {
			t.Fatalf("%s rendered nothing", path)
		}
	}
}

// The meal shape is a single choice (a segmented track) while the slots stay
// a multi choice, because one plan covers every slot it is asked for.
func TestSichuFormSegmented(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	// The second mode is 一人食简餐.
	if err := tt.Click("一人食简餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.style != 1 {
		t.Fatalf("style = %d, want 1", a.sichu.style)
	}
	// Slots toggle independently: lunch is on, dinner is added, lunch stays.
	if !a.sichu.meals["lunch"] {
		t.Fatal("lunch started off")
	}
	if err := tt.Click("晚餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sichu.meals["lunch"] || !a.sichu.meals["dinner"] {
		t.Fatalf("slots = %v, want lunch and dinner", a.sichu.meals)
	}
	if err := tt.Click("晚餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.meals["dinner"] || !a.sichu.meals["lunch"] {
		t.Fatalf("slots after untoggling = %v", a.sichu.meals)
	}
}

// Free counts reach the planner: adding a protein on the page produces a
// plan with two protein dishes, and the local contract accepts it.
func TestSichuCustomCounts(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("自由指定菜数"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.style != 2 {
		t.Fatalf("mode = %d, want custom", a.sichu.style)
	}
	// The stepper writes the count; drive the same state it writes, since
	// the tester finds visible text and the buttons are +/− glyphs.
	a.sichu.mains = 2
	tt.Frame()
	if a.sichu.mains != 2 {
		t.Fatalf("mains = %d, want 2", a.sichu.mains)
	}
	if err := tt.Click("生成配餐"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	meal := a.sichu.plan.Meals[0]
	mains := 0
	for _, dish := range meal.Dishes {
		if dish.Slot == sichu.SlotMain {
			mains++
		}
	}
	if mains != 2 {
		t.Errorf("plan has %d protein dishes, want 2: %+v", mains, meal.Dishes)
	}
	if err := sichu.Validate(a.sichu.plan, a.buildSichuInput()); err != nil {
		t.Fatalf("custom plan invalid: %v", err)
	}
}

// The smart mode derives its counts from the number of diners.
func TestSichuSmartCountsFollowServings(t *testing.T) {
	a := newSichuTestApp()
	a.sichu.style = 0
	a.sichu.servings = 4
	in := a.buildSichuInput()
	if in.Style != sichu.StyleCustom {
		t.Fatalf("style = %q, want custom counts", in.Style)
	}
	if in.Counts != (sichu.Counts{Mains: 2, Vegs: 1, Soups: 1}) {
		t.Fatalf("counts = %+v", in.Counts)
	}
}

// sichuTestItem is a pantry row for the stock-policy tests.
func sichuTestItem(id int64, name string, expire *time.Time) domain.Item {
	return domain.Item{
		ID: id, Name: name, Category: "其他", Kind: domain.ItemKindIngredient,
		GranularLevel: domain.GranularPlenty, ExpireAt: expire,
	}
}

// Without a key the AI choice is greyed and cannot be selected; once a key is
// configured it becomes a real switch.
func TestSichuEngineToggle(t *testing.T) {
	a := newSichuTestApp()
	a.ai.configured = false
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("AI"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.engine != sichuEngineLocal {
		t.Fatalf("engine = %d, want the disabled AI choice to stay local", a.sichu.engine)
	}
	if tt.HasText("未配置密钥，生成时用本地策略。") {
		t.Errorf("the duplicate AI warning is still shown: %q", tt.Texts())
	}

	a.ai.configured = true
	tt.Frame()
	settle(tt)
	if err := tt.Click("AI"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.engine != sichuEngineAI {
		t.Fatalf("engine = %d, want AI once configured", a.sichu.engine)
	}
	if err := tt.Click("本地策略"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.engine != sichuEngineLocal {
		t.Fatalf("engine = %d, want local", a.sichu.engine)
	}
}

// Temporary guests add to the effective serving count, including when a
// member is the diner.
func TestSichuGuestServings(t *testing.T) {
	a := newSichuTestApp()
	a.sichu.style = 0
	a.sichu.servings = 2
	a.sichu.guests = 2
	in := a.buildSichuInput()
	if in.Servings != 4 {
		t.Fatalf("servings = %d, want 4", in.Servings)
	}
	if in.Counts != (sichu.Counts{Mains: 2, Vegs: 1, Soups: 1}) {
		t.Fatalf("counts = %+v", in.Counts)
	}
	a.sichu.whoID = 2
	a.sichu.guests = 1
	in = a.buildSichuInput()
	if in.Servings != 2 {
		t.Fatalf("member + guest servings = %d, want 2", in.Servings)
	}
}

// Checking the box turns the last three days into a hard exclusion.
func TestSichuAvoidRecentExcludes(t *testing.T) {
	a := newSichuTestApp()
	now := time.Now()
	a.records = []domain.Record{
		{ID: 1, RecipeID: 102, CompletedAt: now.Add(-time.Hour)},
		{ID: 2, RecipeID: 105, CompletedAt: now.Add(-2 * time.Hour)},
		{ID: 3, RecipeID: 108, CompletedAt: now.Add(-3 * time.Hour)},
	}
	a.sichu.avoidRecent3d = true
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	recent := map[int64]bool{102: true, 105: true, 108: true}
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			if recent[dish.RecipeID] {
				t.Errorf("recent dish planned again: %q", dish.Name)
			}
		}
	}
}

// 严守库存 keeps only fully covered dishes.
func TestSichuStockPolicyStrict(t *testing.T) {
	a := newSichuTestApp()
	names := []string{"鸡胸肉", "西兰花", "番茄", "鸡蛋", "香菇", "青菜", "时蔬", "紫菜", "鸡腿", "花生米", "米饭"}
	items := make([]domain.Item, 0, len(names))
	for i, name := range names {
		items = append(items, sichuTestItem(int64(i+1), name, nil))
	}
	a.items = items
	a.computeRecipeMatches()
	a.sichu.stockPolicy = sichuStockStrict
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	stock := a.matcher.StockSet(a.items)
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			rec, ok := a.recipeByID[dish.RecipeID]
			if !ok {
				continue
			}
			if got := a.matcher.MatchWithStock(rec, stock).Status; got != cookmatch.CanCook {
				t.Errorf("%s under strict stock = %q", rec.Name, got)
			}
		}
	}
}

// 严守库存 with an empty pantry refuses early instead of planning.
func TestSichuStockPolicyStrictInsufficient(t *testing.T) {
	a := newSichuTestApp()
	a.items = nil
	a.computeRecipeMatches()
	a.sichu.stockPolicy = sichuStockStrict
	a.generateSichuPlan(false)
	if a.sichu.planReady {
		t.Fatal("strict stock planned from an empty pantry")
	}
	if !strings.Contains(a.sichu.planError, "严守库存") {
		t.Errorf("error = %q", a.sichu.planError)
	}
}

// 临期优先 narrows the pool to dishes that consume something expiring.
func TestSichuStockPolicyExpiring(t *testing.T) {
	a := newSichuTestApp()
	now := time.Now()
	soon := now.Add(48 * time.Hour)
	later := now.Add(30 * 24 * time.Hour)
	items := []domain.Item{
		sichuTestItem(1, "鸡胸肉", &soon),
		sichuTestItem(2, "西兰花", &soon),
		sichuTestItem(3, "番茄", &soon),
		sichuTestItem(4, "鸡蛋", &soon),
		sichuTestItem(5, "紫菜", &later),
		sichuTestItem(6, "时蔬", &later),
	}
	a.items = items
	a.computeRecipeMatches()
	a.sichu.stockPolicy = sichuStockExpiring
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	expiring := make(map[string]bool)
	for _, it := range items {
		if it.NeedsEatingFirst(now.Unix()) {
			expiring[a.matcher.Canonical(it.Name)] = true
		}
	}
	for _, meal := range a.sichu.plan.Meals {
		for _, dish := range meal.Dishes {
			rec, ok := a.recipeByID[dish.RecipeID]
			if !ok {
				continue
			}
			uses := false
			for _, ing := range rec.Ingredients {
				if ing.Required && expiring[a.matcher.Canonical(ing.Name)] {
					uses = true
					break
				}
			}
			if !uses {
				t.Errorf("%s does not use an expiring ingredient", rec.Name)
			}
		}
	}
}

// A locked dish survives 换一批.
func TestSichuLockPreservedAcrossRegenerate(t *testing.T) {
	a := newSichuTestApp()
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	dish := a.sichu.plan.Meals[0].Dishes[0]
	a.toggleSichuLock(0, 0, dish)
	a.generateSichuPlan(true)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	if got := a.sichu.plan.Meals[0].Dishes[0].RecipeID; got != dish.RecipeID {
		t.Errorf("locked dish changed: got %d, want %d", got, dish.RecipeID)
	}
	if !strings.Contains(a.sichu.resultNotice, "保留") {
		t.Errorf("notice = %q", a.sichu.resultNotice)
	}
}

// Presets fill the left card and generate in one tap.
func TestSichuPresetOneBowl(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("快手一人食"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.style != 1 || a.sichu.servings != 1 || a.sichu.guests != 0 {
		t.Fatalf("preset state = style %d, servings %.0f, guests %d",
			a.sichu.style, a.sichu.servings, a.sichu.guests)
	}
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	if !sichu.IsOneBowlName(a.sichu.plan.Meals[0].Dishes[0].Name) {
		t.Errorf("not a one-bowl plan: %+v", a.sichu.plan.Meals[0].Dishes)
	}
	if a.router.Path() != "/sichu" {
		t.Errorf("path = %q", a.router.Path())
	}
}

func TestSichuPresetTwoPerson(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("两人两菜一汤"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.style != 0 || a.sichu.servings != 2 || a.sichu.guests != 0 {
		t.Fatalf("preset state = style %d, servings %.0f, guests %d",
			a.sichu.style, a.sichu.servings, a.sichu.guests)
	}
	if !a.sichu.planReady || len(a.sichu.plan.Meals[0].Dishes) != 3 {
		t.Fatalf("plan = %+v (%q)", a.sichu.plan, a.sichu.planError)
	}
}

// The board shows the pantry summary, and an expiring tag activates the
// left-hand policy.
func TestSichuDashboardStatsAndExpiringClick(t *testing.T) {
	a := newSichuTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("私厨"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.dash.FullStocked <= 0 {
		t.Errorf("full-stocked = %d", a.sichu.dash.FullStocked)
	}
	if len(a.sichu.dash.Expiring) == 0 {
		t.Fatal("no expiring items in the fixture")
	}
	if a.sichu.dash.NutritionHint == "" || a.sichu.dash.NutritionVerdict == "" {
		t.Errorf("nutrition hint = %q / %q", a.sichu.dash.NutritionVerdict, a.sichu.dash.NutritionHint)
	}
	name := a.sichu.dash.ExpiringNames[0]
	if err := tt.Click(name); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sichu.stockPolicy != sichuStockExpiring {
		t.Errorf("stock policy = %d, want expiring", a.sichu.stockPolicy)
	}
}

// The generated plan carries a parallel-time estimate from the schedule
// engine.
func TestSichuPlanMinutes(t *testing.T) {
	a := newSichuTestApp()
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	if a.sichu.planMinutes <= 0 {
		t.Errorf("plan minutes = %d", a.sichu.planMinutes)
	}
}

// /sichu/result is a compatibility alias for the in-page result.
func TestSichuResultInPage(t *testing.T) {
	a := newSichuTestApp()
	a.generateSichuPlan(false)
	if !a.sichu.planReady {
		t.Fatalf("plan not ready: %q", a.sichu.planError)
	}
	a.router.Push("/sichu/result")
	tt := ui.NewTester(a.View, 1280, 860)
	if !tt.HasText("推入「掌序」开做") {
		t.Errorf("result alias missing actions: %q", tt.Texts())
	}
}
