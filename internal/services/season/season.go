// Package season answers questions about a recipe's seasonal months.
//
// It is pure logic: no UI, no database and no clock of its own. The caller
// passes the month, so production reads time.Now() once and tests pin a
// month. The seed stores the window as a list of month numbers, 1–12.
package season

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CurrentMonth returns the month of now in the local calendar, 1–12.
func CurrentMonth(now time.Time) int { return int(now.Month()) }

// InSeason reports whether m falls inside months.
//
// An empty list, or all twelve months, means the dish is available all
// year, so every month is in season. Months outside 1–12 are ignored.
func InSeason(months []int, m int) bool {
	window := normalize(months)
	if len(window) == 0 || len(window) == 12 {
		return true
	}
	for _, got := range window {
		if got == m {
			return true
		}
	}
	return false
}

// Marked reports whether months is a real seasonal window: at least one
// month and not the whole year. A marked window is what the seasonal badge
// and the list's 「当季」 filter talk about; an all-year dish has no season
// to be in or out of.
func Marked(months []int) bool {
	window := normalize(months)
	return len(window) > 0 && len(window) < 12
}

// Label returns a compact Chinese label for the window:
//
//	[]            全年
//	1..12         全年
//	[3 4 5]       春 · 3–5 月
//	[12 1 2]      冬 · 12–2 月
//	[6 7 8 9 10 11]  夏 · 6–8 月、秋 · 9–11 月
func Label(months []int) string {
	window := normalize(months)
	if len(window) == 0 || len(window) == 12 {
		return "全年"
	}
	runs := runs(window)
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		parts = append(parts, runLabel(run))
	}
	return strings.Join(parts, "、")
}

// SeasonName returns 春 / 夏 / 秋 / 冬 for a month, "" outside 1–12.
func SeasonName(m int) string {
	switch m {
	case 3, 4, 5:
		return "春"
	case 6, 7, 8:
		return "夏"
	case 9, 10, 11:
		return "秋"
	case 12, 1, 2:
		return "冬"
	}
	return ""
}

// normalize drops months outside 1–12, duplicates and unsorted input.
func normalize(months []int) []int {
	seen := [13]bool{}
	out := make([]int, 0, len(months))
	for _, m := range months {
		if m < 1 || m > 12 || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	sort.Ints(out)
	return out
}

// monthRun is a contiguous stretch of months. It may wrap the year end,
// as 12, 1, 2 does; Start may then be greater than End.
type monthRun struct {
	Start int
	End   int
}

// runs splits a sorted, unique window into contiguous stretches of one
// season, joining December and January so a winter window reads as one run
// but splitting at spring, summer and autumn.
func runs(window []int) []monthRun {
	if len(window) == 0 {
		return nil
	}
	present := [13]bool{}
	for _, m := range window {
		present[m] = true
	}
	starts := func(m int) bool {
		prev := prevMonth(m)
		return !present[prev] || SeasonName(prev) != SeasonName(m)
	}
	out := make([]monthRun, 0, 4)
	for _, m := range window {
		if !starts(m) {
			continue
		}
		run := monthRun{Start: m, End: m}
		for next := nextMonth(run.End); present[next] && next != run.Start &&
			SeasonName(next) == SeasonName(run.End); next = nextMonth(next) {
			run.End = next
		}
		out = append(out, run)
	}
	return out
}

func prevMonth(m int) int {
	if m == 1 {
		return 12
	}
	return m - 1
}

func nextMonth(m int) int {
	if m == 12 {
		return 1
	}
	return m + 1
}

// runLabel names one run: a season prefix when the run stays inside one
// season, otherwise just the numeric range.
func runLabel(run monthRun) string {
	same, ok := oneSeason(run)
	if !ok {
		return monthRange(run)
	}
	return same + " · " + monthRange(run)
}

// oneSeason reports the season a run lies in, false when it straddles two.
func oneSeason(run monthRun) (string, bool) {
	name := SeasonName(run.Start)
	for m := run.Start; ; m = nextMonth(m) {
		if SeasonName(m) != name {
			return "", false
		}
		if m == run.End {
			return name, true
		}
	}
}

func monthRange(run monthRun) string {
	if run.Start == run.End {
		return fmt.Sprintf("%d 月", run.Start)
	}
	return fmt.Sprintf("%d–%d 月", run.Start, run.End)
}
