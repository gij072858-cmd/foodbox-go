package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository/memory"
	"github.com/gij072858-cmd/foodbox-go/internal/services/review"
)

// openM6Store opens an empty migrated database for the M6 tests.
func openM6Store(t *testing.T) testRepo {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "m6.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

// m6Fixture is a ready app with a cook history, a day diary and an account.
func m6Fixture(t *testing.T) *App {
	t.Helper()
	a := newTestApp()
	now := time.Now().In(time.Local)
	day := func(offset, hour, minute int) time.Time {
		d := now.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, time.Local)
	}
	seconds := 2400
	records := []domain.Record{
		{ID: 1, RecipeID: 1, CompletedAt: day(0, 12, 0), XPGained: 10, SessionSeconds: &seconds},
		{ID: 2, RecipeID: 2, CompletedAt: day(0, 12, 0), XPGained: 10, SessionSeconds: &seconds},
		{ID: 3, RecipeID: 1, CompletedAt: day(-1, 18, 30), XPGained: 10},
		{ID: 4, RecipeID: 3, CompletedAt: day(-3, 19, 0), XPGained: 10},
		{ID: 5, RecipeID: 7, CompletedAt: day(-10, 12, 0), XPGained: 10},
	}
	a.records = records
	a.recordCount = len(records)
	a.mealDays = 4
	a.favorites = map[int64]bool{7: true}
	a.prefs.profile.loaded = true
	a.prefs.profile.profile = domain.Profile{
		ID: 1, Nickname: "小满", XP: 250, TotalMeals: 4,
		FavoriteRecipes: []int64{7}, WantedRecipes: []int64{3},
	}
	a.feed.recipes = map[int64]review.Recipe{}
	for _, rec := range a.recipes {
		a.feed.recipes[rec.ID] = review.Recipe{
			Name: rec.Name, Cuisine: rec.Cuisine, Region: rec.Region,
			MainCategory: rec.MainCategory,
		}
	}
	today := now.Format("2006-01-02")
	a.feed.journals = map[string]domain.CookJournal{
		today: {Date: today, Note: "今天试了少放盐。"},
	}
	return a
}

// clickText clicks a label and advances past the page transition.
func clickText(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v", label, err)
	}
	settle(tt)
}

// waitFor polls cond until it holds or the test fails.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The calendar shows the month, the streak and the latest meals, and a day
// opens its diary.
func TestFeedCalendarAndDayJournal(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	clickText(t, tt, "食历")

	if !tt.HasText("最近开饭") {
		t.Fatalf("recent meals missing: %q", tt.Texts())
	}
	if !tt.HasText("青椒肉丝") || tt.HasText("XP") {
		t.Errorf("latest meals missing or XP still shown: %q", tt.Texts())
	}
	if !hasTextContaining(tt, "连续 2 天") {
		t.Errorf("streak line missing: %q", tt.Texts())
	}
	now := time.Now().In(time.Local)
	if !hasTextContaining(tt, fmt.Sprintf("%d 年 %d 月", now.Year(), int(now.Month()))) {
		t.Errorf("month heading missing: %q", tt.Texts())
	}
	// The calendar keeps its intrinsic day-grid width instead of stretching
	// across the split pane, which is what centres the month title over it.
	if cal, ok := tt.Find("开饭月历"); !ok {
		t.Errorf("calendar has no label")
	} else if cal.W > 320 {
		t.Errorf("calendar width = %v, want its intrinsic seven-cell width", cal.W)
	}

	// Choosing a day in the calendar moves the right column to it; its
	// diary opens from the button beside the calendar.
	today := now.Format("2006-01-02")
	clickText(t, tt, fmt.Sprintf("%d年%d月%d日", now.Year(), int(now.Month()), now.Day()))
	if a.feed.date.Format("2006-01-02") != today {
		t.Fatalf("chosen day = %q, want %q", a.feed.date.Format("2006-01-02"), today)
	}
	clickText(t, tt, "写这一天的日记")
	if a.feed.day.date != today {
		t.Fatalf("open day = %q, want %q", a.feed.day.date, today)
	}
	if !tt.HasText("当天记录") || !tt.HasText("备注") || !tt.HasText("照片") {
		t.Errorf("journal editor missing: %q", tt.Texts())
	}
	if a.feed.day.note != "今天试了少放盐。" {
		t.Errorf("journal note = %q", a.feed.day.note)
	}
}

