package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// ListSchedule reads the dishes waiting to be cooked, oldest first.
func (s *Store) ListSchedule(ctx context.Context) ([]domain.ScheduleItem, error) {
	rows, err := s.Query(ctx, `SELECT id, recipe_id, added_at, servings, status, planned_for, meal
		FROM schedule_items ORDER BY added_at, id`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ScheduleItem, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 7 {
			continue
		}
		it := domain.ScheduleItem{
			ID:       asInt(v[0]),
			RecipeID: asInt(v[1]),
			AddedAt:  asTime(v[2]),
			Servings: int(asInt(v[3])),
			Status:   domain.ScheduleStatus(asString(v[4])),
			Meal:     asString(v[6]),
		}
		it.PlannedFor = asTimePtr(v[5])
		out = append(out, it)
	}
	return out, nil
}

// AddScheduleItem adds a dish once; the unique index keeps "tap twice" from
// creating two rows. It reports whether a row was inserted.
func (s *Store) AddScheduleItem(ctx context.Context, recipeID int64, servings int, meal string) (bool, error) {
	if servings <= 0 {
		servings = 2
	}
	res, err := s.DB().Execute(ctx, `INSERT INTO schedule_items (recipe_id, servings, status, meal)
		VALUES (?, ?, 'pending', ?) ON CONFLICT(recipe_id) DO NOTHING`, recipeID, servings, meal)
	if err != nil {
		return false, err
	}
	return res.Changes > 0, nil
}

// RemoveScheduleItem takes one dish off the schedule.
func (s *Store) RemoveScheduleItem(ctx context.Context, recipeID int64) error {
	return s.Execute(ctx, "DELETE FROM schedule_items WHERE recipe_id = ?", recipeID)
}

// ClearSchedule empties the whole schedule.
func (s *Store) ClearSchedule(ctx context.Context) error {
	return s.Execute(ctx, "DELETE FROM schedule_items")
}

// ScheduleCount returns how many dishes are queued.
func (s *Store) ScheduleCount(ctx context.Context) (int, error) {
	rows, err := s.Query(ctx, "SELECT COUNT(*) FROM schedule_items")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	return int(asInt(rows.Values[0][0])), nil
}

// SettleRun writes one record per dish and clears those schedule rows in a
// single transaction. The account's XP and meal count move with it.
func (s *Store) SettleRun(ctx context.Context, recipeIDs []int64, completedAt time.Time, sessionSeconds *int, note string) error {
	if len(recipeIDs) == 0 {
		return nil
	}
	now := completedAt.Unix()
	stmts := make([]Statement, 0, len(recipeIDs)+3)
	for _, id := range recipeIDs {
		stmts = append(stmts, Statement{
			SQL: `INSERT INTO records (recipe_id, completed_at, note, xp_gained, session_seconds)
				VALUES (?, ?, ?, 10, ?)`,
			Args: []any{id, now, nullString(note), nullableInt(sessionSeconds)},
		})
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(recipeIDs)), ",")
	args := make([]any, len(recipeIDs))
	for i, id := range recipeIDs {
		args[i] = id
	}
	stmts = append(stmts,
		Statement{SQL: fmt.Sprintf("DELETE FROM schedule_items WHERE recipe_id IN (%s)", marks), Args: args},
		Statement{SQL: "UPDATE profiles SET xp = xp + ?, total_meals = total_meals + 1 WHERE id = 1", Args: []any{10 * len(recipeIDs)}},
	)
	return s.Run(ctx, stmts)
}

// ListRecords reads the newest records.
func (s *Store) ListRecords(ctx context.Context, limit int) ([]domain.Record, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Query(ctx, `SELECT id, recipe_id, completed_at, note, xp_gained, session_seconds
		FROM records ORDER BY completed_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Record, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 6 {
			continue
		}
		out = append(out, domain.Record{
			ID:             asInt(v[0]),
			RecipeID:       asInt(v[1]),
			CompletedAt:    asTime(v[2]),
			Note:           asString(v[3]),
			XPGained:       int(asInt(v[4])),
			SessionSeconds: asIntPtr(v[5]),
		})
	}
	return out, nil
}

// RecordCount returns how many dish records exist.
func (s *Store) RecordCount(ctx context.Context) (int, error) {
	rows, err := s.Query(ctx, "SELECT COUNT(*) FROM records")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	return int(asInt(rows.Values[0][0])), nil
}

// MealDayCount returns how many distinct days have records.
func (s *Store) MealDayCount(ctx context.Context) (int, error) {
	rows, err := s.Query(ctx, `SELECT COUNT(DISTINCT date(completed_at, 'unixepoch', 'localtime'))
		FROM records`)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 {
		return 0, nil
	}
	return int(asInt(rows.Values[0][0])), nil
}
