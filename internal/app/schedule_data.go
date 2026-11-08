package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// setSchedule publishes a new queue snapshot and rebuilds what depends on it:
// the shopping list (the queue contributes missing ingredients) and the M4
// derived views (prep list and timeline).
func (a *App) setSchedule(items []domain.ScheduleItem) {
	a.schedule = items
	if a.scheduleSelected == 0 && len(items) > 0 {
		a.scheduleSelected = items[0].RecipeID
		a.lyricFocus = 0
		a.lyric.Y = 0
	}
	a.computeShopping()
}

func (a *App) setRecords(records []domain.Record, count, days int) {
	a.records = records
	a.recordCount = count
	a.mealDays = days
}

// reloadSchedule reads the schedule after a write (goroutine).
func (a *App) reloadSchedule() {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		items, err := store.ListSchedule(context.Background())
		a.post(func() {
			if err != nil {
				a.notice = "读取掌序失败：" + err.Error()
				return
			}
			a.setSchedule(items)
		})
	}()
}

// persistScheduleAdd writes one dish to schedule_items. The optimistic row
// is replaced by the reloaded list.
func (a *App) persistScheduleAdd(recipeID int64, servings int, meal string) {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		_, err := store.AddScheduleItem(context.Background(), recipeID, servings, meal)
		a.post(func() {
			if err != nil {
				a.notice = "加入掌序失败：" + err.Error()
				return
			}
			a.reloadSchedule()
		})
	}()
}

func (a *App) removeSchedule(recipeID int64) {
	kept := a.schedule[:0]
	for _, it := range a.schedule {
		if it.RecipeID != recipeID {
			kept = append(kept, it)
		}
	}
	a.schedule = kept
	if a.scheduleSelected == recipeID {
		a.scheduleSelected = 0
		if len(a.schedule) > 0 {
			a.scheduleSelected = a.schedule[0].RecipeID
		}
	}
	a.computeShopping()
	if a.sched.runActive {
		inRun := false
		for _, id := range a.sched.runRecipes {
			if id == recipeID {
				inRun = true
				break
			}
		}
		if len(a.schedule) == 0 || inRun {
			// The run refers to a dish that is no longer queued: drop the
			// run and its saved progress rather than cooking a ghost.
			a.sched.resetRun()
			a.sessionStarted = time.Time{}
			a.persistRunSession()
		}
	}
	store := a.store
	if store == nil {
		return
	}
	go func() {
		err := store.RemoveScheduleItem(context.Background(), recipeID)
		a.post(func() {
			if err != nil {
				a.notice = "移出掌序失败：" + err.Error()
				return
			}
			a.reloadSchedule()
		})
	}()
}

// settleIDs is the queue as settlement input, in queue order.
func (a *App) settleIDs() []int64 {
	ids := make([]int64, 0, len(a.schedule))
	for _, it := range a.schedule {
		ids = append(ids, it.RecipeID)
	}
	return ids
}