// The stats, divider and buttons share the calendar's centre line instead of
// starting at the split pane's left edge.
func TestFeedSummaryCenteredWithCalendar(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	clickText(t, tt, "食历")

	cal, ok := tt.Find("开饭月历")
	if !ok {
		t.Fatal("calendar label missing")
	}
	summary, ok := tt.Find("食历摘要")
	if !ok {
		t.Fatalf("summary block label missing: %q", tt.Texts())
	}
	calCentre := cal.X + cal.W/2
	summaryCentre := summary.X + summary.W/2
	if d := calCentre - summaryCentre; d > 1.5 || d < -1.5 {
		t.Errorf("summary centre x=%v, calendar centre x=%v (widths %v/%v)",
			summaryCentre, calCentre, summary.W, cal.W)
	}
}

// The calendar's own arrows move the month without touching the data.
func TestFeedMonthNavigation(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	clickText(t, tt, "食历")
	before := a.feed.date
	clickText(t, tt, "上个月")
	if !a.feed.date.Before(before) {
		t.Errorf("after 上个月: %v, want before %v", a.feed.date, before)
	}
	if a.feed.month != int(a.feed.date.Month()) {
		t.Errorf("month summary = %d, date month = %d", a.feed.month, a.feed.date.Month())
	}
}

// The annual review reads totals, the top dishes, the monthly chart and the
// cuisine split.
func TestFeedAnnualReview(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	clickText(t, tt, "食历")
	clickText(t, tt, "年度回顾")

	if !tt.HasText("总菜数") || !tt.HasText("开饭天数") || !tt.HasText("最长连续") {
		t.Errorf("review stats missing: %q", tt.Texts())
	}
	if !tt.HasText("月度开饭") || !tt.HasText("菜系分布") {
		t.Errorf("review sections missing: %q", tt.Texts())
	}
	if !tt.HasText("青椒肉丝") {
		t.Errorf("top dishes missing: %q", tt.Texts())
	}
	if a.feed.reviewStats.Dishes != 5 {
		t.Errorf("review dishes = %d, want 5", a.feed.reviewStats.Dishes)
	}
}

// 我的 shows the identity, the level and both entry groups.
func TestProfileOverview(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "我的")

	if !tt.HasText("小满") {
		t.Errorf("nickname missing: %q", tt.Texts())
	}
	if !tt.HasText("Lv.3") || tt.HasText("XP") {
		t.Errorf("level missing or XP still shown: %q", tt.Texts())
	}
	if !tt.HasText("我的菜谱") || !tt.HasText("厨房") {
		t.Errorf("groups missing: %q", tt.Texts())
	}
	if !tt.HasText("最近下厨") || !tt.HasText("青椒肉丝") {
		t.Errorf("recent cooking missing: %q", tt.Texts())
	}

	clickText(t, tt, "打开做过的菜")
	if !tt.HasText("全部时间") || !tt.HasText("排序") {
		t.Errorf("cooked page missing: %q", tt.Texts())
	}
	if !tt.HasText("条记录") {
		t.Errorf("cooked totals missing: %q", tt.Texts())
	}
}

