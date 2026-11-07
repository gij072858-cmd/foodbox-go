// Package matcher answers "can I cook this now?" from the pantry.
//
// The rules follow the Flutter app's RecipeMatcherV1: only required
// ingredients decide the three states, names are compared through the
// shipped alias table, and a depleted pantry row still counts as owned
// (existence matching, not quantity matching, is the v1 contract).
package matcher

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// AlmostThreshold is how many missing required ingredients still count as
// "almost can cook".
const AlmostThreshold = 2

// Status is the three-state availability of a recipe.
type Status string

const (
	CanCook Status = "can"
	Almost  Status = "almost"
	FarAway Status = "far"
)

// Label is the Chinese label shown on cards.
func (s Status) Label() string {
	switch s {
	case CanCook:
		return "现在能做"
	case Almost:
		return "差一点"
	default:
		return "差太多"
	}
}

// Result is one recipe's match.
type Result struct {
	Status          Status
	Missing         []domain.Ingredient
	OptionalMissing []domain.Ingredient
}

// MissingLabel returns "缺：A、B" or "".
func (r Result) MissingLabel() string {
	if len(r.Missing) == 0 {
		return ""
	}
	names := make([]string, len(r.Missing))
	for i, in := range r.Missing {
		names[i] = in.Name
	}
	return "缺：" + strings.Join(names, "、")
}

// Matcher holds the immutable alias table and matching rules.
type Matcher struct {
	canonical map[string]string // normalized name -> canonical
}

// New builds a matcher from the embedded alias map.
func New() (*Matcher, error) {
	b, err := assets.Data("alias_map.json")
	if err != nil {
		return nil, err
	}
	var file struct {
		Groups []struct {
			Canonical string   `json:"canonical"`
			Aliases   []string `json:"aliases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, fmt.Errorf("alias_map.json: %w", err)
	}
	canonical := make(map[string]string, len(file.Groups)*3)
	for _, g := range file.Groups {
		key := Normalize(g.Canonical)
		canonical[key] = key
		for _, alias := range g.Aliases {
			canonical[Normalize(alias)] = key
		}
	}
	return &Matcher{canonical: canonical}, nil
}

// MustNew is New for callers where the shipped asset cannot be missing.
func MustNew() *Matcher {
	m, err := New()
	if err != nil {
		panic(err)
	}
	return m
}

// Normalize trims whitespace and lowercases a name before alias lookup.
func Normalize(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
}

// Canonical returns the normalized canonical ingredient name.
func (m *Matcher) Canonical(name string) string {
	n := Normalize(name)
	if c, ok := m.canonical[n]; ok {
		return c
	}
	return n
}

// StockSet normalizes every non-depleted pantry name. Depleted rows count
// per the v1 contract, so they are included too.
func (m *Matcher) StockSet(items []domain.Item) map[string]bool {
	stock := make(map[string]bool, len(items))
	for _, it := range items {
		stock[m.Canonical(it.Name)] = true
	}
	return stock
}

// MatchWithStock matches against a prebuilt stock set.
func (m *Matcher) MatchWithStock(rec domain.Recipe, stock map[string]bool) Result {
	var missing, optional []domain.Ingredient
	counted := make(map[string]bool)
	for _, in := range rec.Ingredients {
		if !in.Required {
			continue
		}
		key := m.Canonical(in.Name)
		if stock[key] || counted[key] {
			continue
		}
		counted[key] = true
		missing = append(missing, in)
	}
	for _, in := range rec.Ingredients {
		if in.Required {
			continue
		}
		key := m.Canonical(in.Name)
		if stock[key] || counted[key] {
			continue
		}
		counted[key] = true
		optional = append(optional, in)
	}
	status := FarAway
	switch {
	case len(missing) == 0:
		status = CanCook
	case len(missing) <= AlmostThreshold:
		status = Almost
	}
	return Result{Status: status, Missing: missing, OptionalMissing: optional}
}

// Match matches one recipe against items.
func (m *Matcher) Match(rec domain.Recipe, items []domain.Item) Result {
	return m.MatchWithStock(rec, m.StockSet(items))
}

// MatchAll returns each recipe's result keyed by recipe id, building the
// stock set only once.
func (m *Matcher) MatchAll(recipes []domain.Recipe, items []domain.Item) map[int64]Result {
	stock := m.StockSet(items)
	out := make(map[int64]Result, len(recipes))
	for _, r := range recipes {
		out[r.ID] = m.MatchWithStock(r, stock)
	}
	return out
}
