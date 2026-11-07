package app

// M6 · 食历、开饭日记与年度回顾。
//
// This file is the feed workstream's home: the journal state, the calendar
// index, the annual review the charts read, and the day editor. feedPage (the
// calendar page) lives in feed_views.go.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/design"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	"github.com/gij072858-cmd/foodbox-go/internal/repository"
	"github.com/gij072858-cmd/foodbox-go/internal/services/review"
)

// feedState is the feed's view state: the cook history the calendar and the
// review read, one row per day of the cook journal, and the day editor.
type feedState struct {
	// records is the whole cook history. loadFeedState reads it once at
	// startup; ensure() tops it up from the shell's snapshot after a
	// settlement, so the calendar keeps the older months as well.
	records []domain.Record
	// recipes is the brief recipe metadata the calendar and review label
	// things with, keyed by recipe id.
	recipes map[int64]review.Recipe
	// journals is the day diary keyed by local date (YYYY-MM-DD).
	journals map[string]domain.CookJournal

	// built is true once the calendar and the review have been derived.
	built bool
	// stamp and stampLen remember which shell record snapshot the derived
	// views were built from; ensure rebuilds only when it moves.
	stamp    int
	stampLen int

	// Calendar month on screen; jump is the date field's value. date is
	// the day the framework's calendar has chosen and splitWidth the
	// calendar column's width.
	year, month int
	weeks       [][]*review.Day
	today       string
	streak      int
	jump        time.Time
	date        time.Time
	splitWidth  float32

	// Annual review.
	reviewYear  int
	reviewStats review.Stats
	years       []int

	// Day editor.
	day feedDayState
}

// feedDayState is one open day of the cook journal.
type feedDayState struct {
	date    string
	records []domain.Record
	note    string
	photos  []string
	loaded  bool
}

func (s *feedState) init() {
	s.recipes = map[int64]review.Recipe{}
	s.journals = map[string]domain.CookJournal{}
	now := time.Now().In(time.Local)
	s.jump = now
	s.date = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	s.splitWidth = 360
}

// loadFeedState reads the cook history, the day diary and the recipe labels.
func (a *App) loadFeedState(ctx context.Context, store repository.Repository) error {
	records, err := store.ListRecords(ctx, 5000)
	if err != nil {
		return err
	}
	journals, err := store.ListJournals(ctx)
	if err != nil {
		return err
	}
	briefs, err := store.RecipeBriefs(ctx)
	if err != nil {
		return err
	}
	recipes := make(map[int64]review.Recipe, len(briefs))
	for _, b := range briefs {
		recipes[b.ID] = review.Recipe{
			Name: b.Name, Cuisine: b.Cuisine, Region: b.Region,
			MainCategory: b.MainCategory,
		}
	}
	byDate := make(map[string]domain.CookJournal, len(journals))
	for _, j := range journals {
		byDate[j.Date] = j
	}
	a.post(func() {
		f := &a.feed
		f.records = records
		f.recipes = recipes
		f.journals = byDate
		f.built = false // the shell's snapshot is not published yet
		f.rebuild()
	})
	return nil
}

// ensure folds the shell's newest record snapshot into the loaded history and
// rebuilds the derived views when that snapshot moved. It touches memory
// only, and only when the data changed: the shell publishes a new snapshot
// after a settlement or a restore. A page that opens before the database was
// read still gets a calendar from whatever the shell has.
func (s *feedState) ensure(a *App) {
	if s.built && s.stamp == a.recordCount && s.stampLen == len(a.records) {
		return
	}
	s.merge(a.records)
	s.stamp = a.recordCount
	s.stampLen = len(a.records)
	s.built = true
	s.rebuild()
}

// merge folds records into the history, newest first, keeping one row per
// settlement entry. The shell's optimistic records have no id yet; the
// database reload replaces them, so identity is the completion second and the
// dish, not the id.
func (s *feedState) merge(records []domain.Record) {
	seen := make(map[string]bool, len(s.records)+len(records))
	key := func(r domain.Record) string {
		return fmt.Sprintf("%d:%d", r.CompletedAt.Unix(), r.RecipeID)
	}
	merged := make([]domain.Record, 0, len(s.records)+len(records))
	for _, list := range [][]domain.Record{s.records, records} {
		for _, r := range list {
			k := key(r)
			if seen[k] {
				continue
			}
			seen[k] = true
			merged = append(merged, r)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].CompletedAt.After(merged[j].CompletedAt)
	})
	s.records = merged
}

// clearRecords drops the history after the cooked page emptied it.
func (s *feedState) clearRecords() {
	s.records = nil
	s.stamp = 0
	s.stampLen = 0
	s.rebuild()
}

// rebuild recomputes the calendar, the streak and the annual review. It runs
// when the data or the reviewed month/year changes, never while drawing.
func (s *feedState) rebuild() {
	now := time.Now()
	if s.year == 0 {
		s.year, s.month = now.Year(), int(now.Month())
	}
	if s.reviewYear == 0 {
		s.reviewYear = now.Year()
	}
	s.today = now.In(time.Local).Format("2006-01-02")
	s.streak = review.Streak(s.records, now, time.Local)
	s.years = review.Years(s.records, time.Local)
	if len(s.years) == 0 {
		s.years = []int{now.Year()}
	}
	s.reviewStats = review.Build(s.records, s.recipes, s.reviewYear, time.Local)
	s.buildMonth()
}

