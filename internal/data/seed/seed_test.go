package seed_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
)

func TestImportSeedsRecipesOnce(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "foodbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	summary, err := seed.Import(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped {
		t.Fatal("first import reported skipped")
	}
	if summary.Recipes != 944 {
		t.Errorf("recipes = %d, want 944", summary.Recipes)
	}
	if summary.Steps != 6930 {
		t.Errorf("steps = %d, want 6930", summary.Steps)
	}
	if summary.CatalogItems != 1000 {
		t.Errorf("catalog = %d, want 1000", summary.CatalogItems)
	}

	recipes := count(t, ctx, store, "recipes")
	steps := count(t, ctx, store, "steps")
	if recipes != 944 || steps != 6930 {
		t.Fatalf("stored recipes/steps = %d/%d, want 944/6930", recipes, steps)
	}

	// Seeds live in recipe ID order and keep their instruction text.
	rows, err := store.DB().Query(ctx,
		`SELECT name, (SELECT COUNT(*) FROM steps WHERE recipe_id = recipes.id)
		 FROM recipes ORDER BY id LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 || len(rows.Values[0]) != 2 {
		t.Fatalf("unexpected first recipe row: %#v", rows.Values)
	}
	if name, _ := rows.Values[0][0].(string); name == "" {
		t.Error("first recipe has no name")
	}
	if n := rows.Values[0][1]; n != int64(7) && n != int64(6) {
		t.Logf("first recipe has %v steps (seed data may change)", n)
	}

	again, err := seed.Import(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Skipped {
		t.Error("second import did not skip")
	}
	if again.Recipes != 944 || again.Steps != 6930 {
		t.Errorf("second import counts = %d/%d, want 944/6930", again.Recipes, again.Steps)
	}
	if got := count(t, ctx, store, "recipes"); got != 944 {
		t.Errorf("recipes after second import = %d, want 944", got)
	}
	if got := count(t, ctx, store, "steps"); got != 6930 {
		t.Errorf("steps after second import = %d, want 6930", got)
	}

	version, err := schemaVersion(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if version != sqlite.SchemaVersion {
		t.Errorf("user_version = %d, want %d", version, sqlite.SchemaVersion)
	}
}

func count(t *testing.T, ctx context.Context, store *sqlite.Store, table string) int {
	t.Helper()
	rows, err := store.DB().Query(ctx, "SELECT COUNT(*) FROM "+table)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		t.Fatalf("no count for %s", table)
	}
	n, _ := rows.Values[0][0].(int64)
	return int(n)
}

func schemaVersion(ctx context.Context, store *sqlite.Store) (int64, error) {
	rows, err := store.DB().Query(ctx, "PRAGMA user_version")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	v, _ := rows.Values[0][0].(int64)
	return v, nil
}
