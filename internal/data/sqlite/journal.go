package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// Cook journal: one row per day, keyed by the local date string. The unique
// index on date is what makes a save an upsert rather than a second row.

// ListJournals reads every day's diary, newest day first.
func (s *Store) ListJournals(ctx context.Context) ([]domain.CookJournal, error) {
	rows, err := s.Query(ctx, `SELECT id, date, photo_paths, record_ids, note,
		created_at, updated_at FROM cook_journal ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.CookJournal, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 7 {
			continue
		}
		out = append(out, journalRow(v))
	}
	return out, nil
}

// GetJournal reads one day's diary. A missing day is not an error; ok is
// false and the zero journal is returned.
func (s *Store) GetJournal(ctx context.Context, date string) (domain.CookJournal, bool, error) {
	rows, err := s.Query(ctx, `SELECT id, date, photo_paths, record_ids, note,
		created_at, updated_at FROM cook_journal WHERE date = ?`, date)
	if err != nil {
		return domain.CookJournal{}, false, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) < 7 {
		return domain.CookJournal{}, false, nil
	}
	return journalRow(rows.Values[0]), true, nil
}

// SaveJournal writes one day's diary in place: the row for that date is
// updated when present and created otherwise. It returns the stored row so
// the caller can show the timestamps the database wrote.
func (s *Store) SaveJournal(ctx context.Context, j domain.CookJournal) (domain.CookJournal, error) {
	if j.Date == "" {
		return domain.CookJournal{}, errors.New("foodbox: journal date is empty")
	}
	photos, err := encodeStringList(j.PhotoPaths)
	if err != nil {
		return domain.CookJournal{}, err
	}
	records, err := encodeInt64List(j.RecordIDs)
	if err != nil {
		return domain.CookJournal{}, err
	}
	now := time.Now().Unix()
	if err := s.Execute(ctx, `INSERT INTO cook_journal
		(date, photo_paths, record_ids, note, created_at, updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			photo_paths = excluded.photo_paths,
			record_ids = excluded.record_ids,
			note = excluded.note,
			updated_at = excluded.updated_at`,
		j.Date, photos, records, nullString(j.Note), now, now); err != nil {
		return domain.CookJournal{}, err
	}
	saved, _, err := s.GetJournal(ctx, j.Date)
	return saved, err
}

// journalRow maps one cook_journal row.
func journalRow(v []any) domain.CookJournal {
	j := domain.CookJournal{
		ID:        asInt(v[0]),
		Date:      asString(v[1]),
		Note:      asString(v[4]),
		CreatedAt: asTime(v[5]),
		UpdatedAt: asTime(v[6]),
	}
	_ = json.Unmarshal([]byte(asString(v[2])), &j.PhotoPaths)
	_ = json.Unmarshal([]byte(asString(v[3])), &j.RecordIDs)
	if j.PhotoPaths == nil {
		j.PhotoPaths = []string{}
	}
	if j.RecordIDs == nil {
		j.RecordIDs = []int64{}
	}
	return j
}

// encodeStringList marshals a list column, keeping an empty list valid.
func encodeStringList(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// encodeInt64List marshals an id list column, keeping an empty list valid.
func encodeInt64List(v []int64) (string, error) {
	if v == nil {
		v = []int64{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RecipeBrief is the recipe metadata the feed and the annual review read: a
// name for the calendar, a cuisine for the chart. It skips ingredients and
// steps, which a statistics pass never touches.
// RecipeBriefs reads every recipe's label columns.
func (s *Store) RecipeBriefs(ctx context.Context) ([]domain.RecipeBrief, error) {
	rows, err := s.Query(ctx, `SELECT id, name, cuisine, region, main_category FROM recipes`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecipeBrief, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 5 {
			continue
		}
		out = append(out, domain.RecipeBrief{
			ID:           asInt(v[0]),
			Name:         asString(v[1]),
			Cuisine:      asString(v[2]),
			Region:       asString(v[3]),
			MainCategory: asString(v[4]),
		})
	}
	return out, nil
}

// ClearCookedRecords empties the cooked-dish history and resets the account's
// meal counter in one transaction. XP and the favourite list are untouched.
func (s *Store) ClearCookedRecords(ctx context.Context) error {
	return s.Run(ctx, []Statement{
		{SQL: `DELETE FROM records`},
		{SQL: `UPDATE profiles SET total_meals = 0 WHERE id = 1`},
	})
}