// buildMonth refills the calendar grid for the month on screen, Monday first.
func (s *feedState) buildMonth() {
	cells := review.Month(s.records, s.recipes, s.year, s.month, time.Local)
	first := time.Date(s.year, time.Month(s.month), 1, 0, 0, 0, 0, time.Local)
	offset := (int(first.Weekday()) + 6) % 7 // Monday is the first column
	weeks := make([][]*review.Day, 0, 6)
	week := make([]*review.Day, 0, 7)
	for i := 0; i < offset; i++ {
		week = append(week, nil)
	}
	for i := range cells {
		week = append(week, &cells[i])
		if len(week) == 7 {
			weeks = append(weeks, week)
			week = make([]*review.Day, 0, 7)
		}
	}
	if len(week) > 0 {
		for len(week) < 7 {
			week = append(week, nil)
		}
		weeks = append(weeks, week)
	}
	s.weeks = weeks
}

// shiftFeedMonth moves the calendar by delta months.
func (a *App) shiftFeedMonth(delta int) {
	f := &a.feed
	at := time.Date(f.year, time.Month(f.month), 1, 0, 0, 0, 0, time.Local).AddDate(0, delta, 0)
	f.year, f.month = at.Year(), int(at.Month())
	f.buildMonth()
}

// setReviewYear switches the annual review to another year.
func (a *App) setReviewYear(year int) {
	f := &a.feed
	f.reviewYear = year
	f.reviewStats = review.Build(f.records, f.recipes, year, time.Local)
}

// openFeedDay prepares the day editor from the loaded history. It only copies
// memory: no database, no sorting of the history.
func (a *App) openFeedDay(date string) {
	f := &a.feed
	d := &f.day
	if d.date == date && d.loaded {
		return
	}
	d.date = date
	d.loaded = true
	d.records = d.records[:0]
	for _, r := range f.records {
		if r.CompletedAt.In(time.Local).Format("2006-01-02") == date {
			d.records = append(d.records, r)
		}
	}
	sort.SliceStable(d.records, func(i, j int) bool {
		return d.records[i].CompletedAt.Before(d.records[j].CompletedAt)
	})
	if j, ok := f.journals[date]; ok {
		d.note = j.Note
		d.photos = append([]string{}, j.PhotoPaths...)
	} else {
		d.note = ""
		d.photos = nil
	}
}

// saveFeedDay writes the open day's diary in one upsert.
func (a *App) saveFeedDay() {
	d := &a.feed.day
	if !d.loaded || d.date == "" {
		return
	}
	store := a.store
	if store == nil {
		a.notice = "数据库尚未就绪"
		return
	}
	ids := make([]int64, 0, len(d.records))
	for _, r := range d.records {
		if r.ID != 0 {
			ids = append(ids, r.ID)
		}
	}
	journal := domain.CookJournal{
		Date: d.date, Note: strings.TrimSpace(d.note),
		PhotoPaths: append([]string{}, d.photos...), RecordIDs: ids,
	}
	date := d.date
	go func() {
		saved, err := store.SaveJournal(context.Background(), journal)
		a.post(func() {
			if err != nil {
				a.notice = "日记保存失败：" + err.Error()
				return
			}
			a.feed.journals[date] = saved
			a.notice = "这一天的日记已保存"
		})
	}()
}

// pickJournalPhoto asks the system for an image and records its path only.
func (a *App) pickJournalPhoto() {
	if a.win == nil {
		a.notice = "选择照片只在桌面窗口里可用"
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  a.win,
		Title:   "选择这一天的照片",
		Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"jpg", "jpeg", "png", "webp"}}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.feed.day.photos = append(a.feed.day.photos, paths...)
	a.notice = "已记下照片路径，图片本身不复制"
}

// removeJournalPhoto drops one recorded path.
func (a *App) removeJournalPhoto(index int) {
	d := &a.feed.day
	if index < 0 || index >= len(d.photos) {
		return
	}
	d.photos = append(d.photos[:index], d.photos[index+1:]...)
}

// feedDayPage is one day's journal editor.
func (a *App) feedDayPage(c *ui.Context, r *ui.Route, p design.Palette) {
	a.feed.ensure(a)
	date := r.Param("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		a.pageBody(c, func() {
			ui.Text(c, "这一天的日期读不出来。").FontSize(14).TextColor(p.Muted)
			if ui.Button(c, "回到食历").Clicked() {
				a.router.Push("/feed")
			}
		})
		return
	}
	a.openFeedDay(date)
	a.feedDayBody(c, p)
}

// feedReviewPage is the annual review: totals, the most-cooked dishes, the
// monthly chart and the cuisine split.
func (a *App) feedReviewPage(c *ui.Context, p design.Palette) {
	a.feed.ensure(a)
	a.feedReviewBody(c, p)
}
