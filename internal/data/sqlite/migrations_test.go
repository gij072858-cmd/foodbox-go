package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestMigrateCreatesEveryTable(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"app_meta", "items", "recipes", "steps",
		"schedule_items", "records", "profiles", "user_profiles",
		"sichu_profiles", "member_profiles", "cook_journal",
	}
	rows, err := store.DB().Query(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	have := make(map[string]bool, len(rows.Values))
	for _, row := range rows.Values {
		if len(row) == 1 {
			name, _ := row[0].(string)
			have[name] = true
		}
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("missing table %q", name)
		}
	}

	version, err := store.version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Errorf("user_version = %d, want %d", version, SchemaVersion)
	}

	var profile, self int64
	if rows, err := store.DB().Query(ctx, "SELECT COUNT(*) FROM profiles"); err == nil && len(rows.Values) > 0 {
		profile, _ = rows.Values[0][0].(int64)
	}
	if rows, err := store.DB().Query(ctx, "SELECT COUNT(*) FROM member_profiles WHERE is_self = 1"); err == nil && len(rows.Values) > 0 {
		self, _ = rows.Values[0][0].(int64)
	}
	if profile != 1 {
		t.Errorf("profiles rows = %d, want 1", profile)
	}
	if self != 1 {
		t.Errorf("self member rows = %d, want 1", self)
	}

	for _, index := range []string{"schedule_items_recipe_unique", "cook_journal_date_unique", "idx_records_completed_at"} {
		rows, err := store.DB().Query(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", index)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := rows.Values[0][0].(int64)
		if n != 1 {
			t.Errorf("missing index %q", index)
		}
	}
}

// v3 adds the meal column with the ungrouped default, so old queues keep
// working and new plan pushes can carry 早餐/午餐/晚餐.
func TestScheduleItemsMealColumn(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := store.DB().Query(ctx, "PRAGMA table_info(schedule_items)")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows.Values {
		if len(row) < 5 || row[1] != "meal" {
			continue
		}
		found = true
		if row[2] != "TEXT" || row[3] != int64(1) {
			t.Errorf("meal column = %+v, want NOT NULL TEXT", row)
		}
		if row[4] != "''" {
			t.Errorf("meal default = %v, want ''", row[4])
		}
	}
	if !found {
		t.Fatal("schedule_items has no meal column")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrateRejectsNewerDatabase(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if _, err := store.DB().Execute(ctx, "PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Migrate = %v, want ErrSchemaTooNew", err)
	}
}

func openTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "foodbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
