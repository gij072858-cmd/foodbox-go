package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// settledStore is a migrated database with two dishes queued and a pantry that
// can be deducted: precise eggs and tomatoes, a coarse bag of potatoes.
func settledStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	ingredients := map[int64][]domain.Ingredient{
		1: {
			{Name: "番茄", Amount: 2, Unit: "个", Required: true},
			{Name: "鸡蛋", Amount: 3, Unit: "个", Required: true},
			{Name: "盐", Amount: 2, Unit: "g", Required: true, Seasoning: true},
		},
		2: {
			{Name: "土豆", Amount: 1, Unit: "个", Required: true},
			{Name: "番茄", Amount: 1, Unit: "个", Required: true},
		},
	}
	for id, list := range ingredients {
		raw, err := json.Marshal(list)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.DB().Execute(ctx,
			`INSERT INTO recipes (id, name, base_servings, ingredients) VALUES (?, ?, 2, ?)`,
			id, "菜"+string(rune('0'+id)), string(raw)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddScheduleItem(ctx, id, 2, ""); err != nil {
			t.Fatal(err)
		}
	}
	items := []domain.Item{
		{Name: "番茄", Category: "蔬菜", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: floatPtr(4), Unit: "个",
			StorageLocation: domain.StorageFridge},
		{Name: "鸡蛋", Category: "蛋类", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityPrecise, Quantity: floatPtr(6), Unit: "个",
			StorageLocation: domain.StorageFridge},
		{Name: "土豆", Category: "根茎", Kind: domain.ItemKindIngredient,
			QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularSome,
			StorageLocation: domain.StorageRoomTemp},
		{Name: "盐", Category: "调味料", Kind: domain.ItemKindSeasoning,
			QuantityMode: domain.QuantityPrecise, Quantity: floatPtr(500), Unit: "g",
			StorageLocation: domain.StorageRoomTemp},
	}
	if err := store.CreateItems(ctx, items); err != nil {
		t.Fatal(err)
	}
	return store, ctx
}

func floatPtr(v float64) *float64 { return &v }

// A settlement without a pantry still writes records and clears the queue,
// but leaves the stock exactly as it was.
func TestSettleRunFullNoStock(t *testing.T) {
	store, ctx := settledStore(t)
	before, err := store.ListItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 5, 18, 30, 0, 0, time.Local)
	if err := store.SettleRunFull(ctx, []int64{1, 2}, when, nil, "", false); err != nil {
		t.Fatal(err)
	}
	after, err := store.ListItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("items = %d, want %d", len(after), len(before))
	}
	for i := range before {
		if before[i].Quantity == nil || after[i].Quantity == nil {
			continue
		}
		if *before[i].Quantity != *after[i].Quantity || before[i].IsDepleted != after[i].IsDepleted {
			t.Errorf("%s changed: %+v -> %+v", before[i].Name, before[i], after[i])
		}
	}
	if n, err := store.RecordCount(ctx); err != nil || n != 2 {
		t.Errorf("records = %d (%v), want 2", n, err)
	}
	if n, err := store.ScheduleCount(ctx); err != nil || n != 0 {
		t.Errorf("schedule = %d (%v), want empty", n, err)
	}
}

