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
)

// m4TestApp is a ready app with three queued dishes whose steps use boards,
// stoves, hands and one passive wait: the sample the timeline golden test
// uses, so the view tests read the same numbers.
func m4TestApp() *App {
	a := New()
	a.phase = PhaseReady
	minutes := func(v float64) *float64 { return &v }
	a.recipes = []domain.Recipe{
		{ID: 1, Name: "青椒肉丝", DurationMinutes: minutes(20), BaseServings: 2,
			MainCategory: "猪肉", Region: "川菜",
			Ingredients: []domain.Ingredient{
				{Name: "猪肉", Amount: 300, Unit: "g", Required: true},
				{Name: "番茄", Amount: 2, Unit: "个", Required: true},
				{Name: "盐", Amount: 2, Unit: "g", Required: true, Seasoning: true},
			}},
		{ID: 2, Name: "番茄炒蛋", DurationMinutes: minutes(15), BaseServings: 2,
			MainCategory: "蛋类", Region: "家常",
			Ingredients: []domain.Ingredient{
				{Name: "番茄", Amount: 1, Unit: "个", Required: true},
				{Name: "鸡蛋", Amount: 3, Unit: "个", Required: true},
			}},
		{ID: 3, Name: "红烧肉", DurationMinutes: minutes(60), BaseServings: 2,
			MainCategory: "猪肉", Region: "本帮菜",
			Ingredients: []domain.Ingredient{
				{Name: "土豆", Amount: 1, Unit: "个", Required: true},
				{Name: "香菇", Amount: 5, Unit: "朵", Required: true},
			}},
	}
	a.recipeByID = make(map[int64]domain.Recipe, len(a.recipes))
	for _, rec := range a.recipes {
		a.recipeByID[rec.ID] = rec
	}
	a.steps = map[int64][]domain.Step{
		1: {
			{RecipeID: 1, OrderIndex: 1, Instruction: "青椒切丝", DurationMinutes: minutes(2), Resources: []string{"board"}},
			{RecipeID: 1, OrderIndex: 2, Instruction: "滑炒肉丝", DurationMinutes: minutes(5), Resources: []string{"stove"}},
			{RecipeID: 1, OrderIndex: 3, Instruction: "装盘", DurationMinutes: minutes(1)},
		},
		2: {
			{RecipeID: 2, OrderIndex: 1, Instruction: "打蛋", DurationMinutes: minutes(2)},
			{RecipeID: 2, OrderIndex: 2, Instruction: "炒蛋", DurationMinutes: minutes(3), Resources: []string{"stove"}},
		},
		3: {
			{RecipeID: 3, OrderIndex: 1, Instruction: "肉切块", DurationMinutes: minutes(3), Resources: []string{"board"}},
			{RecipeID: 3, OrderIndex: 2, Instruction: "小火焖", DurationMinutes: minutes(10), Resources: []string{"waiting"}},
			{RecipeID: 3, OrderIndex: 3, Instruction: "收汁", DurationMinutes: minutes(1)},
		},
	}
	a.setItems(fixtureItems())
	a.recomputeRecipeView()
	now := time.Now()
	a.setSchedule([]domain.ScheduleItem{
		{RecipeID: 1, AddedAt: now, Servings: 2, Status: domain.SchedulePending},
		{RecipeID: 2, AddedAt: now.Add(time.Second), Servings: 2, Status: domain.SchedulePending},
		{RecipeID: 3, AddedAt: now.Add(2 * time.Second), Servings: 2, Status: domain.SchedulePending},
	})
	return a
}

func openSchedule(tt *ui.Tester) error {
	if err := tt.Click("掌序"); err != nil {
		return err
	}
	settle(tt)
	return nil
}

