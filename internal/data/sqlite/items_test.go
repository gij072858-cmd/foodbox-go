package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

func TestPantryCRUD(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "pantry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	until := now.AddDate(0, 0, 5)
	q := 3.5
	it := domain.Item{
		Name: "番茄", Category: "蔬菜", Kind: domain.ItemKindIngredient,
		QuantityMode: domain.QuantityPrecise, Quantity: &q, Unit: "个",
		StoredAt: &now, ShelfLifeDays: ptrInt(5), ExpireAt: &until,
		StorageLocation: domain.StorageFridge, IsCustom: true,
	}
	id, err := store.CreateItem(ctx, it)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("CreateItem returned id 0")
	}

	items, err := store.ListItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	got := items[0]
	if got.Name != "番茄" || got.Quantity == nil || *got.Quantity != 3.5 || got.Unit != "个" {
		t.Errorf("item = %+v", got)
	}
	if got.QuantityMode != domain.QuantityPrecise {
		t.Errorf("mode = %q", got.QuantityMode)
	}
	if got.ExpireAt == nil || !got.ExpireAt.Equal(until) {
		t.Errorf("expire = %v, want %v", got.ExpireAt, until)
	}
	if !got.IsCustom || got.IsDepleted {
		t.Errorf("flags = custom %v depleted %v", got.IsCustom, got.IsDepleted)
	}

	got.QuantityMode = domain.QuantityGranular
	got.GranularLevel = domain.GranularLow
	got.Quantity = nil
	if err := store.UpdateItem(ctx, got); err != nil {
		t.Fatal(err)
	}
	items, _ = store.ListItems(ctx)
	if items[0].QuantityMode != domain.QuantityGranular || items[0].GranularLevel != domain.GranularLow {
		t.Errorf("after update = %+v", items[0])
	}

	if err := store.SetItemDepleted(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	items, _ = store.ListItems(ctx)
	if !items[0].IsDepleted {
		t.Error("item not marked depleted")
	}

	if err := store.DeleteItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	items, _ = store.ListItems(ctx)
	if len(items) != 0 {
		t.Fatalf("items after delete = %d", len(items))
	}
}

func TestCreateItemsIsOneBatch(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "batch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	items := []domain.Item{
		{Name: "大米", Category: "主食", Kind: domain.ItemKindIngredient, QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularPlenty},
		{Name: "盐", Category: "调味料", Kind: domain.ItemKindSeasoning, QuantityMode: domain.QuantityGranular, GranularLevel: domain.GranularSome},
	}
	if err := store.CreateItems(ctx, items); err != nil {
		t.Fatal(err)
	}
	if n, err := store.ItemCount(ctx); err != nil || n != 2 {
		t.Fatalf("count = %d, %v", n, err)
	}

}

func ptrInt(n int) *int { return &n }