// Settling one meal removes only that meal's rows; the next meal stays.
func TestSettleOneMealKeepsOtherMeals(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		if _, err := store.DB().Execute(ctx,
			`INSERT INTO recipes (id, name, base_servings) VALUES (?, ?, 2)`, id, "菜"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.AddScheduleItem(ctx, 1, 2, "午餐"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddScheduleItem(ctx, 2, 2, "晚餐"); err != nil {
		t.Fatal(err)
	}
	when := time.Now()
	if err := store.SettleRunFull(ctx, []int64{1}, when, nil, "", true); err != nil {
		t.Fatal(err)
	}
	left, err := store.ListSchedule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].RecipeID != 2 || left[0].Meal != "晚餐" {
		t.Fatalf("remaining queue = %+v, want dinner", left)
	}
}

// TestSettleRunFullSuccess pins the happy path of the single-transaction
// settlement: records, stock deduction, the day's journal, the cleared queue
// and the account counters all move together.
func TestSettleRunFullSuccess(t *testing.T) {
	store, ctx := settledStore(t)
	when := time.Date(2026, 11, 5, 18, 30, 0, 0, time.Local)
	seconds := 1500
	if err := store.SettleRunFull(ctx, []int64{1, 2}, when, &seconds, "家常菜", true); err != nil {
		t.Fatal(err)
	}

	records, err := store.ListRecords(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	for _, rec := range records {
		if rec.XPGained != 10 || rec.SessionSeconds == nil || *rec.SessionSeconds != 1500 {
			t.Errorf("record = %+v", rec)
		}
		if !rec.CompletedAt.Equal(when.UTC()) {
			t.Errorf("completed at = %v, want %v", rec.CompletedAt, when.UTC())
		}
	}

	items, err := store.ListItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]domain.Item{}
	for _, it := range items {
		byName[it.Name] = it
	}
	// 番茄 4 - (2 + 1) = 1 个; 鸡蛋 6 - 3 = 3 个; 土豆 还有一些 → 快没了.
	tomato := byName["番茄"]
	if tomato.Quantity == nil || *tomato.Quantity != 1 {
		t.Errorf("tomato = %v, want 1 个", tomato.Quantity)
	}
	egg := byName["鸡蛋"]
	if egg.Quantity == nil || *egg.Quantity != 3 {
		t.Errorf("egg = %v, want 3 个", egg.Quantity)
	}
	potato := byName["土豆"]
	if potato.GranularLevel != domain.GranularLow || potato.IsDepleted {
		t.Errorf("potato = %+v, want 快没了", potato)
	}
	salt := byName["盐"]
	if salt.Quantity == nil || *salt.Quantity != 500 {
		t.Errorf("seasoning was deducted: %v", salt.Quantity)
	}

	var journal recordJournal
	rows, err := store.Query(ctx, "SELECT date, record_ids, note FROM cook_journal")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) != 1 {
		t.Fatalf("journal rows = %d, want 1", len(rows.Values))
	}
	journal.Date = asString(rows.Values[0][0])
	if err := json.Unmarshal([]byte(asString(rows.Values[0][1])), &journal.RecordIDs); err != nil {
		t.Fatal(err)
	}
	journal.Note = asString(rows.Values[0][2])
	if journal.Date != "2026-11-05" {
		t.Errorf("journal date = %q, want 2026-11-05", journal.Date)
	}
	if len(journal.RecordIDs) != 2 || journal.Note != "家常菜" {
		t.Errorf("journal = %+v", journal)
	}

	if n, _ := store.ScheduleCount(ctx); n != 0 {
		t.Errorf("schedule after settle = %d, want 0", n)
	}
	xp, meals := profileCounters(t, store, ctx)
	if xp != 20 || meals != 1 {
		t.Errorf("profile xp/meals = %d/%d, want 20/1", xp, meals)
	}
}

type recordJournal struct {
	Date      string
	RecordIDs []int64
	Note      string
}

// TestSettleRunFullRollback proves the whole settlement is one transaction: a
// failure after the records and stock statements rolls everything back.
func TestSettleRunFullRollback(t *testing.T) {
	store, ctx := settledStore(t)
	before, err := store.ListItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settleFault.Store(true)
	defer settleFault.Store(false)

	when := time.Date(2026, 11, 5, 18, 30, 0, 0, time.Local)
	if err := store.SettleRunFull(ctx, []int64{1, 2}, when, nil, "", true); err == nil {
		t.Fatal("settlement with an injected fault returned no error")
	}

	if n, _ := store.RecordCount(ctx); n != 0 {
		t.Errorf("records after rollback = %d, want 0", n)
	}
	if n, _ := store.ScheduleCount(ctx); n != 2 {
		t.Errorf("schedule after rollback = %d, want 2", n)
	}
	rows, _ := store.Query(ctx, "SELECT COUNT(*) FROM cook_journal")
	if len(rows.Values) > 0 && asInt(rows.Values[0][0]) != 0 {
		t.Errorf("journal after rollback = %d rows", asInt(rows.Values[0][0]))
	}
	after, err := store.ListItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !itemsEqual(before, after) {
		t.Errorf("pantry changed after rollback: %v -> %v", before, after)
	}
	xp, meals := profileCounters(t, store, ctx)
	if xp != 0 || meals != 0 {
		t.Errorf("profile after rollback = %d xp, %d meals", xp, meals)
	}
}