// TestScheduleQueueAndPrep walks the queue into the prep list: the aggregation
// merges by alias, marks what is short, and the gap button feeds the shopping
// list.
func TestScheduleQueueAndPrep(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("3 道菜 · 预计 95 分钟") {
		t.Fatalf("queue totals missing: %q", tt.Texts())
	}
	if !tt.HasText("14 分钟") || !tt.HasText("并行后总时长") {
		t.Errorf("timeline stats missing from the queue page: %q", tt.Texts())
	}
	if a.sched.timeline.TotalMinutes != 14 || a.sched.timeline.SerialMinutes != 27 {
		t.Errorf("timeline = %+v", a.sched.timeline)
	}

	if err := tt.Click("备料清单"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("总备料清单") {
		t.Fatalf("prep page missing: %q", tt.Texts())
	}
	if !tt.HasText("这桌 6 样食材 · 在库 4 样 · 缺口 1 样") {
		t.Errorf("prep summary = %q", tt.Texts())
	}
	// 番茄 aggregates 2 + 1 across two dishes; 猪肉 is not in the pantry.
	if !tt.HasText("3 个") || !tt.HasText("生鲜 · 5 样") || !tt.HasText("调料 · 1 样") {
		t.Errorf("prep groups missing: %q", tt.Texts())
	}
	if err := tt.Click("缺口加入购物清单"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !strings.Contains(a.notice, "1 样缺口") {
		t.Errorf("notice = %q", a.notice)
	}
	found := false
	for _, line := range a.shopping {
		if line.Name == "猪肉" {
			found = true
		}
	}
	if !found {
		t.Errorf("pork was not added to the shopping list: %+v", a.shopping)
	}
}

// TestTimelineStoveReplan checks the stove control re-plans live: one burner
// serialises the stove steps into a single lane.
func TestTimelineStoveReplan(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("排程时间线"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if got := countLanes(a.sched.timeline, domain.ResourceStove); got != 2 {
		t.Fatalf("stove lanes = %d, want 2", got)
	}
	if err := tt.Click("1 灶"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sched.stoveCount != 1 {
		t.Errorf("stove count = %d, want 1", a.sched.stoveCount)
	}
	if got := countLanes(a.sched.timeline, domain.ResourceStove); got != 1 {
		t.Errorf("stove lanes after one burner = %d, want 1", got)
	}
}

// TestRunKeyboardPauseAndHang drives the run page: arrows change the step, the
// pause freezes the clock, and a passive wait gets a countdown in the tray.
func TestRunKeyboardPauseAndHang(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sched.runActive || len(a.sched.runLines) != 8 {
		t.Fatalf("run = %+v lines %d", a.sched.runActive, len(a.sched.runLines))
	}
	if !tt.HasText("第 1/3 步") {
		t.Fatalf("run board missing the current dish's step: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyRight)
	settle(tt)
	if a.sched.runStep != 1 {
		t.Fatalf("right arrow: step = %d, want 1", a.sched.runStep)
	}
	tt.Key(0, ui.KeyLeft)
	settle(tt)
	if a.sched.runStep != 0 {
		t.Errorf("left arrow: step = %d, want 0", a.sched.runStep)
	}

	// Pause freezes the run clock.
	if err := tt.Click("暂停"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sched.runPaused {
		t.Fatal("run did not pause")
	}
	paused := a.sched.elapsed(time.Now().Unix())
	tt.Advance(3 * time.Second)
	settle(tt)
	if got := a.sched.elapsed(time.Now().Unix()); got != paused {
		t.Errorf("elapsed while paused = %d, want %d", got, paused)
	}
	if err := tt.Click("继续"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sched.runPaused {
		t.Error("run did not resume")
	}

	// The passive-wait step can be hung in the tray, wherever the timeline
	// placed it.
	wait := waitingLineIndex(a)
	if wait < 0 {
		t.Fatal("the fixture has no waiting step")
	}
	a.runStepTo(wait)
	tt.Frame()
	settle(tt)
	if !tt.HasText("挂起") {
		t.Fatalf("waiting step offers no countdown: %q", tt.Texts())
	}
	if err := tt.Click("挂起"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.sched.hang) != 1 {
		t.Fatalf("hang tray = %+v", a.sched.hang)
	}
	if !tt.HasText("挂着 1 步计时") {
		t.Errorf("tray missing: %q", tt.Texts())
	}
	if err := tt.Click("取下"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.sched.hang) != 0 {
		t.Errorf("hang tray not cleared: %+v", a.sched.hang)
	}

	// Leaving the run drops the progress but keeps the dishes queued.
	if err := tt.Click("退出这桌"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sched.runActive {
		t.Error("run still active after leaving it")
	}
	if len(a.schedule) != 3 {
		t.Errorf("queue after abandon = %+v", a.schedule)
	}
	if !tt.HasText("待做队列") {
		t.Errorf("abandon did not return to the queue: %q", tt.Texts())
	}
}

// TestSettleSheetPreview opens the confirmation sheet from the run page and
// checks the deduction preview and the cleared queue after confirming.
func TestSettleSheetPreview(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("结算并扣库存"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sched.confirmOpen {
		t.Fatal("confirmation sheet did not open")
	}
	if !tt.HasText("番茄 4 个 → 1 个") {
		t.Errorf("precise deduction missing from the sheet: %q", tt.Texts())
	}
	if !tt.HasText("土豆 还有一些 → 快没了") {
		t.Errorf("coarse deduction missing from the sheet: %q", tt.Texts())
	}
	if !tt.HasText("+30 XP") {
		t.Errorf("xp line missing: %q", tt.Texts())
	}
	if err := tt.Click("确认结算"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if len(a.schedule) != 0 {
		t.Errorf("queue after settle = %+v", a.schedule)
	}
	if a.recordCount != 3 || len(a.records) != 3 {
		t.Errorf("records = %d (count %d), want 3", len(a.records), a.recordCount)
	}
	if !tt.HasText("掌序里还没有菜") {
		t.Errorf("empty state missing after settle: %q", tt.Texts())
	}
}

// TestRunSessionRestored proves the run survives a restart: the session read
// from app_meta reopens on the same step with the paused clock intact.
func TestRunSessionRestored(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	session := domain.RunSession{
		RecipeIDs: []int64{1, 2, 3},
		Step:      4,
		StartedAt: time.Now().Add(-10 * time.Minute).Unix(),
		Paused:    true,
		PausedFor: 60,
		UpdatedAt: time.Now().Add(-2 * time.Minute).Unix(),
	}
	if err := store.SaveRunSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	a := m4TestApp()
	a.store = store
	if err := a.loadScheduleState(ctx, store); err != nil {
		t.Fatal(err)
	}
	if !a.sched.runActive || a.sched.runStep != 4 || !a.sched.runPaused {
		t.Fatalf("restored state = active %v step %d paused %v",
			a.sched.runActive, a.sched.runStep, a.sched.runPaused)
	}
	if len(a.sched.runLines) != 8 {
		t.Fatalf("restored lines = %d, want 8", len(a.sched.runLines))
	}
	// A paused session's clock stays where it was saved.
	frozen := a.sched.elapsed(time.Now().Unix())
	tt := ui.NewTester(a.View, 1280, 860)
	tt.Advance(2 * time.Second)
	settle(tt)
	if got := a.sched.elapsed(time.Now().Unix()); got != frozen {
		t.Errorf("restored paused clock moved: %d -> %d", frozen, got)
	}
	a.router.Push("/schedule/run")
	settle(tt)
	if !tt.HasText("番茄炒蛋 · 第 2 步") {
		t.Errorf("run page did not restore the step: %q", tt.Texts())
	}
}

// A restored session keeps the meal it belongs to, so the board and the next
// settlement stay scoped to that meal.
func TestRunSessionMealRestored(t *testing.T) {
	a := m4TestApp()
	a.restoreRunSession(domain.RunSession{
		RecipeIDs: []int64{1, 2}, Meal: "午餐", StartedAt: time.Now().Unix(),
	})
	if !a.sched.runActive || a.sched.runMeal != "午餐" {
		t.Fatalf("restored run = active %v meal %q", a.sched.runActive, a.sched.runMeal)
	}
	for _, line := range a.sched.runLines {
		if line.RecipeID == 3 {
			t.Fatalf("restored lunch run contains dinner: %+v", a.sched.runLines)
		}
	}
}

// TestRunSessionPersistedOnStep checks a step change writes the session.
func TestRunSessionPersistedOnStep(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "persist.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	a := m4TestApp()
	a.store = store
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	tt.Key(0, ui.KeyRight)
	settle(tt)

	deadline := time.Now().Add(2 * time.Second)
	for {
		session, ok, err := store.LoadRunSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if ok && session.Step == 1 {
			if len(session.RecipeIDs) != 3 {
				t.Errorf("session recipes = %v", session.RecipeIDs)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session not persisted: ok=%v session=%+v", ok, session)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRemoveLastDishEndsRun: emptying the queue mid-run drops the run and its
// saved progress instead of leaving the run page on a stale step list.
func TestRemoveLastDishEndsRun(t *testing.T) {
	a := m4TestApp()
	a.startRun()
	if !a.sched.runActive {
		t.Fatal("run did not start")
	}
	for _, id := range []int64{1, 2, 3} {
		a.removeSchedule(id)
	}
	if len(a.schedule) != 0 {
		t.Fatalf("queue = %+v", a.schedule)
	}
	if a.sched.runActive {
		t.Error("run still active after the last dish was removed")
	}
	if len(a.sched.runLines) != 0 {
		t.Errorf("run lines = %d, want 0", len(a.sched.runLines))
	}
}

func countLanes(tl domain.Timeline, resource string) int {
	n := 0
	for _, lane := range tl.Lanes {
		if lane.Resource == resource {
			n++
		}
	}
	return n
}

// A multi-meal queue is shown as meal groups, each with its own start and
// settle actions.
func TestQueueGroupsByMeal(t *testing.T) {
	a := m4TestApp()
	now := time.Now()
	a.setSchedule([]domain.ScheduleItem{
		{RecipeID: 1, AddedAt: now, Servings: 2, Status: domain.SchedulePending, Meal: "午餐"},
		{RecipeID: 2, AddedAt: now.Add(time.Second), Servings: 2, Status: domain.SchedulePending, Meal: "午餐"},
		{RecipeID: 3, AddedAt: now.Add(2 * time.Second), Servings: 2, Status: domain.SchedulePending, Meal: "晚餐"},
	})
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"午餐", "晚餐", "开始午餐", "结算午餐", "开始晚餐", "结算晚餐"} {
		if !tt.HasText(want) {
			t.Errorf("queue page is missing %q: %q", want, tt.Texts())
		}
	}
}

// Starting one meal cooks only that meal, and settling it leaves the other
// meal in the queue.
func TestStartAndSettleOneMeal(t *testing.T) {
	a := m4TestApp()
	now := time.Now()
	a.setSchedule([]domain.ScheduleItem{
		{RecipeID: 1, AddedAt: now, Servings: 2, Status: domain.SchedulePending, Meal: "午餐"},
		{RecipeID: 2, AddedAt: now.Add(time.Second), Servings: 2, Status: domain.SchedulePending, Meal: "午餐"},
		{RecipeID: 3, AddedAt: now.Add(2 * time.Second), Servings: 2, Status: domain.SchedulePending, Meal: "晚餐"},
	})
	a.startRunMeal("午餐")
	if !a.sched.runActive || a.sched.runMeal != "午餐" {
		t.Fatalf("run = %+v", a.sched.runMeal)
	}
	if len(a.sched.runRecipes) != 2 || a.sched.runRecipes[0] != 1 || a.sched.runRecipes[1] != 2 {
		t.Fatalf("lunch recipes = %+v", a.sched.runRecipes)
	}
	for _, line := range a.sched.runLines {
		if line.RecipeID == 3 {
			t.Fatalf("dinner leaked into the lunch run: %+v", a.sched.runLines)
		}
	}
	a.openSettleConfirmFor("午餐")
	a.settle()
	if len(a.schedule) != 1 || a.schedule[0].Meal != "晚餐" || a.schedule[0].RecipeID != 3 {
		t.Fatalf("queue after settling lunch = %+v", a.schedule)
	}
	if a.recordCount != 2 {
		t.Errorf("records = %d, want the two lunch dishes", a.recordCount)
	}
	if a.sched.runActive {
		t.Error("the run stayed active after settlement")
	}
}

// TestAddRecipesToSchedule covers the cross-page contract the private kitchen
// calls: a batch add queues each dish once and keeps queue order.
func TestAddRecipesToSchedule(t *testing.T) {
	a := newTestApp()
	a.addRecipesToSchedule([]domain.ScheduleItem{
		{RecipeID: 2, Meal: "午餐"}, {RecipeID: 3, Meal: "晚餐"}, {RecipeID: 3, Meal: "晚餐"},
	})
	if len(a.schedule) != 2 {
		t.Fatalf("queue = %+v, want 2 dishes", a.schedule)
	}
	if a.schedule[0].RecipeID != 2 || a.schedule[1].RecipeID != 3 {
		t.Errorf("queue order = %+v, want 2 then 3", a.schedule)
	}
	if a.schedule[0].Meal != "午餐" || a.schedule[1].Meal != "晚餐" {
		t.Errorf("meal groups = %+v", a.schedule)
	}
	a.addRecipesToSchedule([]domain.ScheduleItem{{RecipeID: 2}, {RecipeID: 3}})
	if len(a.schedule) != 2 {
		t.Errorf("duplicate add grew the queue: %+v", a.schedule)
	}
	if a.notice != "这些菜已经在掌序里" {
		t.Errorf("notice = %q", a.notice)
	}
	if a.sched.timeline.SerialMinutes == 0 {
		t.Error("timeline was not rebuilt after the batch add")
	}
}

// The board's dish list writes each dish's own place once, and the run as a
// whole has one progress bar rather than a second global step counter.
func TestRunProgressSingle(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	count := 0
	for _, text := range tt.Texts() {
		if strings.HasPrefix(text, "第 ") && strings.HasSuffix(text, " 步") {
			count++
		}
		if strings.Contains(text, "/ "+fmt.Sprint(len(a.sched.runLines))) {
			t.Fatalf("a second global step counter is on screen: %q", text)
		}
	}
	if count != 1 {
		t.Fatalf("current-dish counters = %d: %q", count, tt.Texts())
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sched.runStep != 1 {
		t.Fatalf("step = %d", a.sched.runStep)
	}
	line := a.sched.runLines[a.sched.runStep]
	done, total := 0, 0
	for idx, l := range a.sched.runLines {
		if l.RecipeID == line.RecipeID {
			total++
			if idx <= a.sched.runStep {
				done++
			}
		}
	}
	if want := fmt.Sprintf("第 %d/%d 步", done, total); !tt.HasText(want) {
		t.Fatalf("board progress did not move to %q: %q", want, tt.Texts())
	}
}

// Two dishes on the table are not finished one after the other: the run
// follows the parallel timeline, so the first steps of different dishes
// come first.
func TestRunOrderInterleavesDishes(t *testing.T) {
	a := m4TestApp()
	a.startRun()
	if len(a.sched.runLines) < 3 {
		t.Fatalf("run lines = %d", len(a.sched.runLines))
	}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if a.sched.runLines[i].RecipeID == a.sched.runLines[j].RecipeID {
				t.Fatalf("lines %d and %d are the same dish: %+v",
					i, j, a.sched.runLines[:3])
			}
		}
	}
}

// The lyric spacers must leave enough room at the end for the last row to
// reach the focus point, whatever the anchor: with a symmetric spacer the
// last row never became current and its 结算 control never built.
func TestLyricSpacersReachLastRow(t *testing.T) {
	const height, anchor, n = 640.0, 0.42, 8
	lead, trail := lyricSpacers(height, anchor)
	if lead < 0 || trail < 0 {
		t.Fatalf("spacers = %v, %v", lead, trail)
	}
	if got, want := lead+n*lyricRowHeight+trail-height, (n-1)*lyricRowHeight; got != want {
		t.Fatalf("content height = %v, want %v", got, want)
	}
}

// Stepping glides the flow instead of jumping it, and the last step can be
// reached and settled from the lyric row itself.
func TestRunLastStepShowsSettle(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)

	mid := len(a.sched.runLines) / 2
	a.runStepTo(mid)
	if !a.lyric.Scrolling() {
		t.Fatal("the lyric flow jumped instead of starting a scroll")
	}
	tt.Frame() // let the scroll container start the glide
	settle(tt)
	if want := float32(mid) * lyricRowHeight; a.lyric.Y != want {
		t.Errorf("lyric Y = %v, want %v", a.lyric.Y, want)
	}

	last := len(a.sched.runLines) - 1
	a.runStepTo(last - 1)
	tt.Frame()
	settle(tt)
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.sched.runStep != last {
		t.Fatalf("step = %d, want %d", a.sched.runStep, last)
	}
	if a.lyric.MaxY <= 0 || a.lyric.Y < a.lyric.MaxY-1 {
		t.Fatalf("the last row is not at the end of the flow: Y=%v MaxY=%v",
			a.lyric.Y, a.lyric.MaxY)
	}
	if !tt.HasText("结算") {
		t.Fatalf("the last row has no settlement control: %q", tt.Texts())
	}
	if err := tt.Click("结算"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.sched.confirmOpen {
		t.Fatal("the settlement sheet did not open from the last step")
	}
}

// waitingLineIndex is the run line whose step is a passive wait, wherever the
// timeline placed it.
func waitingLineIndex(a *App) int {
	for i, line := range a.sched.runLines {
		if line.Waiting {
			return i
		}
	}
	return -1
}

// The board is a narrow, tall card centred in its rail, with the transport
// at the bottom-left and the exit at the top-left.
func TestRunBoardLayout(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)

	board, ok := tt.Find("掌序看板")
	if !ok {
		t.Fatalf("the board has no label: %q", tt.Texts())
	}
	if board.W > 320 {
		t.Errorf("board width = %v, want the narrow rail", board.W)
	}
	if board.H <= board.W {
		t.Errorf("board = %vx%v, want it taller than wide", board.W, board.H)
	}
	center := board.Y + board.H/2
	wantCenter := float32(a.winH-design.TitleBand) / 2
	if d := center - wantCenter; d < -90 || d > 90 {
		t.Errorf("board centre = %v, want near %v", center, wantCenter)
	}

	next, ok := tt.Find("下一步")
	if !ok {
		t.Fatal("the next control is missing")
	}
	if next.Y < a.winH*0.65 || next.X > design.PagePadding+design.RunBoardWidth {
		t.Errorf("next control at %v,%v, want bottom-left", next.X, next.Y)
	}
	exit, ok := tt.Find("退出这桌")
	if !ok {
		t.Fatal("the exit control is missing")
	}
	if exit.Y > 90 || exit.X > 120 {
		t.Errorf("exit control at %v,%v, want top-left", exit.X, exit.Y)
	}
}

// Hanging a wait puts its tray above the board, and the pair stays centred.
func TestHangTraySitsAboveBoard(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1024, 720)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	line := waitingLineIndex(a)
	if line < 0 {
		t.Fatal("the fixture has no waiting step")
	}
	a.runStepTo(line)
	tt.Frame()
	settle(tt)
	if err := tt.Click("挂起"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	tray, ok := tt.Find("挂着 1 步计时")
	if !ok {
		t.Fatalf("tray missing after hanging: %q", tt.Texts())
	}
	board, ok := tt.Find("掌序看板")
	if !ok {
		t.Fatal("board label missing")
	}
	if tray.Y >= board.Y {
		t.Errorf("tray Y = %v, board Y = %v; the tray must sit above the board", tray.Y, board.Y)
	}
}

// At 1024×720 the current row's controls are where they show: three
// advances all land, none hidden under an overlay.
func TestRunClockHit(t *testing.T) {
	a := m4TestApp()
	tt := ui.NewTester(a.View, 1024, 720)
	if err := openSchedule(tt); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("开始掌序"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	for want := 1; want <= 3; want++ {
		r, ok := tt.Find("完成")
		if !ok {
			t.Fatalf("step %d has no finish control: %q", want, tt.Texts())
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
		settle(tt)
		if a.sched.runStep != want {
			t.Fatalf("click %d landed on step %d", want, a.sched.runStep)
		}
	}
}