// The cooked page filters and switches between the list and the totals.
func TestCookedFiltersAndStats(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "我的")
	clickText(t, tt, "打开做过的菜")

	if !tt.HasText("清蒸鲈鱼") || !tt.HasText("红烧肉") {
		t.Fatalf("cooked rows missing: %q", tt.Texts())
	}
	clickText(t, tt, "近 90 天")
	if !tt.HasText("青椒肉丝") {
		t.Errorf("cooked rows after filter missing: %q", tt.Texts())
	}
	clickText(t, tt, "统计")
	if !tt.HasText("月份分布") || !tt.HasText("做得最多") || !tt.HasText("做过菜式") {
		t.Errorf("stats view missing: %q", tt.Texts())
	}
}

// 收藏 lists the stored favourites and removing one writes the database.
func TestShelfListsAndPersists(t *testing.T) {
	store := openM6Store(t)
	a := m6Fixture(t)
	a.store = store
	seed, err := store.GetProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(context.Background(), domain.Profile{
		ID: 1, Nickname: seed.Nickname, FavoriteRecipes: []int64{7},
	}); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "我的")
	clickText(t, tt, "打开收藏")
	if !tt.HasText("清蒸鲈鱼") {
		t.Fatalf("favourite row missing: %q", tt.Texts())
	}
	clickText(t, tt, "去掉")
	waitFor(t, "favourite removed from the database", func() bool {
		p, err := store.GetProfile(context.Background())
		return err == nil && len(p.FavoriteRecipes) == 0
	})
	if a.favorites[7] {
		t.Error("favourites map still holds 7")
	}
}

// Setting the motion level from 设置 survives a restart: a fresh app that
// loads the preferences ends up at the same level.
func TestSettingsMotionSurvivesRestart(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	a := New()
	a.store = store
	a.phase = PhaseReady
	if err := a.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "设置")
	if !tt.HasText("外观与动效") || !tt.HasText("数据与备份") || !tt.HasText("关于") {
		t.Errorf("settings sections missing: %q", tt.Texts())
	}
	clickText(t, tt, "减弱")
	if a.motion != "lite" {
		t.Fatalf("motion = %q, want lite", a.motion)
	}
	clickText(t, tt, "深色")
	if a.theme != "dark" {
		t.Fatalf("theme = %q, want dark", a.theme)
	}
	waitFor(t, "preferences written", func() bool {
		raw, err := store.MetaGet(ctx, "prefs")
		return err == nil && strings.Contains(raw, `"lite"`) && strings.Contains(raw, `"dark"`)
	})

	b := New()
	b.phase = PhaseReady
	if err := b.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if b.motion != "lite" || b.motionIndex != 1 {
		t.Errorf("restarted motion = %q (index %d), want lite (1)", b.motion, b.motionIndex)
	}
	if b.theme != "dark" || b.themeIndex != 2 {
		t.Errorf("restarted theme = %q (index %d), want dark (2)", b.theme, b.themeIndex)
	}
}

// The theme picker is one drop-down that names the wash. It replaced a grid
// of ten painted previews, so what has to hold is that the stored value is
// still the theme id and not the Chinese label the user read.
func TestSettingsThemePickerStoresTheId(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	a := New()
	a.store = store
	a.phase = PhaseReady
	if err := a.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "设置")

	// The trigger wears the stored theme's name, not a swatch.
	shown := design.ThemeLabel(design.DefaultTheme)
	if !tt.HasText(shown) {
		t.Fatalf("the picker does not show %q: %q", shown, tt.Texts())
	}
	clickText(t, tt, shown)
	if !tt.HasText("黄昏") {
		t.Fatalf("the picker does not list the themes: %q", tt.Texts())
	}
	clickText(t, tt, "黄昏")
	if a.themeName != "dusk" {
		t.Fatalf("themeName = %q, want dusk", a.themeName)
	}
	waitFor(t, "theme written", func() bool {
		raw, err := store.MetaGet(ctx, "prefs")
		return err == nil && strings.Contains(raw, `"dusk"`)
	})

	b := New()
	b.phase = PhaseReady
	if err := b.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if b.themeName != "dusk" {
		t.Errorf("restarted themeName = %q, want dusk", b.themeName)
	}
}