// profileCounters reads the account's xp and meal count.
func profileCounters(t *testing.T, store *Store, ctx context.Context) (int, int) {
	t.Helper()
	rows, err := store.Query(ctx, "SELECT xp, total_meals FROM profiles WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) < 2 {
		return 0, 0
	}
	return int(asInt(rows.Values[0][0])), int(asInt(rows.Values[0][1]))
}

func itemsEqual(a, b []domain.Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].GranularLevel != b[i].GranularLevel || a[i].IsDepleted != b[i].IsDepleted {
			return false
		}
		switch {
		case a[i].Quantity == nil && b[i].Quantity == nil:
		case a[i].Quantity == nil || b[i].Quantity == nil:
			return false
		case *a[i].Quantity != *b[i].Quantity:
			return false
		}
	}
	return true
}

// TestSettleRunFullIdempotent settles the same queue twice: the second call is
// a no-op, so stock and records are not written twice.
func TestSettleRunFullIdempotent(t *testing.T) {
	store, ctx := settledStore(t)
	when := time.Date(2026, 11, 5, 18, 30, 0, 0, time.Local)
	if err := store.SettleRunFull(ctx, []int64{1, 2}, when, nil, "", true); err != nil {
		t.Fatal(err)
	}
	if err := store.SettleRunFull(ctx, []int64{1, 2}, when, nil, "", true); err != nil {
		t.Fatalf("second settlement = %v", err)
	}
	if n, _ := store.RecordCount(ctx); n != 2 {
		t.Errorf("records = %d, want 2", n)
	}
	items, _ := store.ListItems(ctx)
	for _, it := range items {
		if it.Name != "番茄" {
			continue
		}
		if it.Quantity == nil || *it.Quantity != 1 {
			t.Errorf("tomato = %v, want 1 个 (deducted once)", it.Quantity)
		}
	}
	xp, meals := profileCounters(t, store, ctx)
	if xp != 20 || meals != 1 {
		t.Errorf("profile = %d xp, %d meals, want 20 xp and 1 meal", xp, meals)
	}
}

// TestEnqueueRecipesBatch checks the batch queue write used by the private
// kitchen's "加入掌序": one transaction, no duplicates.
func TestEnqueueRecipesBatch(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2, 3} {
		if _, err := store.DB().Execute(ctx,
			`INSERT INTO recipes (id, name, base_servings) VALUES (?, ?, 3)`, id, "菜"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.EnqueueRecipes(ctx, []domain.ScheduleItem{{RecipeID: 1}, {RecipeID: 2}, {RecipeID: 2}}); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListSchedule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("schedule = %+v, want 2 rows", items)
	}
	if items[0].Servings != 3 {
		t.Errorf("servings = %d, want the recipe's base servings", items[0].Servings)
	}
}

// TestRunSessionPersistence round-trips the run session through app_meta and
// checks the clear path.
func TestRunSessionPersistence(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.LoadRunSession(ctx); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	session := domain.RunSession{
		RecipeIDs: []int64{5, 7},
		Step:      3,
		StartedAt: 1_760_000_000,
		Paused:    true,
		PausedFor: 90,
		UpdatedAt: 1_760_000_100,
	}
	if err := store.SaveRunSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadRunSession(ctx)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Step != 3 || got.PausedFor != 90 || !got.Paused || len(got.RecipeIDs) != 2 {
		t.Errorf("session = %+v", got)
	}
	if got.ElapsedSeconds(1_760_000_200) != 110 {
		t.Errorf("elapsed = %d, want 110", got.ElapsedSeconds(1_760_000_200))
	}
	if err := store.ClearRunSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.LoadRunSession(ctx); err != nil || ok {
		t.Errorf("after clear: ok=%v err=%v", ok, err)
	}
}
