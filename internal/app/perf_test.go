package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// realRecipes parses the shipped seed, so the performance tests measure the
// data the app actually ships rather than a fixture.
func realRecipes(tb testing.TB) []domain.Recipe {
	tb.Helper()
	b, err := assets.Data("recipes.json")
	if err != nil {
		tb.Fatal(err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(b, &raws); err != nil {
		tb.Fatal(err)
	}
	out := make([]domain.Recipe, len(raws))
	for i := range raws {
		if err := json.Unmarshal(raws[i], &out[i]); err != nil {
			tb.Fatalf("recipes.json[%d]: %v", i, err)
		}
		out[i].ID = int64(i + 1)
	}
	return out
}

// realPantry builds 200 pantry rows spread over the catalog's names.
func realPantry(tb testing.TB) []domain.Item {
	tb.Helper()
	b, err := assets.Data("food_items.json")
	if err != nil {
		tb.Fatal(err)
	}
	var catalog []domain.FoodItem
	if err := json.Unmarshal(b, &catalog); err != nil {
		tb.Fatal(err)
	}
	n := 200
	if len(catalog) < n {
		n = len(catalog)
	}
	items := make([]domain.Item, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, domain.Item{
			ID:            int64(i + 1),
			Name:          catalog[i].Name,
			Category:      catalog[i].Category,
			Kind:          domain.ItemKind(catalog[i].Kind),
			QuantityMode:  domain.QuantityGranular,
			GranularLevel: domain.GranularSome,
		})
	}
	return items
}

// TestMatchPerf944 is the plan's performance gate: 944 recipes against 200
// pantry items in under 50 ms.
func TestMatchPerf944(t *testing.T) {
	recipes := realRecipes(t)
	items := realPantry(t)
	a := New()

	// Warm the alias table so the measured call is the matching itself.
	_ = a.matcher.MatchAll(recipes, items)

	start := time.Now()
	matches := a.matcher.MatchAll(recipes, items)
	elapsed := time.Since(start)
	if len(matches) != len(recipes) {
		t.Fatalf("matched %d of %d recipes", len(matches), len(recipes))
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("matching 944 recipes against 200 items took %v, budget 50ms", elapsed)
	}
	t.Logf("match 944x200: %v", elapsed)
}

// BenchmarkPageSwitchFrame measures a switch in each direction. "across"
// changes sections at the same depth; "depth" opens a dish and comes back,
// the path where the stock router used to rebuild both pages every frame.
// It is a comparison tool, not a wall-clock gate: compare the two runs on
// the same machine rather than pinning nanoseconds in CI.
func BenchmarkPageSwitchFrame(b *testing.B) {
	b.Run("across", func(b *testing.B) {
		a := newTestApp()
		tt := ui.NewTester(a.View, 1280, 860)
		settle(tt)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			a.router.Push("/recipes")
			tt.Frame()
			tt.Advance(200 * time.Millisecond)
			tt.Frame()
			a.router.Push("/pantry")
			tt.Frame()
			tt.Advance(200 * time.Millisecond)
			tt.Frame()
		}
	})
	b.Run("depth", func(b *testing.B) {
		a := newTestApp()
		tt := ui.NewTester(a.View, 1280, 860)
		settle(tt)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			a.router.Push("/recipes/1")
			tt.Frame()
			tt.Advance(200 * time.Millisecond)
			tt.Frame()
			a.router.Back()
			tt.Frame()
			tt.Advance(200 * time.Millisecond)
			tt.Frame()
		}
	})
}

// TestStaticPageRequestsNoFrames is the frame-budget gate: once a page has
// settled, nothing on it may keep asking for frames.
func TestStaticPageRequestsNoFrames(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.View, 1280, 860)
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("the settled pantry page keeps asking for frames")
	}

	if err := tt.Click("菜式"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Error("the settled recipe page keeps asking for frames")
	}
}
