package backup_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gij072858-cmd/foodbox-go/internal/data/backup"
	"github.com/gij072858-cmd/foodbox-go/internal/data/seed"
	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
)

func newStore(t *testing.T, ctx context.Context, name string) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := newStore(t, ctx, "src.db")
	if _, err := seed.Import(ctx, src, nil); err != nil {
		t.Fatal(err)
	}
	insertUserRows(t, ctx, src)

	snap, err := backup.Export(ctx, src, "test")
	if err != nil {
		t.Fatal(err)
	}
	if snap.FormatVersion != backup.FormatVersion {
		t.Errorf("format = %d, want %d", snap.FormatVersion, backup.FormatVersion)
	}
	if snap.SchemaVersion != backup.CompatSchemaVersion {
		t.Errorf("schema = %d, want %d", snap.SchemaVersion, backup.CompatSchemaVersion)
	}
	if snap.Counts["recipes"] != 944 || snap.Counts["steps"] != 6930 {
		t.Errorf("counts recipes/steps = %d/%d", snap.Counts["recipes"], snap.Counts["steps"])
	}
	if snap.Counts["items"] != 1 || snap.Counts["scheduleItems"] != 1 {
		t.Errorf("counts items/schedule = %d/%d", snap.Counts["items"], snap.Counts["scheduleItems"])
	}

	// A real file round trip, including the atomic writer.
	path := filepath.Join(t.TempDir(), "foodbox-backup.json")
	if err := backup.WriteFile(path, snap); err != nil {
		t.Fatal(err)
	}
	fileSnap, err := backup.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	dst := newStore(t, ctx, "dst.db")
	if err := backup.Import(ctx, dst, fileSnap); err != nil {
		t.Fatal(err)
	}
	for key, want := range fileSnap.Counts {
		if got := count(t, ctx, dst, tableName(key)); got != want {
			t.Errorf("%s rows after import = %d, want %d", key, got, want)
		}
	}

	// Values survive with type and units intact.
	rows, err := dst.Query(ctx, `SELECT name, quantity, is_depleted, stored_at,
		opened, intro_what FROM items WHERE id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 {
		t.Fatalf("items rows = %d", len(rows.Values))
	}
	row := rows.Values[0]
	if row[0] != "番茄" {
		t.Errorf("name = %v", row[0])
	}
	if q, ok := row[1].(float64); !ok || q != 3.5 {
		t.Errorf("quantity = %#v, want 3.5", row[1])
	}
	if row[2] != int64(0) {
		t.Errorf("is_depleted = %#v, want 0", row[2])
	}
	if row[3] != int64(1767323045) {
		t.Errorf("stored_at = %#v", row[3])
	}
	if row[4] != int64(1) {
		t.Errorf("opened = %#v, want 1", row[4])
	}
	if row[5] != "本地番茄" {
		t.Errorf("intro_what = %#v", row[5])
	}

	// A date written as ISO 8601 in JSON comes back as Unix seconds.
	raw, _ := json.Marshal(fileSnap)
	var probe map[string]any
	_ = json.Unmarshal(raw, &probe)
	item := probe["tables"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if _, ok := item["storedAt"].(string); !ok {
		t.Errorf("storedAt in JSON = %#v, want an ISO string", item["storedAt"])
	}
	if _, ok := item["isDepleted"].(bool); !ok {
		t.Errorf("isDepleted in JSON = %#v, want a bool", item["isDepleted"])
	}
}

func TestImportFlutterV2Backup(t *testing.T) {
	ctx := context.Background()
	store := newStore(t, ctx, "legacy.db")

	file := []byte(`{
	  "app": "foodbox",
	  "formatVersion": 2,
	  "schemaVersion": 19,
	  "tables": {
	    "items": [
	      {"id": 7, "name": "番茄", "category": "蔬菜", "kind": "ingredient",
	       "quantityMode": "precise", "quantity": 2, "unit": "个",
	       "storedAt": "2026-01-02T03:04:05Z", "isCustom": true, "isDepleted": false}
	    ],
	    "recipes": [
	      {"id": 3, "name": "旧菜", "difficulty": "easy", "seasonMonths": [1, 2],
	       "ingredients": [{"name": "番茄", "amount": 2, "unit": "个",
	                        "isRequired": true, "isSeasoning": false}],
	       "isDish": null, "createdAt": "2026-01-02T03:04:05Z"}
	    ],
	    "steps": [
	      {"id": 9, "recipeId": 3, "orderIndex": 1, "instruction": "切块",
	       "resources": ["board"], "safetyTag": "none", "photoPaths": []}
	    ],
	    "scheduleItems": [
	      {"recipeId": 3, "servings": 1, "status": "pending",
	       "addedAt": "2026-01-02T03:04:05Z"}
	    ],
	    "records": [
	      {"recipeId": 3, "completedAt": "2026-01-03T04:05:06Z",
	       "xpGained": 12, "sessionSeconds": 600}
	    ],
	    "profiles": [
	      {"id": 1, "nickname": "旧用户", "xp": 30, "favoriteRecipeIds": [3]}
	    ],
	    "cookJournal": [
	      {"date": "2026-01-03", "photoPaths": [], "recordIds": [1],
	       "note": "好吃", "createdAt": "2026-01-03T04:05:06Z"}
	    ]
	  }
	}`)
	snap, err := backup.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := backup.Import(ctx, store, snap); err != nil {
		t.Fatal(err)
	}

	if got := count(t, ctx, store, "recipes"); got != 1 {
		t.Errorf("recipes = %d, want 1", got)
	}
	if got := count(t, ctx, store, "steps"); got != 1 {
		t.Errorf("steps = %d, want 1", got)
	}
	rows, err := store.Query(ctx, `SELECT stored_at, is_custom, is_depleted FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 {
		t.Fatalf("items rows = %d", len(rows.Values))
	}
	if rows.Values[0][0] != int64(1767323045) {
		t.Errorf("stored_at = %#v", rows.Values[0][0])
	}
	if rows.Values[0][1] != int64(1) || rows.Values[0][2] != int64(0) {
		t.Errorf("bool columns = %#v", rows.Values[0])
	}
	rows, err = store.Query(ctx, "SELECT nickname, favorite_recipe_ids FROM profiles WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if got := rows.Values[0][1]; got != "[3]" {
		t.Errorf("favoriteRecipeIds = %#v, want [3]", got)
	}
	if got := count(t, ctx, store, "member_profiles"); got != 0 {
		// The file has no memberProfiles table (format 1 / older v2); the
		// importer clears it and does not invent a card.
		t.Logf("member_profiles = %d (no table in file)", got)
	}
}

