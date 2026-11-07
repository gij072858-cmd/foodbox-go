package sqlite

import (
	"context"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// GetUserProfile reads the single questionnaire row. A fresh database has no
// row, so it returns the zero profile with Onboarded false.
func (s *Store) GetUserProfile(ctx context.Context) (domain.UserProfile, error) {
	rows, err := s.Query(ctx, `SELECT stove_count, cookware, hometown, age_range,
		spicy, saltiness, restrictions, allergy_note, household_size,
		household_mix, onboarded, version FROM user_profiles WHERE id = 1`)
	if err != nil {
		return domain.UserProfile{}, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) < 12 {
		return domain.UserProfile{}, nil
	}
	v := rows.Values[0]
	return domain.UserProfile{
		StoveCount:    int(asInt(v[0])),
		Cookware:      asString(v[1]),
		Hometown:      asString(v[2]),
		AgeRange:      asString(v[3]),
		Spicy:         asString(v[4]),
		Saltiness:     asString(v[5]),
		Restrictions:  asString(v[6]),
		AllergyNote:   asString(v[7]),
		HouseholdSize: int(asInt(v[8])),
		HouseholdMix:  asString(v[9]),
		Onboarded:     asBool(v[10]),
		Version:       int(asInt(v[11])),
	}, nil
}

// SaveUserProfile writes the questionnaire row, creating it on first save.
func (s *Store) SaveUserProfile(ctx context.Context, p domain.UserProfile) error {
	version := p.Version
	if version <= 0 {
		version = 1
	}
	return s.Execute(ctx, `INSERT INTO user_profiles
		(id, stove_count, cookware, hometown, age_range, spicy, saltiness,
		 restrictions, allergy_note, household_size, household_mix, onboarded,
		 version, updated_at)
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
		p.StoveCount, nullString(p.Cookware), nullString(p.Hometown),
		nullString(p.AgeRange), nullString(p.Spicy), nullString(p.Saltiness),
		p.Restrictions, nullString(p.AllergyNote), p.HouseholdSize,
		defaultJSON(p.HouseholdMix, `{"adult":1,"kid":0,"elder":0}`),
		p.Onboarded, version, time.Now().Unix())
}

// defaultJSON keeps a NOT NULL JSON column valid when the caller leaves it
// empty.
func defaultJSON(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