// The settings page is a stack of static groups now, not a set of
// disclosures, and the AI section names the thing it configures instead of
// repeating its own heading. Rows that leave the page are links.
func TestSettingsRedesignSectionsAndLinks(t *testing.T) {
	a := New()
	a.phase = PhaseReady
	tt := ui.NewTester(a.View, 1280, 1600)
	clickText(t, tt, "设置")
	for _, want := range []string{
		"外观与动效", "厨房档案", "AI 与配额", "数据与备份", "帮助", "关于", "视频播放",
	} {
		if !tt.HasText(want) {
			t.Errorf("settings group %q is missing: %q", want, tt.Texts())
		}
	}
	if !tt.HasText("接口密钥与模型") {
		t.Fatalf("the AI row is missing: %q", tt.Texts())
	}

	if err := tt.Click("接口密钥与模型"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if got := a.router.Path(); got != "/settings/ai" {
		t.Fatalf("the AI row opened %q, want /settings/ai", got)
	}

	a.router.Back()
	settle(tt)
	if err := tt.Click("身体档案"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if got := a.router.Path(); got != "/sichu/body" {
		t.Fatalf("the body row opened %q, want /sichu/body", got)
	}
}

// The first-run questionnaire appears while the profile is not onboarded and
// does not come back after it is finished or skipped.
func TestOnboardingAppearsOnce(t *testing.T) {
	store := openM6Store(t)
	ctx := context.Background()

	a := New()
	a.phase = PhaseReady
	a.store = store
	if err := a.loadProfileState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if !a.onboardingRequired() {
		t.Fatal("questionnaire not due on a fresh database")
	}
	tt := ui.NewTester(a.View, 1280, 860)
	if !tt.HasText("开始使用食匣") {
		t.Fatalf("questionnaire missing: %q", tt.Texts())
	}
	tt.Scroll(640, 420, 0, 800)
	settle(tt)
	clickText(t, tt, "跳过")
	if a.onboardingRequired() {
		t.Error("questionnaire still due after skipping")
	}
	if !tt.HasText("仓库") {
		t.Errorf("shell missing after skipping: %q", tt.Texts())
	}
	waitFor(t, "onboarding recorded", func() bool {
		up, err := store.GetUserProfile(ctx)
		return err == nil && up.Onboarded
	})

	b := New()
	b.phase = PhaseReady
	b.store = store
	if err := b.loadProfileState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if b.onboardingRequired() {
		t.Error("questionnaire came back after a restart")
	}
}

// The questionnaire saves the answers it was given.
func TestOnboardingSavesAnswers(t *testing.T) {
	store := openM6Store(t)
	ctx := context.Background()

	a := New()
	a.phase = PhaseReady
	a.store = store
	a.prefs.onboarding = onboardingFrom(domain.UserProfile{})
	a.prefs.onboarding.loaded = true
	a.prefs.onboarding.required = true
	a.prefs.onboarding.stove = 2
	a.prefs.onboarding.cookware["wok"] = true
	a.prefs.onboarding.spicy = 3
	a.prefs.onboarding.allergy = "花生"
	a.prefs.onboarding.adults = 2
	a.prefs.onboarding.kids = 1

	tt := ui.NewTester(a.View, 1280, 860)
	if !tt.HasText("开始使用食匣") {
		t.Fatalf("questionnaire missing: %q", tt.Texts())
	}
	// Walk the four stops: the finish action lives on the last one.
	clickText(t, tt, "下一步")
	clickText(t, tt, "下一步")
	clickText(t, tt, "下一步")
	clickText(t, tt, "完成并进入")
	waitFor(t, "answers written", func() bool {
		up, err := store.GetUserProfile(ctx)
		return err == nil && up.Onboarded && up.StoveCount == 3
	})
	up, err := store.GetUserProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if up.Spicy != "hot" || up.AllergyNote != "花生" || up.HouseholdSize != 3 {
		t.Errorf("saved profile = %+v", up)
	}
	if !strings.Contains(up.Cookware, `"wok":1`) {
		t.Errorf("cookware = %q", up.Cookware)
	}
}

// Help groups expand on a click.
func TestHelpAccordion(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "帮助")
	if !tt.HasText("本地使用说明") {
		t.Fatalf("help page missing: %q", tt.Texts())
	}
	if !tt.HasText("数据与备份") || !tt.HasText("快捷键") {
		t.Errorf("help groups missing: %q", tt.Texts())
	}
	clickText(t, tt, "快捷键")
	if !tt.HasText("Tab / Shift+Tab") {
		t.Errorf("shortcut section did not open: %q", tt.Texts())
	}
}

// 编辑资料 writes the nickname to the account row.
func TestProfileEditSavesNickname(t *testing.T) {
	store := openM6Store(t)
	a := m6Fixture(t)
	a.store = store
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "我的")
	clickText(t, tt, "编辑资料")
	if !tt.HasText("保存资料") {
		t.Fatalf("edit page missing: %q", tt.Texts())
	}
	// The form starts from the stored nickname, already focused; append to it.
	tt.Type("阿满")
	clickText(t, tt, "保存资料")
	waitFor(t, "profile written", func() bool {
		p, err := store.GetProfile(context.Background())
		return err == nil && p.Nickname == "小满阿满"
	})
}

