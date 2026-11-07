package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/services/schedule"
)

// hangTimer is one passive-wait countdown hung above the lyric flow. The end
// timestamp is the fact; a paused timer keeps its remaining seconds instead.
type hangTimer struct {
	// Line is the run line the countdown belongs to.
	Line  int
	Title string
	EndAt int64
	Total int
	// Paused freezes the countdown and keeps Remaining.
	Paused    bool
	Remaining int
}

// remaining is how many seconds the countdown has left at now.
func (h hangTimer) remaining(now int64) int {
	if h.Paused {
		if h.Remaining < 0 {
			return 0
		}
		return h.Remaining
	}
	left := int(h.EndAt - now)
	if left < 0 {
		return 0
	}
	return left
}

// hangTimersFromSnapshots reads the tray back. A countdown that ran out while
// the app was closed is kept rather than dropped, so the cook comes back to
// 「时间到」 and not to a silent empty tray.
func hangTimersFromSnapshots(snapshots []domain.HangSnapshot) []hangTimer {
	if len(snapshots) == 0 {
		return nil
	}
	out := make([]hangTimer, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, hangTimer{
			Line: s.Line, Title: s.Title, EndAt: s.EndAt, Total: s.Total,
			Paused: s.Paused, Remaining: s.Remaining,
		})
	}
	return out
}

// scheduleState is the planner's view state. Everything here is derived off
// the view (after a schedule, pantry or run change) and only read while
// drawing.
type scheduleState struct {
	// prep is the aggregated shopping-style list for the queued dishes and
	// prepGroups is the same list split into its display blocks.
	prep       []domain.PrepLine
	prepGroups []domain.PrepGroup
	// timeline is the parallel plan for the queue; stepCount is how many
	// steps the run page will walk.
	timeline  domain.Timeline
	stepCount int
	// stoveCount is the kitchen's burner count; stoveChoice is the segmented
	// control's index (0 → one burner).
	stoveCount  int
	stoveChoice int

	// Run session, restored on launch and persisted on every step / pause.
	runLines    []domain.RunLine
	runRecipes  []int64
	runTimeline domain.Timeline
	runMeal     string
	runActive   bool
	runStep     int
	runStart    int64
	runPaused   bool
	pausedFor   int
	pauseStart  int64
	hang        []hangTimer
	// Settlement scope: the meal and the exact IDs the open sheet will
	// settle, so a multi-meal queue loses only what was cooked.
	settleMeal string
	settleIDs  []int64
	// dragging says a step is being carried toward the tray right now. It
	// is read from the handle each frame, because the drop band only exists
	// while something is in the air — the original shows it the same way
	// (app/lib/features/schedule/schedule_run_page.dart:1228-1235).
	dragging bool

	// confirmOpen shows the settlement sheet; preview is what it lists.
	confirmOpen bool
	preview     domain.SettlePreview
}

func (s *scheduleState) init() {
	if s.stoveCount <= 0 {
		s.stoveCount = schedule.DefaultStoveCount
	}
	s.stoveChoice = schedule.ClampStoveCount(s.stoveCount) - 1
}

// elapsed is the run's wall-clock seconds minus paused time.
func (s *scheduleState) elapsed(now int64) int {
	if s.runStart == 0 {
		return 0
	}
	if s.runPaused && s.pauseStart > 0 && s.pauseStart < now {
		now = s.pauseStart
	}
	return s.session().ElapsedSeconds(now)
}

// session is the persistable form of the run state.
func (s *scheduleState) session() domain.RunSession {
	return domain.RunSession{
		RecipeIDs: append([]int64(nil), s.runRecipes...),
		Meal:      s.runMeal,
		Step:      s.runStep,
		StartedAt: s.runStart,
		Paused:    s.runPaused,
		PausedFor: s.pausedFor,
		UpdatedAt: time.Now().Unix(),
		Hang:      s.hangSnapshots(),
	}
}

// hangSnapshots is the tray as it will be read back: same order, same
// running-or-frozen state, nothing recomputed.
func (s *scheduleState) hangSnapshots() []domain.HangSnapshot {
	if len(s.hang) == 0 {
		return nil
	}
	out := make([]domain.HangSnapshot, 0, len(s.hang))
	for _, h := range s.hang {
		out = append(out, domain.HangSnapshot{
			Line: h.Line, Title: h.Title, EndAt: h.EndAt,
			Total: h.Total, Paused: h.Paused, Remaining: h.Remaining,
		})
	}
	return out
}

