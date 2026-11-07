package nutrition

import (
	"math"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return c
}

func approx(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// The shipped table loads and is keyed by normalized name.
func TestCatalogLoads(t *testing.T) {
	c := testCatalog(t)
	if c.Size() < 1000 {
		t.Fatalf("Size() = %d, want the full table", c.Size())
	}
	e, ok := c.Lookup("鸡蛋")
	if !ok {
		t.Fatal("鸡蛋 missing from the table")
	}
	if e.EnergyKcal == nil || *e.EnergyKcal != 139 {
		t.Errorf("鸡蛋 energy = %v, want 139", e.EnergyKcal)
	}
	if e.EdibleFactor == nil || *e.EdibleFactor != 0.87 {
		t.Errorf("鸡蛋 edibleFactor = %v, want 0.87", e.EdibleFactor)
	}
	if _, ok := c.Lookup("表里没有的食材"); ok {
		t.Error("unknown name should not be found")
	}
}

// Aliases resolve to the table's canonical row.
func TestCatalogAliasLookup(t *testing.T) {
	c := testCatalog(t)
	cases := []struct {
		alias, canonical string
	}{
		{"大蒜", "蒜"},
		{"西红柿", "番茄"},
		{"马铃薯", "土豆"},
		{"白砂糖", "白糖"},
	}
	for _, tc := range cases {
		aliasEntry, ok := c.Lookup(tc.alias)
		if !ok {
			t.Errorf("%s not found", tc.alias)
			continue
		}
		canonicalEntry, ok := c.Lookup(tc.canonical)
		if !ok {
			t.Errorf("%s not found", tc.canonical)
			continue
		}
		if aliasEntry.Name != canonicalEntry.Name {
			t.Errorf("%s resolved to %q, want %q", tc.alias, aliasEntry.Name, canonicalEntry.Name)
		}
	}
}

// A dish's numbers come from every line: grams → per 100 g → summed. The
// expected values below are hand computed from the shipped rows for 鸡蛋
// (139 kcal, editable 0.87) and 番茄 (15 kcal, editable 0.97).
func TestRecipeAggregation(t *testing.T) {
	c := testCatalog(t)
	g100 := 100.0
	g200 := 200.0
	g3 := 3.0
	rec := domain.Recipe{
		BaseServings: 1,
		Ingredients: []domain.Ingredient{
			{Name: "鸡蛋", Amount: 2, Unit: "个", Required: true, Grams: &g100},
			{Name: "番茄", Amount: 1, Unit: "个", Required: true, Grams: &g200},
			{Name: "盐", Amount: 1, Unit: "茶匙", Seasoning: true, Grams: &g3},
		},
	}
	got := c.Recipe(rec)
	approx(t, "Kcal", got.Kcal, 139*0.87+15*0.97*2)
	approx(t, "ProteinG", got.ProteinG, 13.1*0.87+0.9*0.97*2)
	approx(t, "FatG", got.FatG, 8.6*0.87+0.2*0.97*2)
	approx(t, "CarbG", got.CarbG, 2.4*0.87+3.3*0.97*2)
	approx(t, "SodiumMg", got.SodiumMg, 131.5*0.87+9.7*0.97*2+39311*0.03)
	approx(t, "PurineMg", got.PurineMg, 17*0.97*2)
	if got.Coverage != CoverageFull {
		t.Errorf("Coverage = %q, want full", got.Coverage)
	}
}

// The estimate is per base serving, so a 2-serving dish reports half.
func TestRecipePerServing(t *testing.T) {
	c := testCatalog(t)
	g := 200.0
	rec := domain.Recipe{
		BaseServings: 2,
		Ingredients:  []domain.Ingredient{{Name: "番茄", Grams: &g}},
	}
	got := c.Recipe(rec)
	approx(t, "Kcal", got.Kcal, 15*0.97*1)
}

// Weight comes from amount × unit when the line carries no gram column.
func TestRecipeFromUnits(t *testing.T) {
	c := testCatalog(t)
	rec := domain.Recipe{
		BaseServings: 1,
		Ingredients: []domain.Ingredient{
			{Name: "鸡蛋", Amount: 2, Unit: "个"}, // 2 × 50 g
		},
	}
	got := c.Recipe(rec)
	approx(t, "Kcal", got.Kcal, 139*0.87)
	if got.Coverage != CoverageFull {
		t.Errorf("Coverage = %q, want full: both grams and the row are known", got.Coverage)
	}
}

// One unknown ingredient degrades the dish to partial but keeps the rest.
func TestRecipeCoveragePartial(t *testing.T) {
	c := testCatalog(t)
	g100 := 100.0
	g50 := 50.0
	rec := domain.Recipe{
		BaseServings: 1,
		Ingredients: []domain.Ingredient{
			{Name: "鸡蛋", Grams: &g100},
			{Name: "表里没有的食材", Grams: &g50},
		},
	}
	got := c.Recipe(rec)
	approx(t, "Kcal", got.Kcal, 139*0.87)
	if got.Coverage != CoveragePartial {
		t.Errorf("Coverage = %q, want partial", got.Coverage)
	}
}

// An ingredient with no amount falls back to a typical portion and is partial.
func TestRecipeCoverageInferred(t *testing.T) {
	c := testCatalog(t)
	rec := domain.Recipe{
		BaseServings: 1,
		Ingredients:  []domain.Ingredient{{Name: "鸡蛋"}},
	}
	got := c.Recipe(rec)
	approx(t, "Kcal", got.Kcal, 139*0.87) // typical portion is 100 g
	if got.Coverage != CoveragePartial {
		t.Errorf("Coverage = %q, want partial", got.Coverage)
	}
	parts := c.Breakdown(rec)
	if len(parts) != 1 || !parts[0].Inferred {
		t.Fatalf("Breakdown = %+v, want one inferred row", parts)
	}
}

// An unmapped unit is a guess, never a silent gram count.
func TestRecipeUnknownUnit(t *testing.T) {
	c := testCatalog(t)
	rec := domain.Recipe{
		BaseServings: 1,
		Ingredients:  []domain.Ingredient{{Name: "番茄", Amount: 1, Unit: "罐"}},
	}
	if _, ok := GramsFor("番茄", 1, "罐"); ok {
		t.Fatal("罐 must not be convertible")
	}
	got := c.Recipe(rec)
	approx(t, "Kcal", got.Kcal, 15*0.97*1.5) // typical portion 150 g
	if got.Coverage != CoveragePartial {
		t.Errorf("Coverage = %q, want partial", got.Coverage)
	}
}

// Nothing matched → none, and no invented calories.
func TestRecipeCoverageNone(t *testing.T) {
	c := testCatalog(t)
	g := 100.0
	rec := domain.Recipe{
		BaseServings: 2,
		Ingredients:  []domain.Ingredient{{Name: "不存在的食材", Grams: &g}},
	}
	got := c.Recipe(rec)
	if got.HasData() {
		t.Errorf("estimate = %+v, want no data", got)
	}
	if got.Coverage != CoverageNone {
		t.Errorf("Coverage = %q, want none", got.Coverage)
	}
	if parts := c.Breakdown(rec); len(parts) != 1 || parts[0].Matched {
		t.Errorf("Breakdown = %+v, want one unmatched row", parts)
	}
}

// An empty recipe has nothing to estimate.
func TestRecipeEmpty(t *testing.T) {
	c := testCatalog(t)
	if got := c.Recipe(domain.Recipe{}); got.HasData() || got.Coverage != CoverageNone {
		t.Errorf("empty recipe = %+v, want zeroes with none coverage", got)
	}
}

// Canonical merges alias spellings for the contributor lists.
func TestCanonical(t *testing.T) {
	c := testCatalog(t)
	if got := c.Canonical("大蒜"); got != c.Canonical("蒜") {
		t.Errorf("Canonical(大蒜) = %q, want %q", got, c.Canonical("蒜"))
	}
	if got := c.Canonical("表里没有的食材"); got != "表里没有的食材" {
		t.Errorf("Canonical kept %q, want the normalized name", got)
	}
}
