package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduleAndSettlement(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "schedule.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Execute(ctx,
		"INSERT INTO recipes (id, name) VALUES (1, '测试一'), (2, '测试二')"); err != nil {
		t.Fatal(err)
	}

	added, err := store.AddScheduleItem(ctx, 1, 2, "")
	if err != nil || !added {
		t.Fatalf("first add = %v, %v", added, err)
	}
	added, err = store.AddScheduleItem(ctx, 1, 4, "")
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Error("duplicate recipe created a second schedule row")
	}
	if _, err := store.AddScheduleItem(ctx, 2, 1, ""); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListSchedule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].RecipeID != 1 || items[0].Servings != 2 {
		t.Fatalf("schedule = %+v", items)
	}

	seconds := 600
	when := time.Now().UTC().Truncate(time.Second)
	if err := store.SettleRun(ctx, []int64{1, 2}, when, &seconds, "好吃"); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.ScheduleCount(ctx); n != 0 {
		t.Errorf("schedule after settle = %d", n)
	}
	records, err := store.ListRecords(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d", len(records))
	}
	if records[0].XPGained != 10 || records[0].SessionSeconds == nil || *records[0].SessionSeconds != 600 {
		t.Errorf("record = %+v", records[0])
	}
	if n, _ := store.RecordCount(ctx); n != 2 {
		t.Errorf("record count = %d", n)
	}
	if n, _ := store.MealDayCount(ctx); n != 1 {
		t.Errorf("meal days = %d", n)
	}
	rows, err := store.Query(ctx, "SELECT xp, total_meals FROM profiles WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Values[0][0] != int64(20) || rows.Values[0][1] != int64(1) {
		t.Errorf("profile xp/meals = %#v", rows.Values[0])
	}
}

func TestRemoveScheduleItem(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Execute(ctx, `INSERT INTO recipes (id, name) VALUES (1, '测试')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddScheduleItem(ctx, 1, 2, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveScheduleItem(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.ScheduleCount(ctx); n != 0 {
		t.Errorf("count = %d", n)
	}
}