// Saving a day's journal writes it, and a second save keeps one row per day.
func TestJournalSavePersists(t *testing.T) {
	store := openM6Store(t)
	a := m6Fixture(t)
	a.store = store
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	clickText(t, tt, "食历")
	now := time.Now().In(time.Local)
	date := now.Format("2006-01-02")
	clickText(t, tt, fmt.Sprintf("%d年%d月%d日", now.Year(), int(now.Month()), now.Day()))
	clickText(t, tt, "写这一天的日记")

	a.feed.day.note = "少放了一点盐"
	a.feed.day.photos = []string{"/tmp/今天.jpg"}
	tt.Frame() // the photo row changes the layout; redraw before clicking below it
	clickText(t, tt, "保存日记")
	waitFor(t, "journal written", func() bool {
		j, ok, err := store.GetJournal(context.Background(), date)
		return err == nil && ok && j.Note == "少放了一点盐"
	})

	// Editing the same day updates the row rather than adding a second one.
	a.feed.day.note = "第二天再试"
	tt.Frame()
	clickText(t, tt, "保存日记")
	waitFor(t, "journal updated", func() bool {
		j, ok, err := store.GetJournal(context.Background(), date)
		return err == nil && ok && j.Note == "第二天再试"
	})
	all, err := store.ListJournals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("journals = %d rows, want 1 per day", len(all))
	}
	if len(all[0].PhotoPaths) != 1 || all[0].PhotoPaths[0] != "/tmp/今天.jpg" {
		t.Errorf("journal photos = %v", all[0].PhotoPaths)
	}
}

// 一键清空 removes the cooked history and leaves the favourites alone.
func TestClearCookedRecords(t *testing.T) {
	ctx := context.Background()
	store := openM6Store(t)
	a := m6Fixture(t)
	a.store = store
	if err := store.SettleRun(ctx, []int64{1, 2}, time.Now(), nil, ""); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListRecords(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	count, _ := store.RecordCount(ctx)
	days, _ := store.MealDayCount(ctx)
	a.setRecords(records, count, days)

	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "我的")
	clickText(t, tt, "打开做过的菜")
	clickText(t, tt, "清空记录")
	clickText(t, tt, "清空")
	waitFor(t, "cooked history cleared", func() bool {
		n, err := store.RecordCount(ctx)
		return err == nil && n == 0
	})
	settle(tt)
	if a.recordCount != 0 {
		t.Errorf("record count = %d after clearing", a.recordCount)
	}
	if !tt.HasText("这个筛选下没有做过的菜") {
		t.Errorf("cooked empty state missing: %q", tt.Texts())
	}
}

