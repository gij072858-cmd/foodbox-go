package nutrition

// Intake aggregation for the nutrition dashboard: the last seven days, today's
// macros, and the ingredients that contributed most. Everything here runs off
// the record slice the app already holds; nothing touches the database.

import (
	"sort"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// RecentDays is how many days the dashboard charts.
const RecentDays = 7

// TopContributors is how many rows the "top ingredients" lists keep.
const TopContributors = 6

// DayIntake is one local calendar day's estimated intake.
type DayIntake struct {
	Date     time.Time
	Kcal     float64
	ProteinG float64
	FatG     float64
	CarbG    float64
	PurineMg float64
	// Dishes is how many records fall on the day.
	Dishes int
	// Full / Partial / Uncovered count dish estimates by coverage.
	Full      int
	Partial   int
	Uncovered int
	// PurineMissing counts dishes the purine table could not estimate.
	PurineMissing int
}

// IntakeReport is what the dashboard needs, computed once per records change.
type IntakeReport struct {
	// Days is oldest first, ending with today.
	Days  []DayIntake
	Today DayIntake
	// Week is the total over Days.
	Week DayIntake
	// Focus is the day the contributor lists were built from: today when it
	// has records, otherwise the most recent day that does.
	Focus time.Time
	// FocusPurine is the worst purine tier among the focus day's dishes.
	FocusPurine PurineLevel
	// TopKcal / TopProtein / TopPurine are the focus day's largest
	// contributors, per serving, already sorted and limited.
	TopKcal    []Contribution
	TopProtein []Contribution
	TopPurine  []Contribution
}

// AnalyzeIntake buckets records into the last RecentDays local days and picks
// the focus day's top contributors. It is deterministic for a given input.
func AnalyzeIntake(records []domain.Record, recipes map[int64]domain.Recipe, catalog *Catalog, now time.Time) IntakeReport {
	start := startOfDay(now).AddDate(0, 0, -(RecentDays - 1))
	report := IntakeReport{Days: make([]DayIntake, RecentDays)}
	for i := range report.Days {
		report.Days[i].Date = start.AddDate(0, 0, i)
	}
	index := make(map[string]int, RecentDays)
	for i := range report.Days {
		index[report.Days[i].Date.Format("2006-01-02")] = i
	}

	// totes accumulates the focus day's per-ingredient contributions.
	type acc struct {
		name   string
		grams  float64
		kcal   float64
		prot   float64
		purine float64
	}
	totes := make(map[string]*acc)
	// partsCache keeps one recipe's breakdown for the whole pass: a meal can
	// list the same dish twice, and the focus day re-reads the same rows.
	partsCache := make(map[int64][]Contribution, len(records))

	partsOf := func(rec domain.Recipe) []Contribution {
		if parts, ok := partsCache[rec.ID]; ok {
			return parts
		}
		parts := catalog.Breakdown(rec)
		partsCache[rec.ID] = parts
		return parts
	}

	for _, record := range records {
		day, ok := index[record.CompletedAt.Local().Format("2006-01-02")]
		if !ok {
			continue
		}
		bucket := &report.Days[day]
		bucket.Dishes++
		rec, known := recipes[record.RecipeID]
		if !known {
			bucket.Uncovered++
			continue
		}
		parts := partsOf(rec)
		estimate := SumContributions(parts)
		bucket.Kcal += estimate.Kcal
		bucket.ProteinG += estimate.ProteinG
		bucket.FatG += estimate.FatG
		bucket.CarbG += estimate.CarbG
		bucket.PurineMg += estimate.PurineMg
		switch estimate.Coverage {
		case CoverageFull:
			bucket.Full++
		case CoveragePartial:
			bucket.Partial++
		default:
			bucket.Uncovered++
		}
		// A dish counts as missing purine data when no ingredient in it had a
		// purine value; that is different from a dish that is simply zero.
		hasPurine := false
		for _, part := range parts {
			if part.Matched && part.PurineKnown {
				hasPurine = true
				break
			}
		}
		if !hasPurine {
			bucket.PurineMissing++
		}
	}

	report.Today = report.Days[RecentDays-1]
	for _, day := range report.Days {
		report.Week.Kcal += day.Kcal
		report.Week.ProteinG += day.ProteinG
		report.Week.FatG += day.FatG
		report.Week.CarbG += day.CarbG
		report.Week.PurineMg += day.PurineMg
		report.Week.Dishes += day.Dishes
		report.Week.Full += day.Full
		report.Week.Partial += day.Partial
		report.Week.Uncovered += day.Uncovered
		report.Week.PurineMissing += day.PurineMissing
	}

	// The contributor lists describe today when today has any record,
	// otherwise the newest day that does.
	focus := -1
	if report.Today.Dishes > 0 {
		focus = RecentDays - 1
	} else {
		for i := RecentDays - 2; i >= 0; i-- {
			if report.Days[i].Dishes > 0 {
				focus = i
				break
			}
		}
	}
	if focus < 0 {
		return report
	}
	report.Focus = report.Days[focus].Date
	focusKey := report.Focus.Format("2006-01-02")
	for _, record := range records {
		if record.CompletedAt.Local().Format("2006-01-02") != focusKey {
			continue
		}
		rec, known := recipes[record.RecipeID]
		if !known {
			continue
		}
		names := make([]string, 0, len(rec.Ingredients))
		for _, ingredient := range rec.Ingredients {
			names = append(names, ingredient.Name)
		}
		if level := catalog.DishLevel(names); level.Severity() > report.FocusPurine.Severity() {
			report.FocusPurine = level
		}
		for _, part := range partsOf(rec) {
			if part.Grams <= 0 || !part.Matched {
				continue
			}
			key := catalog.Canonical(part.Name)
			row, ok := totes[key]
			if !ok {
				row = &acc{name: key}
				totes[key] = row
			}
			row.grams += part.ServedGrams
			row.kcal += part.Kcal
			row.prot += part.ProteinG
			row.purine += part.PurineMg
		}
	}
	rows := make([]Contribution, 0, len(totes))
	for _, row := range totes {
		rows = append(rows, Contribution{
			Name:        row.name,
			Grams:       row.grams,
			ServedGrams: row.grams,
			Matched:     true,
			Kcal:        row.kcal,
			ProteinG:    row.prot,
			PurineMg:    row.purine,
		})
	}
	report.TopKcal = topBy(rows, func(c Contribution) float64 { return c.Kcal })
	report.TopProtein = topBy(rows, func(c Contribution) float64 { return c.ProteinG })
	report.TopPurine = topBy(rows, func(c Contribution) float64 { return c.PurineMg })
	return report
}

// topBy sorts copies of rows with the given metric, largest first, then by
// name so equal values keep a stable order.
func topBy(rows []Contribution, metric func(Contribution) float64) []Contribution {
	out := make([]Contribution, 0, len(rows))
	for _, row := range rows {
		if metric(row) > 0 {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := metric(out[i]), metric(out[j])
		if a != b {
			return a > b
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > TopContributors {
		out = out[:TopContributors]
	}
	return out
}

// Canonical returns the normalized canonical name for an ingredient, using the
// alias table so 大蒜 and 蒜 merge into one contributor row.
func (c *Catalog) Canonical(name string) string {
	key := Normalize(name)
	if c != nil {
		if canonical, ok := c.aliases[key]; ok {
			return canonical
		}
	}
	return key
}

// startOfDay truncates to local midnight.
func startOfDay(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}