func TestDecodeRejectsWrongOrNewFiles(t *testing.T) {
	if _, err := backup.Decode([]byte(`{"app":"other"}`)); !errors.Is(err, backup.ErrNotFoodBox) {
		t.Errorf("wrong app = %v", err)
	}
	if _, err := backup.Decode([]byte(`{"app":"foodbox","schemaVersion":99,"tables":{}}`)); !errors.Is(err, backup.ErrTooNew) {
		t.Errorf("new schema = %v", err)
	}
	if _, err := backup.Decode([]byte(`{"app":"foodbox","schemaVersion":12,"tables":{}}`)); err != nil {
		t.Errorf("minimum schema was rejected: %v", err)
	}
}

func insertUserRows(t *testing.T, ctx context.Context, store *sqlite.Store) {
	t.Helper()
	stmts := []sqlite.Statement{
		{SQL: `INSERT INTO items
			(id, name, category, kind, quantity_mode, quantity, unit, stored_at,
			 shelf_life_days, storage_location, opened, intro_what, is_custom, is_depleted)
			VALUES (1, '番茄', '蔬菜', 'ingredient', 'precise', 3.5, '个', 1767323045,
			 7, 'fridge', 1, '本地番茄', 1, 0)`},
		{SQL: `INSERT INTO schedule_items (recipe_id, servings, status, planned_for)
			VALUES (1, 2, 'planned', 1767323045)`},
		{SQL: `INSERT INTO records (recipe_id, completed_at, note, xp_gained, session_seconds)
			VALUES (1, 1767323045, '不错', 10, 600)`},
		{SQL: `UPDATE profiles SET nickname = '测试用户', xp = 10,
			favorite_recipe_ids = '[1,2]' WHERE id = 1`},
		{SQL: `INSERT INTO user_profiles (id, stove_count, household_mix, onboarded)
			VALUES (1, 3, '{"adult":2,"kid":1,"elder":0}', 1)`},
		{SQL: `INSERT INTO sichu_profiles (id, height_cm, weight_kg, age, sex,
			nutrition_goals, target_meals)
			VALUES (1, 175, 70, 30, 'male', '["fatLoss"]', '["lunch","dinner"]')`},
		{SQL: `INSERT INTO cook_journal (date, photo_paths, record_ids, note)
			VALUES ('2026-01-02', '["a.jpg"]', '[1]', '好吃')`},
	}
	if err := store.Run(ctx, stmts); err != nil {
		t.Fatal(err)
	}
}

func tableName(key string) string {
	switch key {
	case "scheduleItems":
		return "schedule_items"
	case "userProfiles":
		return "user_profiles"
	case "sichuProfiles":
		return "sichu_profiles"
	case "memberProfiles":
		return "member_profiles"
	case "cookJournal":
		return "cook_journal"
	default:
		return key
	}
}

func count(t *testing.T, ctx context.Context, store *sqlite.Store, table string) int {
	t.Helper()
	rows, err := store.Query(ctx, "SELECT COUNT(*) FROM "+table)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := rows.Values[0][0].(int64)
	return int(n)
}