// resetRun clears the run state after a settlement or an abandon.
func (s *scheduleState) resetRun() {
	s.runLines = nil
	s.runRecipes = nil
	s.runTimeline = domain.Timeline{}
	s.runMeal = ""
	s.runActive = false
	s.runStep = 0
	s.runStart = 0
	s.runPaused = false
	s.pausedFor = 0
	s.pauseStart = 0
	s.hang = nil
	s.settleMeal = ""
	s.settleIDs = nil
}

// loadScheduleState restores the persisted run session and the kitchen's stove
// count after bootstrap. It runs on bootstrap's goroutine.
func (a *App) loadScheduleState(ctx context.Context, store repository.Repository) error {
	session, ok, err := store.LoadRunSession(ctx)
	if err != nil {
		return err
	}
	profile, err := store.GetUserProfile(ctx)
	if err != nil {
		return err
	}
	a.post(func() {
		if profile.StoveCount > 0 {
			a.sched.stoveCount = schedule.ClampStoveCount(profile.StoveCount)
			a.sched.stoveChoice = a.sched.stoveCount - 1
		}
		if ok {
			a.restoreRunSession(session)
		}
	})
	return nil
}

// restoreRunSession puts a persisted session back into view state. The lyric
// lines are rebuilt once the schedule snapshot lands.
func (a *App) restoreRunSession(session domain.RunSession) {
	if session.Empty() {
		return
	}
	a.sched.runRecipes = append([]int64(nil), session.RecipeIDs...)
	a.sched.runMeal = session.Meal
	a.sched.runStep = session.Step
	a.sched.runStart = session.StartedAt
	a.sched.runPaused = session.Paused
	a.sched.pausedFor = session.PausedFor
	if session.Paused && session.UpdatedAt > 0 {
		// Freeze the clock where the session was saved: elapsed = UpdatedAt -
		// StartedAt - PausedFor, so the pause started at UpdatedAt.
		a.sched.pauseStart = session.UpdatedAt
	}
	a.sched.runActive = true
	a.sessionStarted = time.Unix(session.StartedAt, 0)
	// The tray comes back in the order it was hung, and a countdown that was
	// still running keeps the moment it ends rather than its full length:
	// closing the app must not hand the cook extra time.
	a.sched.hang = hangTimersFromSnapshots(session.Hang)
	a.focusLyric(session.Step, nil)
	// The queue snapshot may already be loaded (or land right after). Rebuild
	// the derived views so the run page has its lines either way.
	a.recomputeScheduleDerived()
}

// queuedRecipes returns the queued dishes in queue order.
func (a *App) queuedRecipes() []domain.Recipe {
	out := make([]domain.Recipe, 0, len(a.schedule))
	for _, item := range a.schedule {
		if rec, ok := a.recipeByID[item.RecipeID]; ok {
			out = append(out, rec)
		}
	}
	return out
}

// queueServings maps each queued dish to its serving count.
func (a *App) queueServings() map[int64]int {
	out := make(map[int64]int, len(a.schedule))
	for _, item := range a.schedule {
		out[item.RecipeID] = item.Servings
	}
	return out
}

// scheduleMealGroup is one meal's dishes, in queue order.
type scheduleMealGroup struct {
	Meal  string
	Items []domain.ScheduleItem
}

// scheduleMealGroups groups the queue by meal, in first-appearance order.
// Dishes added from the recipe page have no meal and form 本桌.
func (a *App) scheduleMealGroups() []scheduleMealGroup {
	index := map[string]int{}
	out := make([]scheduleMealGroup, 0, 3)
	for _, item := range a.schedule {
		i, ok := index[item.Meal]
		if !ok {
			i = len(out)
			index[item.Meal] = i
			out = append(out, scheduleMealGroup{Meal: item.Meal})
		}
		out[i].Items = append(out[i].Items, item)
	}
	return out
}

// scheduleItemsForMeal is one group's rows.
func (a *App) scheduleItemsForMeal(meal string) []domain.ScheduleItem {
	out := make([]domain.ScheduleItem, 0, len(a.schedule))
	for _, item := range a.schedule {
		if item.Meal == meal {
			out = append(out, item)
		}
	}
	return out
}

// mealGroupLabel is what the queue prints for a group.
func mealGroupLabel(meal string) string {
	if meal == "" {
		return "本桌"
	}
	return meal
}

