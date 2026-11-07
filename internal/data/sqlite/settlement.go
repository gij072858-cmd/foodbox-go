package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
	cookmatch "github.com/gij072858-cmd/foodbox-go/internal/services/matcher"
	"github.com/gij072858-cmd/foodbox-go/internal/services/schedule"
)

// settleFault injects a failing statement into the next settlement. It exists
// so the rollback test can fail inside the transaction the way a disk error
// would; production never sets it.
var settleFault atomic.Bool

var (
	settleMatcherOnce sync.Once
	settleMatcher     *cookmatch.Matcher
)

// settlementMatcher is the alias table the settlement deducts stock with. It is
// the same table the three-state matcher uses, so "can cook" and "deducts on
// settle" cannot drift apart.
func settlementMatcher() *cookmatch.Matcher {
	settleMatcherOnce.Do(func() { settleMatcher = cookmatch.MustNew() })
	return settleMatcher
}

// SettleRunFull closes a cooking run in one transaction:
//
//   - one records row per dish (xp 10, the run's elapsed seconds);
//   - the pantry deduction: required non-seasoning ingredients matched through
//     the alias table lose their amount, coarse rows step down a level, rows
//     that reach zero are marked depleted, seasonings are left alone;
//   - today's cook_journal row (local date) upserted with the new record ids
//     appended;
//   - the settled rows cleared from schedule_items;
//   - profiles.xp and profiles.total_meals updated.
//
// Dishes no longer in the schedule are skipped, which makes a repeated
// settlement a no-op instead of a second deduction. Any error rolls the whole
// transaction back: stock, records, journal and profile return to what they
// were before the call.
func (s *Store) SettleRunFull(ctx context.Context, recipeIDs []int64, completedAt time.Time, sessionSeconds *int, note string, deductPantry bool) error {
	if len(recipeIDs) == 0 {
		return nil
	}
	pending, err := s.pendingRecipeIDs(ctx, recipeIDs)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		// Already settled (or removed from the queue): nothing to write.
		return nil
	}
	var changes []domain.StockChange
	if deductPantry {
		recipes, err := s.recipesForSettlement(ctx, pending)
		if err != nil {
			return err
		}
		items, err := s.ListItems(ctx)
		if err != nil {
			return err
		}
		changes = schedule.PlanDeduction(recipes, items, settlementMatcher())
	}

	now := completedAt.Unix()
	date := completedAt.In(time.Local).Format("2006-01-02")

	existing, err := s.journalRecordIDs(ctx, date)
	if err != nil {
		return err
	}
	baseID, err := s.maxRecordID(ctx)
	if err != nil {
		return err
	}
	recordIDs := make([]int64, len(pending))
	for i := range pending {
		recordIDs[i] = baseID + 1 + int64(i)
	}
	journalIDs, err := json.Marshal(mergeIDs(existing, recordIDs))
	if err != nil {
		return err
	}

	stmts := make([]Statement, 0, len(pending)+len(changes)+4)
	for _, id := range pending {
		stmts = append(stmts, Statement{
			SQL: `INSERT INTO records (recipe_id, completed_at, note, xp_gained, session_seconds)
				VALUES (?, ?, ?, 10, ?)`,
			Args: []any{id, now, nullString(note), nullableInt(sessionSeconds)},
		})
	}
	for _, ch := range changes {
		if ch.Skipped {
			continue
		}
		if ch.Precise {
			stmts = append(stmts, Statement{
				SQL:  `UPDATE items SET quantity = ?, is_depleted = ?, updated_at = ? WHERE id = ?`,
				Args: []any{ch.After, ch.Depleted, now, ch.ItemID},
			})
			continue
		}
		stmts = append(stmts, Statement{
			SQL: `UPDATE items SET granular_level = ?, quantity = NULL,
				is_depleted = ?, updated_at = ? WHERE id = ?`,
			Args: []any{nullString(string(ch.AfterLevel)), ch.Depleted, now, ch.ItemID},
		})
	}
	stmts = append(stmts, Statement{
		SQL: `INSERT INTO cook_journal (date, photo_paths, record_ids, note, created_at, updated_at)
			VALUES (?, '[]', ?, ?, ?, ?)
			ON CONFLICT(date) DO UPDATE SET
				record_ids = excluded.record_ids,
				updated_at = excluded.updated_at`,
		Args: []any{date, string(journalIDs), nullString(note), now, now},
	})
	marks := strings.TrimSuffix(strings.Repeat("?,", len(pending)), ",")
	args := make([]any, len(pending))
	for i, id := range pending {
		args[i] = id
	}
	stmts = append(stmts,
		Statement{SQL: fmt.Sprintf("DELETE FROM schedule_items WHERE recipe_id IN (%s)", marks), Args: args},
		Statement{SQL: "UPDATE profiles SET xp = xp + ?, total_meals = total_meals + 1 WHERE id = 1",
			Args: []any{10 * len(pending)}},
	)
	if settleFault.Load() {
		stmts = append(stmts, Statement{SQL: "INSERT INTO foodbox_settle_fault (id) VALUES (1)"})
	}
	return s.Run(ctx, stmts)
}