// settle closes the run: records, pantry deduction, the day's journal, the
// cleared queue and the account counters are written in one transaction. The
// queue is cleared optimistically so the sheet closes at once; a failure
// reloads it from the database, where the rollback left it untouched.
func (a *App) settle() {
	ids := append([]int64(nil), a.sched.settleIDs...)
	if len(ids) == 0 && a.sched.runActive {
		ids = append(ids, a.sched.runRecipes...)
	}
	if len(ids) == 0 {
		ids = a.settleIDs()
	}
	if len(ids) == 0 {
		a.notice = "掌序是空的"
		return
	}
	meal := a.sched.settleMeal
	if meal == "" && a.sched.runActive {
		meal = a.sched.runMeal
	}
	now := time.Now()
	var seconds *int
	if elapsed := a.sched.elapsed(now.Unix()); elapsed > 0 {
		seconds = &elapsed
	}
	// Optimistic local clear: the sheet closes at once. Only this meal's
	// rows leave the queue; the next meal stays.
	settled := make(map[int64]bool, len(ids))
	for _, id := range ids {
		settled[id] = true
	}
	keep := a.schedule[:0]
	for _, item := range a.schedule {
		if !settled[item.RecipeID] {
			keep = append(keep, item)
		}
	}
	a.schedule = keep
	if settled[a.scheduleSelected] {
		a.scheduleSelected = 0
		if len(a.schedule) > 0 {
			a.scheduleSelected = a.schedule[0].RecipeID
		}
	}
	a.sched.confirmOpen = false
	a.sched.preview = domain.SettlePreview{}
	a.sched.resetRun()
	a.sessionStarted = time.Time{}
	a.recordCount += len(ids)
	a.mealDays++
	for _, id := range ids {
		a.records = append([]domain.Record{{
			RecipeID: id, CompletedAt: now, XPGained: 10, SessionSeconds: seconds,
		}}, a.records...)
	}
	a.computeShopping()

	done := fmt.Sprintf("已结算 %d 道菜", len(ids))
	if meal != "" {
		done = fmt.Sprintf("已结算%s %d 道菜", meal, len(ids))
	}
	if len(a.schedule) > 0 {
		done += fmt.Sprintf("，还有 %d 道待做", len(a.schedule))
	}

	store := a.store
	if store == nil {
		a.notice = "已结算（内存模式）"
		if strings.HasPrefix(a.router.Path(), "/schedule/run") {
			a.router.Replace("/schedule")
		}
		return
	}
	go func() {
		ctx := context.Background()
		err := store.SettleRunFull(ctx, ids, now, seconds, "", !a.pantryOff)
		if err != nil {
			a.post(func() {
				a.notice = "结算失败，掌序已恢复：" + err.Error()
				// The transaction rolled back; reload what is really stored.
				a.reloadSchedule()
				a.reloadRecords()
				a.refreshItems()
				a.reloadRunSession()
			})
			return
		}
		_ = store.ClearRunSession(ctx)
		// profiles.xp is not part of the other reloads; without this the
		// level on 我的 would stay at its pre-settlement value until restart.
		p, perr := store.GetProfile(ctx)
		a.post(func() {
			a.notice = done
			a.reloadSchedule()
			a.reloadRecords()
			if perr == nil {
				a.prefs.profile.profile.XP = p.XP
				a.prefs.profile.profile.TotalMeals = p.TotalMeals
			}
			// The settlement deducted stock: reload the pantry snapshot.
			a.refreshItems()
			if strings.HasPrefix(a.router.Path(), "/schedule/run") {
				a.router.Replace("/schedule")
			}
		})
	}()
}

// addPrepGapsToShopping puts the prep list's shortfalls on the shopping list.
// Seasonings are skipped (they are never deducted) and so are ingredients the
// pantry already covers. A partial gap on an item that is in stock cannot be
// expressed on the shopping list — the pantry row owns it — so the notice says
// how many were added and how many stay in the prep list.
func (a *App) addPrepGapsToShopping() {
	existing := make(map[string]bool, len(a.shopExtras))
	for _, line := range a.shopExtras {
		existing[line.Name] = true
	}
	added, partial := 0, 0
	for _, line := range a.sched.prep {
		if line.Seasoning {
			continue
		}
		if line.InStock && line.Missing <= 0 {
			continue
		}
		if line.InStock {
			// Partially in stock: the shopping list drops names that are in
			// the pantry, so count it for the notice instead of adding it.
			partial++
			continue
		}
		if existing[line.Name] {
			continue
		}
		a.shopExtras = append(a.shopExtras, domain.ShoppingLine{
			Name: line.Name, Needed: line.Missing, Unit: line.Unit,
			Recipe: strings.Join(line.Recipes, "、"),
		})
		existing[line.Name] = true
		added++
	}
	a.computeShopping()
	switch {
	case added == 0 && partial == 0:
		a.notice = "缺口已经在购物清单里"
	case partial > 0:
		a.notice = fmt.Sprintf("已加入 %d 样缺口；另 %d 样在库但不够，按需补", added, partial)
	default:
		a.notice = fmt.Sprintf("已把 %d 样缺口加入购物清单", added)
	}
}

func (a *App) reloadRecords() {
	store := a.store
	if store == nil {
		return
	}
	go func() {
		recs, err := store.ListRecords(context.Background(), 200)
		if err != nil {
			return
		}
		count, _ := store.RecordCount(context.Background())
		days, _ := store.MealDayCount(context.Background())
		a.post(func() { a.setRecords(recs, count, days) })
	}()
}

// scheduleTotals sums the queue's dish durations. Unknown durations count as 0.
func (a *App) scheduleTotals() (int, int) {
	minutes := 0
	for _, it := range a.schedule {
		if rec, ok := a.recipeByID[it.RecipeID]; ok && rec.DurationMinutes != nil {
			minutes += int(*rec.DurationMinutes)
		}
	}
	return len(a.schedule), minutes
}