// recipesForItems resolves a group's dishes in queue order.
func (a *App) recipesForItems(items []domain.ScheduleItem) []domain.Recipe {
	out := make([]domain.Recipe, 0, len(items))
	for _, item := range items {
		if rec, ok := a.recipeByID[item.RecipeID]; ok {
			out = append(out, rec)
		}
	}
	return out
}

// recomputeScheduleDerived rebuilds the prep list and the timeline. It runs on
// the main thread whenever the queue or the pantry changes, never while
// drawing.
func (a *App) recomputeScheduleDerived() {
	queued := a.queuedRecipes()
	a.sched.timeline = schedule.BuildTimeline(queued, a.steps, a.queueServings(), a.sched.stoveCount)
	prepItems := a.items
	if a.pantryOff {
		prepItems = nil
	}
	a.sched.prep = schedule.BuildPrep(queued, prepItems, a.matcher)
	a.sched.prepGroups = schedule.GroupPrep(a.sched.prep)
	a.sched.stepCount = len(a.sched.timeline.Steps)
	if a.sched.runActive && len(a.sched.runLines) == 0 {
		recipes := a.activeRunRecipes()
		a.sched.runTimeline = schedule.BuildTimeline(recipes, a.steps,
			a.queueServings(), a.sched.stoveCount)
		a.sched.runLines = schedule.RunLines(a.sched.runTimeline)
		if len(a.sched.runLines) == 0 {
			a.sched.runLines = schedule.RunLines(a.sched.timeline)
		}
		if a.sched.runStep >= len(a.sched.runLines) {
			a.sched.runStep = max(0, len(a.sched.runLines)-1)
		}
	}
}

// activeRunRecipes resolves the run's own dishes in queue order. A restored
// legacy session may have IDs but no meal; both are honoured.
func (a *App) activeRunRecipes() []domain.Recipe {
	ids := make(map[int64]bool, len(a.sched.runRecipes))
	for _, id := range a.sched.runRecipes {
		ids[id] = true
	}
	out := make([]domain.Recipe, 0, len(ids))
	for _, item := range a.schedule {
		if len(ids) > 0 && !ids[item.RecipeID] {
			continue
		}
		if len(ids) == 0 && a.sched.runMeal != "" && item.Meal != a.sched.runMeal {
			continue
		}
		if rec, ok := a.recipeByID[item.RecipeID]; ok {
			out = append(out, rec)
		}
	}
	if len(out) == 0 {
		for _, id := range a.sched.runRecipes {
			if rec, ok := a.recipeByID[id]; ok {
				out = append(out, rec)
			}
		}
	}
	return out
}

// buildRunLines is the queue's lyric lines in the timeline's execution
// order, so parallel dishes interleave.
func (a *App) buildRunLines() []domain.RunLine {
	return schedule.RunLines(a.sched.timeline)
}

// setStoveCount applies a kitchen burner count from the timeline page and
// recomputes the plan before the next frame.
func (a *App) setStoveCount(choice int) {
	a.sched.stoveChoice = choice
	a.sched.stoveCount = schedule.ClampStoveCount(choice + 1)
	a.recomputeScheduleDerived()
}

// startRun begins the first meal group waiting in the queue.
func (a *App) startRun() {
	groups := a.scheduleMealGroups()
	if len(groups) == 0 {
		a.notice = "掌序是空的"
		return
	}
	a.startRunMeal(groups[0].Meal)
}

// startRunMeal begins one meal's run: only its dishes enter the lyric flow,
// so breakfast, lunch and dinner are cooked one at a time.
func (a *App) startRunMeal(meal string) {
	if a.sched.runActive {
		a.notice = "先结束当前这餐"
		return
	}
	items := a.scheduleItemsForMeal(meal)
	if len(items) == 0 {
		a.notice = "这一餐没有菜"
		return
	}
	recipes := a.recipesForItems(items)
	tl := schedule.BuildTimeline(recipes, a.steps, a.queueServings(), a.sched.stoveCount)
	lines := schedule.RunLines(tl)
	if len(lines) == 0 {
		a.notice = "这桌的菜还没有步骤"
		return
	}
	a.sched.runLines = lines
	a.sched.runTimeline = tl
	a.sched.runMeal = meal
	a.sched.runRecipes = make([]int64, 0, len(items))
	for _, item := range items {
		a.sched.runRecipes = append(a.sched.runRecipes, item.RecipeID)
	}
	a.sched.runActive = true
	a.sched.runStep = 0
	a.sched.runStart = time.Now().Unix()
	a.sched.runPaused = false
	a.sched.pausedFor = 0
	a.sched.pauseStart = 0
	a.sched.hang = nil
	a.sessionStarted = time.Now()
	a.focusLyric(0, nil)
	a.persistRunSession()
}

