package app

import (
	"context"
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/repository/memory"
)

// testRepo is the real store plus the few reads and writes the tests make
// outside the app's own interface. Naming the interface here keeps the
// concrete SQLite type out of the app package, tests included.
type testRepo interface {
	repository.Repository
	RecipeCount(ctx context.Context) (int, error)
	RecipeByID(ctx context.Context, id int64) (domain.Recipe, error)
	MetaGet(ctx context.Context, key string) (string, error)
	MetaSet(ctx context.Context, key, value string) error
	SettleRun(ctx context.Context, recipeIDs []int64, completedAt time.Time, sessionSeconds *int, note string) error
	GetJournal(ctx context.Context, date string) (domain.CookJournal, bool, error)
}

// settle advances past the page transition, which moves a new page in.
func settle(tt *ui.Tester) { tt.Advance(500 * time.Millisecond) }

// newTestApp builds the ready state directly. Production reaches it through
// bootstrap(), which needs a window and a real database; the views themselves
// only need the loaded slices.
func newTestApp() *App {
	a := New()
	a.phase = PhaseReady
	a.recipes = fixtureRecipes()
	a.recipeByID = make(map[int64]domain.Recipe, len(a.recipes))
	for _, r := range a.recipes {
		a.recipeByID[r.ID] = r
	}
	a.steps = fixtureSteps()
	a.catalog = fixtureCatalog()
	a.catalogGroups = groupCatalog(a.catalog)
	a.catalogIndex = make(map[string]domain.FoodItem, len(a.catalog))
	a.catalogNames = make([]string, 0, len(a.catalog))
	for _, it := range a.catalog {
		a.catalogIndex[it.Name] = it
		a.catalogNames = append(a.catalogNames, it.Name)
	}
	a.packs = []domain.StarterPack{{
		ID: "test", Region: "测试地域",
		Core: []string{"番茄", "黄瓜"}, Optional: []string{"土豆"},
	}}
	a.setItems(fixtureItems())
	a.recomputeRecipeView()
	// The settings page reads this probe instead of touching the disk and
	// the credential store every frame; bootstrap fills it in the real app.
	a.videoProbe = videoProbeState{foundOK: true, cookieOK: true}
	return a
}

func fixtureItems() []domain.Item {
	now := time.Now()
	days := func(n int) *time.Time { t := now.AddDate(0, 0, n); return &t }
	qty := func(n float64) *float64 { return &n }
	shelf := func(n int) *int { return &n }
	return []domain.Item{
		{ID: 1, Name: "番茄", Category: "蔬菜", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: qty(4), Unit: "个",
			StoredAt: &now, ShelfLifeDays: shelf(2), ExpireAt: days(2),
			StorageLocation: domain.StorageFridge,
			IntroWhat:       "番茄是常见的果菜。", IntroStore: "冷藏保存，避免挤压。"},
		{ID: 2, Name: "土豆", Category: "根茎", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularSome,
			StoredAt: &now, ShelfLifeDays: shelf(30), ExpireAt: days(30),
			StorageLocation: domain.StorageRoomTemp,
			IntroWhat:       "土豆适合炖炒。", IntroStore: "阴凉干燥处保存。"},
		{ID: 3, Name: "鸡蛋", Category: "蛋类", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: qty(6), Unit: "个",
			StoredAt: &now, ExpireAt: days(10), StorageLocation: domain.StorageFridge},
		{ID: 4, Name: "生抽", Category: "调味汁", Kind: domain.ItemKindSeasoning,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularPlenty,
			StorageLocation: domain.StorageRoomTemp},
		{ID: 5, Name: "香菇", Category: "菌菇", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularLow,
			StoredAt: &now, StorageLocation: domain.StorageFridge},
		{ID: 6, Name: "青椒", Category: "蔬菜", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: qty(0.5), Unit: "kg",
			StoredAt: &now, ExpireAt: days(-1), StorageLocation: domain.StorageFridge,
			IsDepleted: true},
	}
}

