package legacy_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/data/legacy"
	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
)

// openMigrated opens a fresh app database.
func openMigrated(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	ctx := context.Background()
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

// driftFixture writes a Drift-shaped database: only the columns an older
// Flutter schema has, so the import's column-name mapping is exercised
// rather than a positional copy.
func driftFixture(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "foodbox.sqlite")
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []sqlite.Statement{
		{SQL: `CREATE TABLE items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			category TEXT,
			kind TEXT,
			quantity_mode TEXT,
			quantity REAL,
			unit TEXT,
			granular_level TEXT,
			stored_at INTEGER,
			shelf_life_days INTEGER,
			expire_at INTEGER,
			storage_location TEXT,
			opened INTEGER,
			is_depleted INTEGER DEFAULT 0)`},
		{SQL: `CREATE TABLE recipes (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			cuisine TEXT,
			region TEXT,
			main_category TEXT,
			duration_minutes INTEGER,
			difficulty TEXT,
			base_servings INTEGER,
			season_months TEXT,
			ingredients TEXT,
			intro TEXT,
			version_group TEXT,
			is_dish INTEGER)`},
		{SQL: `CREATE TABLE steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recipe_id INTEGER,
			order_index INTEGER,
			instruction TEXT,
			duration_minutes INTEGER,
			resources TEXT)`},
		{SQL: `CREATE TABLE records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recipe_id INTEGER,
			completed_at INTEGER,
			note TEXT,
			xp_gained INTEGER)`},
		{SQL: `CREATE TABLE profiles (
			id INTEGER PRIMARY KEY,
			nickname TEXT,
			xp INTEGER,
			total_meals INTEGER,
			favorite_recipe_ids TEXT)`},
		{SQL: `INSERT INTO items(id, name, category, kind, quantity_mode, quantity, unit, storage_location, is_depleted)
			VALUES(3, '番茄', '蔬菜', 'ingredient', 'precise', 4, '个', 'fridge', 0)`},
		{SQL: `INSERT INTO recipes(id, name, cuisine, region, main_category, difficulty, base_servings, season_months, ingredients, is_dish)
			VALUES(7, '青椒肉丝', '家常菜', '川菜', '猪肉', 'easy', 2, '[1,2]',
				'[{"name":"猪肉","amount":300,"unit":"g","isRequired":true}]', 1)`},
		{SQL: `INSERT INTO steps(recipe_id, order_index, instruction, resources)
			VALUES(7, 1, '切丝', '["board"]')`},
		{SQL: `INSERT INTO records(recipe_id, completed_at, xp_gained)
			VALUES(7, 1700000000, 10)`},
		{SQL: `INSERT INTO profiles(id, nickname, xp, total_meals, favorite_recipe_ids)
			VALUES(1, '老用户', 30, 3, '[7]')`},
	}
	if err := store.Run(ctx, stmts); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func count(t *testing.T, store *sqlite.Store, table string) int {
	t.Helper()
	res, err := store.Query(context.Background(), "SELECT COUNT(*) FROM "+table)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Values) == 0 || len(res.Values[0]) == 0 {
		return 0
	}
	switch v := res.Values[0][0].(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

func TestInspectLegacyDatabase(t *testing.T) {
	path := driftFixture(t)
	sum, err := legacy.Inspect(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Recipes != 1 || sum.Steps != 1 || sum.Items != 1 || sum.Records != 1 || sum.Profiles != 1 {
		t.Fatalf("inspect counts = %+v", sum)
	}
	if sum.Empty() {
		t.Error("fixture reported empty")
	}
	if sum.Counts() == "" {
		t.Error("empty summary line")
	}
}

func TestImportLegacyReplacesData(t *testing.T) {
	ctx := context.Background()
	legacyPath := driftFixture(t)
	store := openMigrated(t, filepath.Join(t.TempDir(), "app.db"))

	// Seed first: the import must replace the seeded dishes, not append.
	if _, err := seed.Import(ctx, store, nil); err != nil {
		t.Fatal(err)
	}
	if got := count(t, store, "recipes"); got != 944 {
		t.Fatalf("seeded recipes = %d", got)
	}

	sum, err := legacy.Import(ctx, legacyPath, store)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Recipes != 1 || sum.Items != 1 {
		t.Fatalf("imported summary = %+v", sum)
	}
	if got := count(t, store, "recipes"); got != 1 {
		t.Errorf("recipes after import = %d, want 1", got)
	}
	if got := count(t, store, "steps"); got != 1 {
		t.Errorf("steps after import = %d, want 1", got)
	}
	if got := count(t, store, "items"); got != 1 {
		t.Errorf("items after import = %d, want 1", got)
	}
	if got := count(t, store, "records"); got != 1 {
		t.Errorf("records after import = %d, want 1", got)
	}

	// Values arrive by column name, with defaults for missing columns.
	res, err := store.Query(ctx, "SELECT name, category, quantity, unit, note, is_depleted FROM items")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Values) != 1 {
		t.Fatalf("items rows = %d", len(res.Values))
	}
	row := res.Values[0]
	if name, _ := row[0].(string); name != "番茄" {
		t.Errorf("item name = %v", row[0])
	}
	if category, _ := row[1].(string); category != "蔬菜" {
		t.Errorf("item category = %v", row[1])
	}
	if qty, ok := row[2].(float64); !ok || qty != 4 {
		t.Errorf("item quantity = %v", row[2])
	}
	if row[4] != nil {
		t.Errorf("item note should default to NULL, got %v", row[4])
	}

	res, err = store.Query(ctx, "SELECT name, season_months, ingredients, difficulty FROM recipes")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Values) != 1 {
		t.Fatalf("recipes rows = %d", len(res.Values))
	}
	if name, _ := res.Values[0][0].(string); name != "青椒肉丝" {
		t.Errorf("recipe name = %v", res.Values[0][0])
	}
	if months, _ := res.Values[0][1].(string); months != "[1,2]" {
		t.Errorf("season_months = %v", res.Values[0][1])
	}

	// The profile row is replaced.
	res, err = store.Query(ctx, "SELECT nickname, xp, total_meals FROM profiles WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Values) != 1 {
		t.Fatalf("profile rows = %d", len(res.Values))
	}
	if nick, _ := res.Values[0][0].(string); nick != "老用户" {
		t.Errorf("nickname = %v", res.Values[0][0])
	}

	// Importing the same file again is idempotent.
	if _, err := legacy.Import(ctx, legacyPath, store); err != nil {
		t.Fatal(err)
	}
	if got := count(t, store, "recipes"); got != 1 {
		t.Errorf("recipes after second import = %d, want 1", got)
	}
}

func TestImportRejectsNonFoodBoxDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "other.db")
	src, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := src.Run(ctx, []sqlite.Statement{{SQL: `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`}}); err != nil {
		t.Fatal(err)
	}
	_ = src.Close()

	store := openMigrated(t, filepath.Join(t.TempDir(), "app.db"))
	if _, err := legacy.Import(ctx, path, store); err == nil {
		t.Fatal("importing a non-FoodBox database succeeded")
	}
	if got := count(t, store, "recipes"); got != 0 {
		t.Errorf("failed import wrote %d recipes", got)
	}
}