// startOrResumeRun opens the run page, starting a run only when none is
// active: a second tap must not throw away the progress of a run in flight.
func (a *App) startOrResumeRun() {
	if !a.sched.runActive {
		a.startRun()
	}
	if !a.sched.runActive {
		return
	}
	a.router.Push("/schedule/run")
}

// runStepTo moves the run to step i and persists the new position.
func (a *App) runStepTo(i int) {
	if !a.sched.runActive {
		return
	}
	if i < 0 {
		i = 0
	}
	if last := len(a.sched.runLines) - 1; i > last {
		i = last
	}
	if i < 0 || i == a.sched.runStep {
		return
	}
	a.sched.runStep = i
	a.glideLyric(i)
	a.persistRunSession()
}

// toggleRunPause freezes or resumes the run clock and the countdowns.
func (a *App) toggleRunPause() {
	if !a.sched.runActive {
		return
	}
	now := time.Now().Unix()
	if a.sched.runPaused {
		a.sched.pausedFor += int(now - a.sched.pauseStart)
		a.sched.pauseStart = 0
		a.sched.runPaused = false
	} else {
		a.sched.pauseStart = now
		a.sched.runPaused = true
	}
	a.persistRunSession()
}

// hangCurrentStep hangs a countdown for the current step. Only passive-wait
// steps get one: the hands-on steps are the cook's own pace.
func (a *App) hangCurrentStep() {
	a.hangLine(a.sched.runStep)
}

// hangLine hangs the countdown of one step, by its index in the run. Dragging
// a step up into the tray names the step, so it goes through here rather
// than being limited to the step on screen. Hanging twice is one hanging:
// the original drops the second drag on its way in (onWillAcceptWithDetails,
// app/lib/features/schedule/schedule_run_page.dart:1274-1278).
func (a *App) hangLine(line int) {
	if !a.sched.runActive {
		return
	}
	if line < 0 || line >= len(a.sched.runLines) {
		return
	}
	for _, h := range a.sched.hang {
		if h.Line == line {
			return
		}
	}
	run := a.sched.runLines[line]
	// A waiting step is the kind that can be walked away from; the rest are
	// the cook's own pace.
	if !run.Waiting {
		a.notice = "这一步要守着做，不用挂计时"
		return
	}
	total := run.Minutes * 60
	a.sched.hang = append(a.sched.hang, hangTimer{
		Line:  line,
		Title: run.Text,
		EndAt: time.Now().Unix() + int64(total),
		Total: total,
	})
	a.persistRunSession()
}

// unhang drops one countdown from the tray; the run itself does not change.
func (a *App) unhang(line int) {
	kept := a.sched.hang[:0]
	for _, h := range a.sched.hang {
		if h.Line != line {
			kept = append(kept, h)
		}
	}
	a.sched.hang = kept
}

// toggleHangPause freezes or resumes one countdown.
func (a *App) toggleHangPause(i int) {
	if i < 0 || i >= len(a.sched.hang) {
		return
	}
	now := time.Now().Unix()
	h := a.sched.hang[i]
	if h.Paused {
		h.Paused = false
		h.EndAt = now + int64(h.Remaining)
		h.Remaining = 0
	} else {
		h.Remaining = h.remaining(now)
		h.Paused = true
	}
	a.sched.hang[i] = h
}

// currentRunLine is the step the run is on.
func (a *App) currentRunLine() (domain.RunLine, bool) {
	if a.sched.runStep < 0 || a.sched.runStep >= len(a.sched.runLines) {
		return domain.RunLine{}, false
	}
	return a.sched.runLines[a.sched.runStep], true
}

// persistRunSession writes the run position for the next launch. The write is
// small and one statement, so it runs on a goroutine and reports only errors.
func (a *App) persistRunSession() {
	store := a.store
	if store == nil {
		return
	}
	session := a.sched.session()
	go func() {
		if err := store.SaveRunSession(context.Background(), session); err != nil {
			a.post(func() { a.notice = "保存掌序进度失败：" + err.Error() })
		}
	}()
}

