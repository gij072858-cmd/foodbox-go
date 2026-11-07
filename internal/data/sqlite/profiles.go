package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// The account row: nickname, avatar, XP and the three recipe lists. A fresh
// database seeds exactly one row (id = 1).

// GetProfile reads the single account row. A database that never wrote one
// still answers with the default nickname.
func (s *Store) GetProfile(ctx context.Context) (domain.Profile, error) {
	rows, err := s.Query(ctx, `SELECT id, nickname, avatar_path, background_path,
		xp, total_meals, favorite_recipe_ids, cooked_recipe_ids,
		wanted_recipe_ids, reminder_mode FROM profiles WHERE id = 1`)
	if err != nil {
		return domain.Profile{}, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) < 10 {
		return domain.Profile{ID: 1, Nickname: "食匣用户"}, nil
	}
	v := rows.Values[0]
	p := domain.Profile{
		ID:             asInt(v[0]),
		Nickname:       asString(v[1]),
		AvatarPath:     asString(v[2]),
		BackgroundPath: asString(v[3]),
		XP:             int(asInt(v[4])),
		TotalMeals:     int(asInt(v[5])),
		ReminderMode:   asString(v[9]),
	}
	_ = json.Unmarshal([]byte(asString(v[6])), &p.FavoriteRecipes)
	_ = json.Unmarshal([]byte(asString(v[7])), &p.CookedRecipes)
	_ = json.Unmarshal([]byte(asString(v[8])), &p.WantedRecipes)
	if p.Nickname == "" {
		p.Nickname = "食匣用户"
	}
	return p, nil
}

// SaveProfile writes the account row's editable fields. XP and the meal count
// are left alone: they belong to settlement, not to the profile form.
func (s *Store) SaveProfile(ctx context.Context, p domain.Profile) error {
	nickname := p.Nickname
	if nickname == "" {
		nickname = "食匣用户"
	}
	favorites, err := encodeInt64List(p.FavoriteRecipes)
	if err != nil {
		return err
	}
	cooked, err := encodeInt64List(p.CookedRecipes)
	if err != nil {
		return err
	}
	wanted, err := encodeInt64List(p.WantedRecipes)
	if err != nil {
		return err
	}
	reminder := p.ReminderMode
	if reminder == "" {
		reminder = "vibrate"
	}
	return s.Execute(ctx, `INSERT INTO profiles
		(id, nickname, avatar_path, background_path, favorite_recipe_ids,
		 cooked_recipe_ids, wanted_recipe_ids, reminder_mode)
		VALUES(1,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			nickname = excluded.nickname,
			avatar_path = excluded.avatar_path,
			background_path = excluded.background_path,
			favorite_recipe_ids = excluded.favorite_recipe_ids,
			cooked_recipe_ids = excluded.cooked_recipe_ids,
			wanted_recipe_ids = excluded.wanted_recipe_ids,
			reminder_mode = excluded.reminder_mode`,
		nickname, nullString(p.AvatarPath), nullString(p.BackgroundPath),
		favorites, cooked, wanted, reminder)
}

// SetFavoriteRecipeIDs replaces the favourite list.
func (s *Store) SetFavoriteRecipeIDs(ctx context.Context, ids []int64) error {
	return s.setProfileList(ctx, "favorite_recipe_ids", ids)
}

// SetWantedRecipeIDs replaces the "want to cook" list.
func (s *Store) SetWantedRecipeIDs(ctx context.Context, ids []int64) error {
	return s.setProfileList(ctx, "wanted_recipe_ids", ids)
}

// setProfileList writes one JSON list column of the account row.
func (s *Store) setProfileList(ctx context.Context, column string, ids []int64) error {
	raw, err := encodeInt64List(ids)
	if err != nil {
		return err
	}
	return s.Execute(ctx, `INSERT INTO profiles (id, `+column+`) VALUES(1, ?)
		ON CONFLICT(id) DO UPDATE SET `+column+` = excluded.`+column, raw)
}

// SaveOnboarding writes the first-run questionnaire and the private kitchen's
// initial restriction row in one transaction: the two tables describe the same
// household, so they move together.
func (s *Store) SaveOnboarding(ctx context.Context, p domain.UserProfile) error {
	version := p.Version
	if version <= 0 {
		version = 1
	}
	now := time.Now().Unix()
	return s.Run(ctx, []Statement{
		{
			SQL: `INSERT INTO user_profiles
				(id, stove_count, cookware, hometown, age_range, spicy, saltiness,
				 restrictions, allergy_note, household_size, household_mix,
				 onboarded, version, updated_at)
				VALUES(1,?,?,?,?,?,?,?,?,?,?,?,?,?)
				ON CONFLICT(id) DO UPDATE SET
					stove_count = excluded.stove_count,
					cookware = excluded.cookware,
					hometown = excluded.hometown,
					age_range = excluded.age_range,
					spicy = excluded.spicy,
					saltiness = excluded.saltiness,
					restrictions = excluded.restrictions,
					allergy_note = excluded.allergy_note,
					household_size = excluded.household_size,
					household_mix = excluded.household_mix,
					onboarded = excluded.onboarded,
					version = excluded.version,
					updated_at = excluded.updated_at`,
			Args: []any{
				p.StoveCount, defaultJSON(p.Cookware, `{}`),
				nullString(p.Hometown), nullString(p.AgeRange),
				nullString(p.Spicy), nullString(p.Saltiness),
				p.Restrictions, nullString(p.AllergyNote), p.HouseholdSize,
				defaultJSON(p.HouseholdMix, `{"adult":1,"kid":0,"elder":0}`),
				p.Onboarded, version, now,
			},
		},
		{
			SQL: `INSERT INTO sichu_profiles (id, restrictions, allergy_note, updated_at)
				VALUES(1,?,?,?)
				ON CONFLICT(id) DO UPDATE SET
					restrictions = excluded.restrictions,
					allergy_note = excluded.allergy_note,
					updated_at = excluded.updated_at`,
			Args: []any{nullString(p.Restrictions), nullString(p.AllergyNote), now},
		},
	})
}