// The chosen Kero theme survives a restart, and old preference blobs
// without a kero_theme field fall back to paper.
func TestSettingsThemeSurvivesRestart(t *testing.T) {
	store := openM6Store(t)
	ctx := context.Background()

	a := New()
	a.store = store
	a.phase = PhaseReady
	if err := a.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "设置")
	if !tt.HasText("主题") {
		t.Fatalf("theme picker missing: %q", tt.Texts())
	}
	// The picker is a drop-down now: it shows the stored wash's name and
	// lists the ten only once opened.
	clickText(t, tt, design.ThemeLabel(a.themeName))
	clickText(t, tt, "沙丘")
	if a.themeName != "dune" {
		t.Fatalf("themeName = %q, want dune", a.themeName)
	}
	waitFor(t, "theme written", func() bool {
		raw, err := store.MetaGet(ctx, "prefs")
		return err == nil && strings.Contains(raw, `"kero_theme":"dune"`)
	})

	b := New()
	b.phase = PhaseReady
	if err := b.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if b.themeName != "dune" {
		t.Errorf("restarted themeName = %q, want dune", b.themeName)
	}

	// An old blob — no kero_theme key — reads back as paper.
	if err := store.MetaSet(ctx, "prefs", `{"motion":"full","theme":"auto"}`); err != nil {
		t.Fatal(err)
	}
	c := New()
	c.phase = PhaseReady
	if err := c.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if c.themeName != design.DefaultTheme {
		t.Errorf("legacy themeName = %q, want %q", c.themeName, design.DefaultTheme)
	}

	// A garbage name reads back as paper too.
	if err := store.MetaSet(ctx, "prefs", `{"motion":"full","theme":"auto","kero_theme":"solarized"}`); err != nil {
		t.Fatal(err)
	}
	d := New()
	d.phase = PhaseReady
	if err := d.loadPrefsState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if d.themeName != design.DefaultTheme {
		t.Errorf("invalid themeName = %q, want %q", d.themeName, design.DefaultTheme)
	}
}

// No records: the page is one panel with one action, not a calendar of
// empty months plus zeroed cards.
func TestFeedEmptySinglePanel(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	clickText(t, tt, "食历")
	if !tt.HasText("还没有开饭记录") {
		t.Fatalf("empty panel missing: %q", tt.Texts())
	}
	if tt.HasText("本月开饭") || tt.HasText("最近开饭") {
		t.Fatalf("empty state stacked extra blocks: %q", tt.Texts())
	}
	if err := tt.Click("去掌序做一道菜"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.router.Path() != "/schedule" {
		t.Fatalf("path = %q", a.router.Path())
	}
}

// Choosing another day rebuilds the right column from that day.
func TestFeedSplitDayChange(t *testing.T) {
	a := m6Fixture(t)
	tt := ui.NewTester(a.View, 1280, 860)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	clickText(t, tt, "食历")
	yesterday := time.Now().In(time.Local).AddDate(0, 0, -1)
	clickText(t, tt, fmt.Sprintf("%d年%d月%d日", yesterday.Year(), int(yesterday.Month()), yesterday.Day()))
	if a.feed.date.Format("2006-01-02") != yesterday.Format("2006-01-02") {
		t.Fatalf("chosen day = %q", a.feed.date.Format("2006-01-02"))
	}
	if !tt.HasText("18:30") {
		t.Fatalf("the day's own meal is missing: %q", tt.Texts())
	}
	if !hasTextContaining(tt, fmt.Sprintf("%d 年 %d 月 %d 日", yesterday.Year(), int(yesterday.Month()), yesterday.Day())) {
		t.Fatalf("day heading did not change: %q", tt.Texts())
	}
}