// abandonRun drops the run progress and returns to the queue; the dishes stay
// queued.
func (a *App) abandonRun() {
	a.sched.resetRun()
	a.sessionStarted = time.Time{}
	a.notice = "已退出这桌掌序"
	a.persistRunSession()
	a.router.Replace("/schedule")
}

// reloadRunSession re-reads the persisted run after a failed settlement, which
// cleared the progress optimistically.
func (a *App) reloadRunSession() {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		session, ok, err := store.LoadRunSession(context.Background())
		if err != nil || !ok {
			return
		}
		a.post(func() { a.restoreRunSession(session) })
	}()
}

// addRecipesToSchedule queues dishes chosen on another page (the private
// kitchen's result, for instance). It is the single batch entry point those
// pages call; the queue is written in one transaction, and each item keeps
// the meal group it came from.
func (a *App) addRecipesToSchedule(items []domain.ScheduleItem) {
	if len(items) == 0 {
		return
	}
	added := make([]domain.ScheduleItem, 0, len(items))
	for _, it := range items {
		rec, ok := a.recipeByID[it.RecipeID]
		if !ok {
			continue
		}
		if a.scheduleHas(it.RecipeID) {
			continue
		}
		it.AddedAt = time.Now()
		it.Status = domain.SchedulePending
		if it.Servings <= 0 {
			it.Servings = rec.BaseServings
		}
		a.schedule = append(a.schedule, it)
		added = append(added, it)
	}
	if len(added) == 0 {
		a.notice = "这些菜已经在掌序里"
		return
	}
	if a.scheduleSelected == 0 {
		a.scheduleSelected = added[0].RecipeID
	}
	a.computeShopping()
	store := a.store
	if store == nil {
		a.notice = fmt.Sprintf("已加入 %d 道菜", len(added))
		return
	}
	go func() {
		err := store.EnqueueRecipes(context.Background(), added)
		a.post(func() {
			if err != nil {
				a.notice = "加入掌序失败：" + err.Error()
			} else {
				a.notice = fmt.Sprintf("已加入 %d 道菜", len(added))
			}
			a.reloadSchedule()
		})
	}()
}

// scheduleHas reports whether a dish is already queued.
func (a *App) scheduleHas(id int64) bool {
	for _, item := range a.schedule {
		if item.RecipeID == id {
			return true
		}
	}
	return false
}

// openSettleConfirm settles what the run page is cooking, or the first meal
// in the queue when no run is active.
func (a *App) openSettleConfirm() {
	if a.sched.runActive {
		// Settle exactly the dishes the run walked, not everything that
		// happens to share its meal label.
		ids := make(map[int64]bool, len(a.sched.runRecipes))
		for _, id := range a.sched.runRecipes {
			ids[id] = true
		}
		items := make([]domain.ScheduleItem, 0, len(ids))
		for _, item := range a.schedule {
			if ids[item.RecipeID] {
				items = append(items, item)
			}
		}
		a.openSettleConfirmItems(a.sched.runMeal, items)
		return
	}
	groups := a.scheduleMealGroups()
	if len(groups) == 0 {
		a.notice = "掌序是空的"
		return
	}
	a.openSettleConfirmFor(groups[0].Meal)
}

// openSettleConfirmFor builds the preview for one meal group.
func (a *App) openSettleConfirmFor(meal string) {
	if a.sched.runActive && meal != a.sched.runMeal {
		a.notice = "先结束当前这餐"
		return
	}
	a.openSettleConfirmItems(meal, a.scheduleItemsForMeal(meal))
}

// openSettleConfirmItems shows the confirmation sheet for an explicit set of
// queue rows. The preview is computed from the loaded snapshot; the
// transaction recomputes the same plan, so the sheet and the writes agree.
func (a *App) openSettleConfirmItems(meal string, items []domain.ScheduleItem) {
	if len(items) == 0 {
		a.notice = "掌序是空的"
		return
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RecipeID)
	}
	preview := domain.SettlePreview{
		RecipeIDs: ids,
		Dishes:    len(ids),
		XP:        10 * len(ids),
	}
	if !a.pantryOff {
		preview.Changes = schedule.PlanDeduction(a.recipesForItems(items), a.items, a.matcher)
	}
	a.sched.preview = preview
	a.sched.settleMeal = meal
	a.sched.settleIDs = ids
	a.sched.confirmOpen = true
}
