package nutrition

// The catalog: the embedded per-100 g table, the alias map that lets a recipe
// say 大蒜 while the table says 蒜, and the purine appendix. Loading happens
// once at startup; every estimate is a table lookup plus arithmetic.
//
// Honesty rules carried over from the reference app:
//   - a name the table does not know is reported, never silently skipped;
//   - no amount means a typical portion, and that makes the estimate partial;
//   - purine values are only summed where the table actually has one.

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Coverage levels for a dish estimate.
const (
	CoverageFull    = "full"
	CoveragePartial = "partial"
	CoverageNone    = "none"
)

// Contribution is one ingredient's share of a dish estimate. The nutrient
// numbers are per base serving; Grams is the line's total weight.
type Contribution struct {
	Name        string
	Grams       float64
	ServedGrams float64
	Matched     bool
	// Inferred is true when the weight came from a typical portion rather
	// than from the line's amount.
	Inferred bool
	// UnknownUnit is true when the line named a unit this package cannot
	// convert (罐 / 包 / 份 …), so the weight is a guess.
	UnknownUnit bool
	Kcal        float64
	ProteinG    float64
	FatG        float64
	CarbG       float64
	FiberG      float64
	SodiumMg    float64
	PurineMg    float64
	PurineKnown bool
}

// load reads the three embedded files. Purines and aliases are loaded before
// the entries so the entry table can absorb them during the same pass.
func (c *Catalog) load() error {
	if err := c.loadPurines(); err != nil {
		return err
	}
	if err := c.loadAliases(); err != nil {
		return err
	}
	return c.loadEntries()
}

// loadEntries parses food_nutrition.json, filling the purine column from the
// appendix where the main table leaves it empty (the main table has no purine
// data for most meat and fish, the appendix covers the rest).
func (c *Catalog) loadEntries() error {
	b, err := assets.Data("food_nutrition.json")
	if err != nil {
		return err
	}
	var rows []Entry
	if err := json.Unmarshal(b, &rows); err != nil {
		return fmt.Errorf("food_nutrition.json: %w", err)
	}
	entries := make(map[string]Entry, len(rows))
	for _, e := range rows {
		key := Normalize(e.Name)
		if key == "" {
			continue
		}
		if e.PurineMg == nil {
			if mg, ok := c.purines[key]; ok {
				value := mg
				e.PurineMg = &value
			}
		}
		if e.PurineMg != nil && e.PurineLevel == "" {
			e.PurineLevel = string(LevelFromMg(*e.PurineMg))
		}
		entries[key] = e
	}
	c.entries = entries
	return nil
}

// loadAliases parses alias_map.json into alias -> canonical, both normalized.
func (c *Catalog) loadAliases() error {
	b, err := assets.Data("alias_map.json")
	if err != nil {
		return err
	}
	var file struct {
		Groups []struct {
			Canonical string   `json:"canonical"`
			Aliases   []string `json:"aliases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return fmt.Errorf("alias_map.json: %w", err)
	}
	aliases := make(map[string]string, len(file.Groups)*3)
	for _, g := range file.Groups {
		key := Normalize(g.Canonical)
		if key == "" {
			continue
		}
		aliases[key] = key
		for _, alias := range g.Aliases {
			if n := Normalize(alias); n != "" {
				aliases[n] = key
			}
		}
	}
	c.aliases = aliases
	return nil
}

// loadPurines parses food_purine.json: thresholds plus a name -> mg/100 g map.
func (c *Catalog) loadPurines() error {
	b, err := assets.Data("food_purine.json")
	if err != nil {
		return err
	}
	var file struct {
		Thresholds struct {
			LowBelow    float64 `json:"lowBelow"`
			MediumBelow float64 `json:"mediumBelow"`
		} `json:"thresholds"`
		Items map[string]float64 `json:"items"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return fmt.Errorf("food_purine.json: %w", err)
	}
	purines := make(map[string]float64, len(file.Items))
	for name, mg := range file.Items {
		if key := Normalize(name); key != "" {
			purines[key] = mg
		}
	}
	c.purines = purines
	if file.Thresholds.LowBelow > 0 {
		c.purineLow = file.Thresholds.LowBelow
	} else {
		c.purineLow = 50
	}
	if file.Thresholds.MediumBelow > c.purineLow {
		c.purineMedium = file.Thresholds.MediumBelow
	} else {
		c.purineMedium = 150
	}
	return nil
}

// Lookup returns one entry by ingredient name, trying the name as written and
// then its alias group's canonical name.
func (c *Catalog) Lookup(name string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	key := Normalize(name)
	if e, ok := c.entries[key]; ok {
		return e, true
	}
	if canonical, ok := c.aliases[key]; ok && canonical != key {
		if e, ok := c.entries[canonical]; ok {
			return e, true
		}
	}
	return Entry{}, false
}

// Size is how many foods the table covers.
func (c *Catalog) Size() int {
	if c == nil {
		return 0
	}
	return len(c.entries)
}

// Normalize trims, lowercases and strips spaces for table lookups. It also
// drops full-width spaces, which the seed data uses between words.
func Normalize(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.TrimSpace(name))
}