func fixtureRecipes() []domain.Recipe {
	minutes := func(n float64) *float64 { return &n }
	return []domain.Recipe{
		{ID: 1, Name: "青椒肉丝", Cuisine: "家常菜", Region: "川菜", MainCategory: "猪肉",
			DurationMinutes: minutes(20), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			Ingredients: []domain.Ingredient{
				{Name: "猪肉", Amount: 300, Unit: "g", Required: true},
				{Name: "青椒", Amount: 60, Unit: "g", Required: true},
			}},
		{ID: 2, Name: "番茄炒蛋", Cuisine: "家常菜", Region: "家常", MainCategory: "蛋类",
			DurationMinutes: minutes(15), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{ID: 3, Name: "红烧肉", Cuisine: "炖菜", Region: "本帮菜", MainCategory: "猪肉",
			DurationMinutes: minutes(90), Difficulty: domain.DifficultyMedium, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{ID: 4, Name: "麻婆豆腐", Cuisine: "家常菜", Region: "川菜", MainCategory: "豆制品",
			DurationMinutes: minutes(25), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{ID: 5, Name: "鱼香肉丝", Cuisine: "快炒", Region: "川菜", MainCategory: "猪肉",
			DurationMinutes: minutes(25), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{ID: 6, Name: "宫保鸡丁", Cuisine: "快炒", Region: "川菜", MainCategory: "禽肉",
			DurationMinutes: minutes(25), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{ID: 7, Name: "清蒸鲈鱼", Cuisine: "蒸菜", Region: "粤菜", MainCategory: "鱼类",
			DurationMinutes: minutes(30), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
		{ID: 8, Name: "香菇青菜", Cuisine: "快炒", Region: "家常", MainCategory: "菌菇",
			DurationMinutes: minutes(15), Difficulty: domain.DifficultyEasy, BaseServings: 2,
			SeasonMonths: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
	}
}

func fixtureSteps() map[int64][]domain.Step {
	m := make(map[int64][]domain.Step)
	for _, r := range fixtureRecipes() {
		for i := 0; i < 4; i++ {
			m[r.ID] = append(m[r.ID], domain.Step{
				RecipeID:    r.ID,
				OrderIndex:  i + 1,
				Instruction: fmt.Sprintf("%s：第 %d 步，按说明处理食材并加热至熟。", r.Name, i+1),
			})
		}
	}
	return m
}

func fixtureCatalog() []domain.FoodItem {
	return []domain.FoodItem{
		{Name: "番茄", Category: "蔬菜", Kind: "ingredient", StorageLocation: "fridge"},
		{Name: "黄瓜", Category: "蔬菜", Kind: "ingredient", StorageLocation: "fridge"},
		{Name: "土豆", Category: "蔬菜", Kind: "ingredient", StorageLocation: "roomTemp"},
		{Name: "猪肉", Category: "禽肉", Kind: "ingredient", StorageLocation: "freezer"},
		{Name: "酱油", Category: "调味汁", Kind: "seasoning", StorageLocation: "roomTemp"},
		{Name: "盐", Category: "调味料", Kind: "seasoning", StorageLocation: "roomTemp"},
	}
}

// The shell opens on the pantry, and the sidebar changes the page.
func TestShellNavigation(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if !tt.HasText("仓库") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("新建菜式") {
		t.Errorf("texts after 菜式: %q", tt.Texts())
	}
	if err := tt.Click("掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("0 道菜 · 预计 0 分钟") {
		t.Errorf("texts after 掌序: %q", tt.Texts())
	}
	if !tt.HasText("掌序里还没有菜") {
		t.Errorf("empty schedule state missing: %q", tt.Texts())
	}
}

// Cmd/Ctrl+2 reaches the recipe section and Cmd/Ctrl+F puts the caret in
// the sidebar search, as the help page promises.
func TestKeyboardShortcuts(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	tt.Key(ui.Cmd, ui.Key2)
	settle(tt)
	if !tt.HasText("新建菜式") {
		t.Fatalf("Cmd+2 did not open 菜式: %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyF)
	if !tt.Focused("搜索菜式与食材") {
		t.Errorf("Cmd+F did not focus the search field")
	}
}

// The pantry shows real items, filters them, and opens the catalog.
func TestPantryViews(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if !tt.HasText("在库 5 样") {
		t.Fatalf("pantry overview missing: %q", tt.Texts())
	}
	if !tt.HasText("先吃我") {
		t.Errorf("expiring band missing: %q", tt.Texts())
	}
	if err := tt.Click("土豆"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("阴凉干燥处保存。") {
		t.Errorf("detail intro missing: %q", tt.Texts())
	}
	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("+ 添加食材"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("从系统目录添加"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("系统食材库") {
		t.Errorf("catalog page missing: %q", tt.Texts())
	}
}

// Filtering by 「用完」 shows only depleted rows.
func TestPantryFilterDepleted(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("用完(1)"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("青椒") {
		t.Errorf("depleted item missing: %q", tt.Texts())
	}
	if tt.HasText("鸡蛋") {
		t.Errorf("non-depleted item still shown: %q", tt.Texts())
	}
}

// Settling a run clears the schedule and writes a feed record.
func TestSettleRun(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("加入掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("结算并扣库存"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("确认结算"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("掌序里还没有菜") {
		t.Errorf("schedule not cleared: %q", tt.Texts())
	}
	if err := tt.Click("食历"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("青椒肉丝") || !tt.HasText("+10 XP") {
		t.Errorf("feed record missing: %q", tt.Texts())
	}
}

// Starter packs, quick edit and shopping list are reachable.
func TestPantryStarterAndShopping(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("新手包"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("测试地域") {
		t.Fatalf("starter page missing: %q", tt.Texts())
	}
	if err := tt.Click("测试地域"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("加入这些食材") {
		t.Errorf("pack detail missing: %q", tt.Texts())
	}

	a = newTestApp()
	tt = ui.NewTester(a.View, 1280, 860)
	if !tt.HasText("购物清单 1") {
		t.Fatalf("shopping count missing: %q", tt.Texts())
	}
	if err := tt.Click("购物清单 1"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("青椒") || !tt.HasText("仓库里已用完") {
		t.Errorf("shopping page missing: %q", tt.Texts())
	}
}

// A recipe card opens its detail page, which the back button leaves.
func TestRecipeDetail(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("青椒肉丝"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("步骤") {
		t.Errorf("texts after opening a recipe: %q", tt.Texts())
	}
	if err := tt.Click("加入掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	// Adding no longer throws the cook into 掌序; the dish page stays put.
	if !strings.HasPrefix(a.router.Path(), "/recipes/") {
		t.Fatalf("path after adding = %q, want the dish page", a.router.Path())
	}
	if err := tt.Click("掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("1 道菜 · 预计 20 分钟") {
		t.Errorf("texts after adding to schedule: %q", tt.Texts())
	}
	// M4: the queue lists the dish with its servings, steps and duration.
	if !tt.HasText("2 人份 · 4 步 · 20 分钟") {
		t.Errorf("queue row missing after adding to schedule: %q", tt.Texts())
	}
	if !tt.HasText("开始掌序") {
		t.Errorf("run entry missing from the schedule page: %q", tt.Texts())
	}
}

// The title bar's crumbs go to the parent that exists, not through the
// history: Replace, so Back still returns to the page the user came from.
func TestShellBreadcrumbPop(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("新手包"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.router.Path() != "/pantry/starter" {
		t.Fatalf("path = %q", a.router.Path())
	}
	last, ok := tt.Find("新手包")
	if !ok {
		t.Fatalf("the crumb is missing: %q", tt.Texts())
	}
	// The section crumb sits to the left of the leaf crumb on the same row.
	tt.ClickAt(last.X-60, last.Y+last.H/2)
	settle(tt)
	if a.router.Path() != "/pantry" {
		t.Fatalf("breadcrumb did not pop: path = %q", a.router.Path())
	}
}

// Back and Forward are the router's history, not new pushes.
func TestShellBackForward(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("新手包"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.router.Path() != "/pantry" {
		t.Fatalf("back: path = %q", a.router.Path())
	}
	if err := tt.Click("Forward"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.router.Path() != "/pantry/starter" {
		t.Fatalf("forward: path = %q", a.router.Path())
	}
}

// Typing a destination's pinyin moves the sidebar there; c lands on 菜式,
// the first match by pinyin.
func TestSidebarTypeahead(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	tt.Frame()
	// The letters live on the tree, so the keyboard must be in it: clicking
	// the chosen destination focuses it without changing the page.
	if err := tt.Click("仓库"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	tt.Key(0, ui.KeyC)
	settle(tt)
	if a.router.Path() != "/recipes" {
		t.Fatalf("c went to %q", a.router.Path())
	}
	tt.Key(0, ui.KeyHome)
	settle(tt)
	if a.router.Path() != "/pantry" {
		t.Fatalf("Home went to %q", a.router.Path())
	}
	tt.Key(0, ui.KeyEnd)
	settle(tt)
	if a.router.Path() != "/settings" {
		t.Fatalf("End went to %q", a.router.Path())
	}
}

// A letter typed into the search field is a letter, not a destination.
func TestSidebarTypeaheadLeavesTypingAlone(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	if !tt.Focused("搜索菜式与食材") {
		t.Fatalf("Cmd+F did not focus the search field")
	}
	// On the recipes page, "z" is a search word; were it read as a
	// destination it would open 掌序.
	clickText(t, tt, "菜式")
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("z")
	settle(tt)
	if a.router.Path() != "/recipes" {
		t.Fatalf("typing in search navigated to %q", a.router.Path())
	}
}

// Picking a template and turning one optional item off changes exactly
// that item in what reaches the pantry.
func TestPantryOptionalToggle(t *testing.T) {
	pack := domain.StarterPack{
		ID: "test-pack", Region: "测试地域",
		Core: []string{"猪肉"}, Optional: []string{"酱油"},
	}
	add := func(master, pick bool) int {
		store := memory.New()
		a := newTestApp()
		a.store = store
		a.packs = []domain.StarterPack{pack}
		a.selectPack(pack)
		a.packOptional = master
		a.packPicks["酱油"] = pick
		a.addPack(pack)
		var n int
		waitFor(t, "pantry written", func() bool {
			items, err := store.ListItems(context.Background())
			n = len(items)
			return err == nil && n > 0
		})
		return n
	}
	if n := add(false, true); n != 1 {
		t.Fatalf("core only added %d items, want 1", n)
	}
	if n := add(true, true); n != 2 {
		t.Fatalf("with the optional item on added %d items, want 2", n)
	}
	if n := add(true, false); n != 1 {
		t.Fatalf("with the optional item off added %d items, want 1", n)
	}

	// The same switch is on the page, on the item itself.
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := tt.Click("新手包"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if err := tt.Click("测试地域"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.packPicks["土豆"] {
		t.Fatalf("picks after selecting the template = %v", a.packPicks)
	}
	if err := tt.Click("土豆"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.packPicks["土豆"] {
		t.Fatal("the optional item stayed on after its toggle")
	}
}

// A prefs blob written by the folded-sidebar version carries nav_fold; the
// sidebar no longer folds, so every destination must still be there and the
// old key must simply be ignored.
func TestOldNavFoldPreferenceIsIgnored(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	if err := store.MetaSetJSON(ctx, "prefs", map[string]any{
		"motion": "full", "theme": "auto", "nav_fold": []bool{false, false, true},
	}); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.store = store
	a.phase = PhaseReady
	if err := a.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	for _, want := range []string{"仓库", "菜式", "私厨", "掌序", "食历", "营养", "我的", "帮助", "设置"} {
		if !tt.HasText(want) {
			t.Errorf("the old fold preference hid the %q entry: %q", want, tt.Texts())
		}
	}
}

// The selected pill is still the matched element Cloudy's sidebar needs:
// one accent rectangle in the sidebar, and it moves to the item chosen
// without changing its shape.
func TestSidebarPillFollowsSelection(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	accent := design.PaletteFor(a.themeName, false).Accent
	near := func(v, want uint8) bool {
		d := int(v) - int(want)
		return d >= -8 && d <= 8
	}
	// The pill is a wide accent fill; the focus ring around the chosen
	// button is a thin accent outline. Counting per row picks the fill and
	// ignores the ring.
	box := func() image.Rectangle {
		img := tt.Image()
		r := image.Rectangle{Min: image.Point{X: 1 << 20, Y: 1 << 20}, Max: image.Point{X: -1, Y: -1}}
		for y := 0; y < 860; y++ {
			run, first, last := 0, -1, -1
			for x := 0; x < design.SidebarWidth; x++ {
				i := img.PixOffset(x, y)
				if near(img.Pix[i], accent.R) && near(img.Pix[i+1], accent.G) && near(img.Pix[i+2], accent.B) {
					run++
					if first < 0 {
						first = x
					}
					last = x
				}
			}
			if run < 100 {
				continue
			}
			r.Min.X = min(r.Min.X, first)
			r.Min.Y = min(r.Min.Y, y)
			r.Max.X = max(r.Max.X, last+1)
			r.Max.Y = max(r.Max.Y, y+1)
		}
		return r
	}
	before := box()
	if before.Empty() {
		t.Fatal("no pill in the sidebar")
	}
	clickText(t, tt, "菜式")
	after := box()
	if after.Empty() {
		t.Fatal("the pill vanished after choosing another page")
	}
	if after.Min.Y <= before.Min.Y {
		t.Errorf("the pill did not follow: %v -> %v", before, after)
	}
	if d := after.Dx() - before.Dx(); d > 2 || d < -2 {
		t.Errorf("the pill changed width: %v -> %v", before, after)
	}
	if d := after.Dy() - before.Dy(); d > 2 || d < -2 {
		t.Errorf("the pill changed height: %v -> %v", before, after)
	}
}

// hasTextContaining finds a substring among the frame's labels, for text
// that is assembled with numbers at run time.
func hasTextContaining(tt *ui.Tester, sub string) bool {
	for _, text := range tt.Texts() {
		if strings.Contains(text, sub) {
			return true
		}
	}
	return false
}

// At 1280×860 the pantry's first screen holds at least eighteen items:
// four columns by five rows.
func TestPantryFirstScreenDensity(t *testing.T) {
	a := newTestApp()
	items := make([]domain.Item, 0, 18)
	for i := 0; i < 18; i++ {
		items = append(items, domain.Item{
			ID: int64(i + 1), Name: fmt.Sprintf("食材%02d", i+1),
			Category: "蔬菜", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularSome,
			StorageLocation: domain.StorageFridge,
		})
	}
	a.setItems(items)
	tt := ui.NewTester(a.View, 1280, 860)
	settle(tt)
	shown := 0
	for _, it := range items {
		if _, ok := tt.Find(it.Name); ok {
			shown++
		}
	}
	if shown < 18 {
		t.Fatalf("first screen shows %d of 18 items", shown)
	}
}