// EnqueueRecipes adds several dishes to the schedule in one transaction. It is
// the batch entry point the private kitchen's "加入掌序" calls; each item
// carries the meal group it belongs to.
func (s *Store) EnqueueRecipes(ctx context.Context, items []domain.ScheduleItem) error {
	if len(items) == 0 {
		return nil
	}
	stmts := make([]Statement, 0, len(items))
	for _, it := range items {
		stmts = append(stmts, Statement{
			SQL: `INSERT INTO schedule_items (recipe_id, servings, status, meal)
				SELECT id, CASE WHEN ? > 0 THEN ? WHEN base_servings > 0 THEN base_servings ELSE 2 END,
					'pending', ?
				FROM recipes WHERE id = ?
				ON CONFLICT(recipe_id) DO NOTHING`,
			Args: []any{it.Servings, it.Servings, it.Meal, it.RecipeID},
		})
	}
	return s.Run(ctx, stmts)
}

// pendingRecipeIDs filters the requested dishes down to the ones still in the
// queue, keeping the queue's order.
func (s *Store) pendingRecipeIDs(ctx context.Context, recipeIDs []int64) ([]int64, error) {
	wanted := make(map[int64]bool, len(recipeIDs))
	for _, id := range recipeIDs {
		wanted[id] = true
	}
	rows, err := s.Query(ctx, "SELECT recipe_id FROM schedule_items ORDER BY added_at, id")
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(recipeIDs))
	for _, v := range rows.Values {
		if len(v) == 0 {
			continue
		}
		id := asInt(v[0])
		if wanted[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// recipesForSettlement reads the dishes to settle, in the requested order.
func (s *Store) recipesForSettlement(ctx context.Context, ids []int64) ([]domain.Recipe, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.Query(ctx, "SELECT id, name, ingredients, base_servings FROM recipes WHERE id IN ("+marks+")", args...)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.Recipe, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 4 {
			continue
		}
		rec := domain.Recipe{
			ID:           asInt(v[0]),
			Name:         asString(v[1]),
			BaseServings: int(asInt(v[3])),
		}
		if err := json.Unmarshal([]byte(asString(v[2])), &rec.Ingredients); err != nil {
			return nil, fmt.Errorf("foodbox: recipe %d ingredients: %w", rec.ID, err)
		}
		byID[rec.ID] = rec
	}
	out := make([]domain.Recipe, 0, len(ids))
	for _, id := range ids {
		if rec, ok := byID[id]; ok {
			out = append(out, rec)
		}
	}
	return out, nil
}

// journalRecordIDs reads the record ids already on a day's journal.
func (s *Store) journalRecordIDs(ctx context.Context, date string) ([]int64, error) {
	rows, err := s.Query(ctx, "SELECT record_ids FROM cook_journal WHERE date = ?", date)
	if err != nil {
		return nil, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return nil, nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(asString(rows.Values[0][0])), &ids); err != nil {
		return nil, nil
	}
	return ids, nil
}

// maxRecordID is the highest record id; the newly inserted rows follow it.
func (s *Store) maxRecordID(ctx context.Context) (int64, error) {
	rows, err := s.Query(ctx, "SELECT COALESCE(MAX(id), 0) FROM records")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	return asInt(rows.Values[0][0]), nil
}

// mergeIDs appends the new ids to the existing ones, de-duplicated and sorted
// so the journal stays stable across settlements.
func mergeIDs(existing, added []int64) []int64 {
	seen := make(map[int64]bool, len(existing)+len(added))
	out := make([]int64, 0, len(existing)+len(added))
	for _, id := range append(append([]int64{}, existing...), added...) {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