// Recipe estimates one recipe's nutrition for one base serving.
func (c *Catalog) Recipe(rec domain.Recipe) Nutrients {
	return SumContributions(c.Breakdown(rec))
}

// Breakdown weighs every ingredient of a recipe and looks it up, returning
// one row per line. The rows are what the dashboard's "top contributors" list
// is built from; Recipe is their sum.
func (c *Catalog) Breakdown(rec domain.Recipe) []Contribution {
	if c == nil || len(rec.Ingredients) == 0 {
		return nil
	}
	servings := rec.BaseServings
	if servings <= 0 {
		servings = 1
	}
	out := make([]Contribution, 0, len(rec.Ingredients))
	for _, ing := range rec.Ingredients {
		part := Contribution{Name: ing.Name}
		part.Grams, part.Inferred, part.UnknownUnit = IngredientGrams(ing)
		entry, found := c.Lookup(ing.Name)
		part.Matched = found
		if found && part.Grams > 0 {
			part.ServedGrams = part.Grams / float64(servings)
			factor := part.ServedGrams / 100
			if entry.EdibleFactor != nil && *entry.EdibleFactor > 0 && *entry.EdibleFactor <= 1 {
				factor *= *entry.EdibleFactor
			}
			part.Kcal = floatValue(entry.EnergyKcal) * factor
			part.ProteinG = floatValue(entry.ProteinG) * factor
			part.FatG = floatValue(entry.FatG) * factor
			part.CarbG = floatValue(entry.CarbG) * factor
			part.FiberG = floatValue(entry.FiberG) * factor
			part.SodiumMg = floatValue(entry.SodiumMg) * factor
			part.PurineKnown = entry.PurineMg != nil
			part.PurineMg = floatValue(entry.PurineMg) * factor
		}
		out = append(out, part)
	}
	return out
}

// SumContributions adds the rows of one dish and grades the coverage: full
// when every line was weighed from its own amount and found in the table,
// partial when something was guessed or unknown, none when nothing matched.
func SumContributions(parts []Contribution) Nutrients {
	var total Nutrients
	weighed, matched := 0, 0
	clean := true
	for _, p := range parts {
		if p.Grams <= 0 {
			// A line with no weight at all cannot be counted, and the dish
			// is not fully covered.
			clean = false
			continue
		}
		weighed++
		if p.Inferred || p.UnknownUnit {
			clean = false
		}
		if !p.Matched {
			clean = false
			continue
		}
		matched++
		total.Kcal += p.Kcal
		total.ProteinG += p.ProteinG
		total.FatG += p.FatG
		total.CarbG += p.CarbG
		total.FiberG += p.FiberG
		total.SodiumMg += p.SodiumMg
		total.PurineMg += p.PurineMg
	}
	switch {
	case weighed == 0 || matched == 0:
		total.Coverage = CoverageNone
	case clean && matched == weighed:
		total.Coverage = CoverageFull
	default:
		total.Coverage = CoveragePartial
	}
	return total
}

// IngredientGrams resolves one ingredient line to a weight in grams, plus how
// much that weight had to be guessed.
//
// The shipped recipes already carry a gram column from the data rewrite; it is
// authoritative when present. Hand-written or imported lines fall back to the
// amount × unit table, then to a typical portion.
func IngredientGrams(ing domain.Ingredient) (grams float64, inferred, unknownUnit bool) {
	if ing.Grams != nil && *ing.Grams > 0 {
		return *ing.Grams, false, false
	}
	if ing.Amount > 0 {
		if g, ok := GramsFor(ing.Name, ing.Amount, ing.Unit); ok {
			return g, false, false
		}
		return TypicalGrams(ing.Name, ing.Seasoning), true, true
	}
	return TypicalGrams(ing.Name, ing.Seasoning), true, false
}

// floatValue dereferences an optional table column, treating "no data" as
// zero. Callers that must not read a missing value as zero check the pointer
// first (see Contribution.PurineKnown).
func floatValue(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
