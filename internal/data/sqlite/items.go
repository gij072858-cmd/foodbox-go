package sqlite

import (
	"context"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

const itemColumns = `id, name, category, kind, quantity_mode, quantity, unit,
	granular_level, stored_at, shelf_life_days, expire_at, storage_location,
	opened, barcode, image_path, intro_what, intro_how, intro_store, intro_tip,
	is_custom, is_depleted, note, created_at, updated_at`

// ItemCount returns the number of pantry items.
func (s *Store) ItemCount(ctx context.Context) (int, error) {
	rows, err := s.Query(ctx, "SELECT COUNT(*) FROM items")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	return int(asInt(rows.Values[0][0])), nil
}

// ListItems reads the pantry: usable items first, then expiring soonest,
// then by name. Depleted rows stay at the end (they are kept, greyed out,
// because the user may remove them or add them to a shopping list).
func (s *Store) ListItems(ctx context.Context) ([]domain.Item, error) {
	rows, err := s.Query(ctx, `SELECT `+itemColumns+` FROM items
		ORDER BY is_depleted,
			CASE WHEN expire_at IS NULL THEN 1 ELSE 0 END,
			expire_at,
			name`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Item, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 24 {
			continue
		}
		it := domain.Item{
			ID:              asInt(v[0]),
			Name:            asString(v[1]),
			Category:        asString(v[2]),
			Kind:            domain.ItemKind(asString(v[3])),
			QuantityMode:    domain.QuantityMode(asString(v[4])),
			Quantity:        asFloatPtr(v[5]),
			Unit:            asString(v[6]),
			GranularLevel:   domain.GranularLevel(asString(v[7])),
			StoredAt:        asTimePtr(v[8]),
			ShelfLifeDays:   asIntPtr(v[9]),
			ExpireAt:        asTimePtr(v[10]),
			StorageLocation: domain.StorageLocation(asString(v[11])),
			Opened:          asBoolPtr(v[12]),
			Barcode:         asString(v[13]),
			ImagePath:       asString(v[14]),
			IntroWhat:       asString(v[15]),
			IntroHow:        asString(v[16]),
			IntroStore:      asString(v[17]),
			IntroTip:        asString(v[18]),
			IsCustom:        asBool(v[19]),
			IsDepleted:      asBool(v[20]),
			Note:            asString(v[21]),
			CreatedAt:       asTime(v[22]),
			UpdatedAt:       asTime(v[23]),
		}
		if it.QuantityMode == "" {
			it.QuantityMode = domain.QuantityGranular
		}
		if it.StorageLocation == "" {
			it.StorageLocation = domain.StorageFridge
		}
		out = append(out, it)
	}
	return out, nil
}

const insertItemSQL = `INSERT INTO items
	(name, category, kind, quantity_mode, quantity, unit, granular_level,
	 stored_at, shelf_life_days, expire_at, storage_location, opened,
	 barcode, image_path, intro_what, intro_how, intro_store, intro_tip,
	 is_custom, is_depleted, note, created_at, updated_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func insertItemArgs(it domain.Item, now int64) []any {
	return []any{
		it.Name, it.Category, string(it.Kind), string(it.QuantityMode),
		nullableFloat(it.Quantity), nullString(it.Unit), nullString(string(it.GranularLevel)),
		nullableTime(it.StoredAt), nullableInt(it.ShelfLifeDays), nullableTime(it.ExpireAt),
		string(it.StorageLocation), nullableBool(it.Opened), nullString(it.Barcode),
		nullString(it.ImagePath), nullString(it.IntroWhat), nullString(it.IntroHow),
		nullString(it.IntroStore), nullString(it.IntroTip), it.IsCustom,
		it.IsDepleted, nullString(it.Note), now, now,
	}
}

// CreateItem inserts a pantry item and returns its id.
func (s *Store) CreateItem(ctx context.Context, it domain.Item) (int64, error) {
	res, err := s.DB().Execute(ctx, insertItemSQL, insertItemArgs(it, time.Now().Unix())...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertID, nil
}

// CreateItems inserts many items in one transaction, for a starter pack.
func (s *Store) CreateItems(ctx context.Context, items []domain.Item) error {
	if len(items) == 0 {
		return nil
	}
	now := time.Now().Unix()
	stmts := make([]Statement, 0, len(items))
	for _, it := range items {
		stmts = append(stmts, Statement{SQL: insertItemSQL, Args: insertItemArgs(it, now)})
	}
	return s.Run(ctx, stmts)
}

// UpdateItem writes the editable columns of an item.
func (s *Store) UpdateItem(ctx context.Context, it domain.Item) error {
	return s.Execute(ctx, `UPDATE items SET
		name = ?, category = ?, kind = ?, quantity_mode = ?, quantity = ?,
		unit = ?, granular_level = ?, stored_at = ?, shelf_life_days = ?,
		expire_at = ?, storage_location = ?, opened = ?, barcode = ?,
		image_path = ?, intro_what = ?, intro_how = ?, intro_store = ?,
		intro_tip = ?, is_custom = ?, is_depleted = ?, note = ?, updated_at = ?
		WHERE id = ?`,
		it.Name, it.Category, string(it.Kind), string(it.QuantityMode),
		nullableFloat(it.Quantity), nullString(it.Unit), nullString(string(it.GranularLevel)),
		nullableTime(it.StoredAt), nullableInt(it.ShelfLifeDays), nullableTime(it.ExpireAt),
		string(it.StorageLocation), nullableBool(it.Opened), nullString(it.Barcode),
		nullString(it.ImagePath), nullString(it.IntroWhat), nullString(it.IntroHow),
		nullString(it.IntroStore), nullString(it.IntroTip), it.IsCustom,
		it.IsDepleted, nullString(it.Note), time.Now().Unix(), it.ID)
}

// DeleteItem removes a pantry row permanently.
func (s *Store) DeleteItem(ctx context.Context, id int64) error {
	return s.Execute(ctx, "DELETE FROM items WHERE id = ?", id)
}

// SetItemDepleted marks an item used up (or puts it back in the pantry).
func (s *Store) SetItemDepleted(ctx context.Context, id int64, depleted bool) error {
	return s.Execute(ctx, "UPDATE items SET is_depleted = ?, updated_at = ? WHERE id = ?",
		depleted, time.Now().Unix(), id)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}
