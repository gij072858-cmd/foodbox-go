// Package review turns cook records into the numbers the feed and the annual
// review show: dishes per day, the calendar cells, the streak and one year's
// most-cooked lists. It is pure logic over domain records: no UI, no database.
package review

import (
	"sort"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Recipe is the recipe metadata the review needs: a name for the list, a
// cuisine for the chart. The full recipe (ingredients, steps) is heavier than
// a statistics pass uses.
type Recipe struct {
	Name         string
	Cuisine      string
	Region       string
	MainCategory string
}

// Day is one day of the feed calendar: how many dishes were cooked and the
// first characters of their names.
type Day struct {
	Date     string // YYYY-MM-DD in the review's zone
	Day      int
	Count    int
	Initials string
}

// Dish is one row of the most-cooked list.
type Dish struct {
	RecipeID int64
	Name     string
	Count    int
}

// Slice is one label with the number of dishes behind it.
type Slice struct {
	Label string
	Count int
}

// Stats is one year's review.
type Stats struct {
	Year          int
	Dishes        int
	Days          int
	Months        [12]int
	Top           []Dish
	Cuisines      []Slice
	LongestStreak int
}

// TopDishes is how many rows the most-cooked list keeps.
const TopDishes = 5

// maxInitials caps how many characters one calendar cell shows.
const maxInitials = 3

// Build returns the review of one year. loc decides which calendar day a
// record belongs to; nil means the machine's local zone.
func Build(records []domain.Record, recipes map[int64]Recipe, year int, loc *time.Location) Stats {
	loc = zone(loc)
	stats := Stats{Year: year, Top: []Dish{}, Cuisines: []Slice{}}
	days := make(map[string]bool)
	counts := make(map[int64]int)
	for _, r := range records {
		at := r.CompletedAt.In(loc)
		if at.Year() != year {
			continue
		}
		stats.Dishes++
		stats.Months[int(at.Month())-1]++
		days[at.Format("2006-01-02")] = true
		counts[r.RecipeID]++
	}
	stats.Days = len(days)
	stats.LongestStreak = longestStreak(days)

	top := make([]Dish, 0, len(counts))
	for id, n := range counts {
		top = append(top, Dish{RecipeID: id, Name: dishName(recipes, id), Count: n})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Count != top[j].Count {
			return top[i].Count > top[j].Count
		}
		if top[i].Name != top[j].Name {
			return top[i].Name < top[j].Name
		}
		return top[i].RecipeID < top[j].RecipeID
	})
	if len(top) > TopDishes {
		top = top[:TopDishes]
	}
	stats.Top = top

	cuisines := make(map[string]int)
	for id, n := range counts {
		cuisines[cuisineOf(recipes, id)] += n
	}
	for label, n := range cuisines {
		stats.Cuisines = append(stats.Cuisines, Slice{Label: label, Count: n})
	}
	sort.Slice(stats.Cuisines, func(i, j int) bool {
		if stats.Cuisines[i].Count != stats.Cuisines[j].Count {
			return stats.Cuisines[i].Count > stats.Cuisines[j].Count
		}
		return stats.Cuisines[i].Label < stats.Cuisines[j].Label
	})
	return stats
}

// CountByDay counts the records per local day (YYYY-MM-DD).
func CountByDay(records []domain.Record, loc *time.Location) map[string]int {
	loc = zone(loc)
	out := make(map[string]int, len(records))
	for _, r := range records {
		out[r.CompletedAt.In(loc).Format("2006-01-02")]++
	}
	return out
}

// Month returns every day of one month in order, with its count. Days without
// records are kept, with a zero count, so the caller can draw a full grid.
func Month(records []domain.Record, recipes map[int64]Recipe, year, month int, loc *time.Location) []Day {
	loc = zone(loc)
	perDay := make(map[string][]cookedDish)
	for _, r := range records {
		at := r.CompletedAt.In(loc)
		if at.Year() != year || int(at.Month()) != month {
			continue
		}
		key := at.Format("2006-01-02")
		perDay[key] = append(perDay[key], cookedDish{at: at, name: dishName(recipes, r.RecipeID)})
	}
	days := make([]Day, 0, 31)
	for d := 1; ; d++ {
		at := time.Date(year, time.Month(month), d, 0, 0, 0, 0, loc)
		if at.Month() != time.Month(month) {
			break
		}
		key := at.Format("2006-01-02")
		cell := Day{Date: key, Day: d, Count: len(perDay[key])}
		cell.Initials = initials(perDay[key])
		days = append(days, cell)
	}
	return days
}

// cookedDish is one record inside a day while Month builds the cells.
type cookedDish struct {
	at   time.Time
	name string
}

// initials takes the first character of the first distinct dishes of a day.
func initials(items []cookedDish) string {
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	seen := make(map[rune]bool, maxInitials)
	out := make([]rune, 0, maxInitials)
	for _, it := range items {
		for _, r := range it.name {
			if seen[r] {
				break
			}
			seen[r] = true
			out = append(out, r)
			break
		}
		if len(out) >= maxInitials {
			break
		}
	}
	return string(out)
}

// Streak counts the consecutive days that end today, or yesterday while today
// has no record yet. A day without records before that breaks the run.
func Streak(records []domain.Record, now time.Time, loc *time.Location) int {
	loc = zone(loc)
	days := CountByDay(records, loc)
	cursor := now.In(loc)
	if days[cursor.Format("2006-01-02")] == 0 {
		cursor = cursor.AddDate(0, 0, -1)
	}
	streak := 0
	for days[cursor.Format("2006-01-02")] > 0 {
		streak++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return streak
}

// Years lists the years that have records, newest first.
func Years(records []domain.Record, loc *time.Location) []int {
	loc = zone(loc)
	seen := make(map[int]bool)
	years := make([]int, 0, 4)
	for _, r := range records {
		y := r.CompletedAt.In(loc).Year()
		if seen[y] {
			continue
		}
		seen[y] = true
		years = append(years, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))
	return years
}

// longestStreak returns the longest run of consecutive days.
func longestStreak(days map[string]bool) int {
	keys := make([]string, 0, len(days))
	for k := range days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	best, run := 0, 0
	var previous time.Time
	for _, k := range keys {
		at, err := time.Parse("2006-01-02", k)
		if err != nil {
			continue
		}
		if !previous.IsZero() && at.Sub(previous) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
		previous = at
	}
	return best
}

func dishName(recipes map[int64]Recipe, id int64) string {
	if rec, ok := recipes[id]; ok && rec.Name != "" {
		return rec.Name
	}
	return "已删除的菜式"
}

func cuisineOf(recipes map[int64]Recipe, id int64) string {
	rec, ok := recipes[id]
	if !ok {
		return "未分类"
	}
	switch {
	case rec.Cuisine != "":
		return rec.Cuisine
	case rec.Region != "":
		return rec.Region
	case rec.MainCategory != "":
		return rec.MainCategory
	default:
		return "未分类"
	}
}

func zone(loc *time.Location) *time.Location {
	if loc == nil {
		return time.Local
	}
	return loc
}
