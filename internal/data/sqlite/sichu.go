package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// ErrSelfMember is returned when a caller tries to edit or delete the self
// member as if it were an ordinary person.
var ErrSelfMember = errors.New("foodbox: 本人卡由身体档案同步，不能单独修改或删除")

// ErrMemberNotFound is returned when a member id does not exist.
var ErrMemberNotFound = errors.New("foodbox: 成员不存在")

const selfMemberName = "本人"
const selfMemberRole = "adult"

// sichuProfileColumns is the fixed column order GetSichuProfile scans.
const sichuProfileColumns = `height_cm, weight_kg, age, sex, activity_level,
	target_weight_kg, body_fat_pct, life_stage, allergens, health_stages,
	health_tags, health_note, nutrition_goal, nutrition_goals, calorie_target,
	restrictions, allergy_note, explore_ratio, target_meals`

// memberColumns is the fixed column order ListMembers scans.
const memberColumns = `id, name, role, is_self, height_cm, weight_kg, age, sex,
	activity_level, target_weight_kg, body_fat_pct, life_stage, allergens,
	health_stages, health_tags, health_note, nutrition_goal, nutrition_goals,
	calorie_target`

// GetSichuProfile reads the single body profile row. ok is false on a fresh
// database, where the questionnaire's defaults are all there is: a missing
// row is a state, not an error.
func (s *Store) GetSichuProfile(ctx context.Context) (domain.SichuProfile, bool, error) {
	rows, err := s.Query(ctx, `SELECT `+sichuProfileColumns+` FROM sichu_profiles WHERE id = 1`)
	if err != nil {
		return domain.SichuProfile{}, false, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) < 19 {
		return domain.SichuProfile{ExploreRatio: 0.3}, false, nil
	}
	v := rows.Values[0]
	p := domain.SichuProfile{
		HeightCm:       int(asInt(v[0])),
		WeightKg:       int(asInt(v[1])),
		Age:            int(asInt(v[2])),
		Sex:            asString(v[3]),
		ActivityLevel:  asString(v[4]),
		TargetWeightKg: int(asInt(v[5])),
		BodyFatPct:     asFloat(v[6]),
		LifeStage:      asString(v[7]),
		Allergens:      asString(v[8]),
		HealthStages:   asString(v[9]),
		HealthTags:     asString(v[10]),
		HealthNote:     asString(v[11]),
		NutritionGoal:  asString(v[12]),
		NutritionGoals: asString(v[13]),
		CalorieTarget:  int(asInt(v[14])),
		Restrictions:   asString(v[15]),
		AllergyNote:    asString(v[16]),
		ExploreRatio:   asFloat(v[17]),
		TargetMeals:    asString(v[18]),
	}
	if p.ExploreRatio < 0 || p.ExploreRatio > 1 {
		p.ExploreRatio = 0.3
	}
	return p, true, nil
}

// SaveSichuProfileWithSelf writes the body profile and the is_self member in
// one transaction, so the two never disagree about the same person.
//
// The self member's own column set is narrower than the profile (no
// restrictions, allergy note or target meals), so this copies the health
// columns and keeps the name the user gave the card.
func (s *Store) SaveSichuProfileWithSelf(ctx context.Context, p domain.SichuProfile) error {
	if p.ExploreRatio < 0 || p.ExploreRatio > 1 {
		p.ExploreRatio = 0.3
	}
	now := time.Now().Unix()

	profile := Statement{SQL: `INSERT INTO sichu_profiles
		(id, height_cm, weight_kg, age, sex, activity_level, target_weight_kg,
		 body_fat_pct, life_stage, allergens, health_stages, health_tags,
		 health_note, nutrition_goal, nutrition_goals, calorie_target,
		 restrictions, allergy_note, explore_ratio, target_meals, updated_at)
		VALUES(1,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			height_cm = excluded.height_cm,
			weight_kg = excluded.weight_kg,
			age = excluded.age,
			sex = excluded.sex,
			activity_level = excluded.activity_level,
			target_weight_kg = excluded.target_weight_kg,
			body_fat_pct = excluded.body_fat_pct,
			life_stage = excluded.life_stage,
			allergens = excluded.allergens,
			health_stages = excluded.health_stages,
			health_tags = excluded.health_tags,
			health_note = excluded.health_note,
			nutrition_goal = excluded.nutrition_goal,
			nutrition_goals = excluded.nutrition_goals,
			calorie_target = excluded.calorie_target,
			restrictions = excluded.restrictions,
			allergy_note = excluded.allergy_note,
			explore_ratio = excluded.explore_ratio,
			target_meals = excluded.target_meals,
			updated_at = excluded.updated_at`,
		Args: []any{
			nullablePositiveInt(p.HeightCm), nullablePositiveInt(p.WeightKg),
			nullablePositiveInt(p.Age), nullString(p.Sex),
			nullString(p.ActivityLevel), nullablePositiveInt(p.TargetWeightKg),
			nullablePositiveFloat(p.BodyFatPct), nullString(p.LifeStage),
			nullString(p.Allergens), nullString(p.HealthStages),
			p.HealthTags, nullString(p.HealthNote), nullString(p.NutritionGoal),
			nullString(p.NutritionGoals), nullablePositiveInt(p.CalorieTarget),
			nullString(p.Restrictions), nullString(p.AllergyNote),
			p.ExploreRatio, nullString(p.TargetMeals), now,
		},
	}

	insertSelf := Statement{SQL: `INSERT INTO member_profiles
		(name, role, is_self, height_cm, weight_kg, age, sex, activity_level,
		 target_weight_kg, body_fat_pct, life_stage, allergens, health_stages,
		 health_tags, health_note, nutrition_goal, nutrition_goals,
		 calorie_target, updated_at)
		SELECT ?, ?, 1, ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
		WHERE NOT EXISTS (SELECT 1 FROM member_profiles WHERE is_self = 1)`,
		Args: []any{
			selfMemberName, selfMemberRole,
			nullablePositiveInt(p.HeightCm), nullablePositiveInt(p.WeightKg),
			nullablePositiveInt(p.Age), nullString(p.Sex),
			nullString(p.ActivityLevel), nullablePositiveInt(p.TargetWeightKg),
			nullablePositiveFloat(p.BodyFatPct), nullString(p.LifeStage),
			nullString(p.Allergens), nullString(p.HealthStages),
			p.HealthTags, nullString(p.HealthNote), nullString(p.NutritionGoal),
			nullString(p.NutritionGoals), nullablePositiveInt(p.CalorieTarget),
			now,
		},
	}

	updateSelf := Statement{SQL: `UPDATE member_profiles SET
			name = CASE WHEN name IS NULL OR name = '' THEN ? ELSE name END,
			height_cm = ?, weight_kg = ?, age = ?, sex = ?, activity_level = ?,
			target_weight_kg = ?, body_fat_pct = ?, life_stage = ?, allergens = ?,
			health_stages = ?, health_tags = ?, health_note = ?,
			nutrition_goal = ?, nutrition_goals = ?, calorie_target = ?,
			updated_at = ?
		WHERE is_self = 1`,
		Args: []any{
			selfMemberName,
			nullablePositiveInt(p.HeightCm), nullablePositiveInt(p.WeightKg),
			nullablePositiveInt(p.Age), nullString(p.Sex),
			nullString(p.ActivityLevel), nullablePositiveInt(p.TargetWeightKg),
			nullablePositiveFloat(p.BodyFatPct), nullString(p.LifeStage),
			nullString(p.Allergens), nullString(p.HealthStages),
			p.HealthTags, nullString(p.HealthNote), nullString(p.NutritionGoal),
			nullString(p.NutritionGoals), nullablePositiveInt(p.CalorieTarget),
			now,
		},
	}

	return s.Run(ctx, []Statement{profile, insertSelf, updateSelf})
}

// ListMembers reads every member card, self first, then by id.
func (s *Store) ListMembers(ctx context.Context) ([]domain.MemberProfile, error) {
	rows, err := s.Query(ctx, `SELECT `+memberColumns+` FROM member_profiles
		ORDER BY is_self DESC, id`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MemberProfile, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 19 {
			return nil, fmt.Errorf("foodbox: member_profiles row has %d columns", len(v))
		}
		out = append(out, domain.MemberProfile{
			ID:             asInt(v[0]),
			Name:           asString(v[1]),
			Role:           asString(v[2]),
			IsSelf:         asBool(v[3]),
			HeightCm:       int(asInt(v[4])),
			WeightKg:       int(asInt(v[5])),
			Age:            int(asInt(v[6])),
			Sex:            asString(v[7]),
			ActivityLevel:  asString(v[8]),
			TargetWeightKg: int(asInt(v[9])),
			BodyFatPct:     asFloat(v[10]),
			LifeStage:      asString(v[11]),
			Allergens:      asString(v[12]),
			HealthStages:   asString(v[13]),
			HealthTags:     asString(v[14]),
			HealthNote:     asString(v[15]),
			NutritionGoal:  asString(v[16]),
			NutritionGoals: asString(v[17]),
			CalorieTarget:  int(asInt(v[18])),
		})
	}
	return out, nil
}

// CreateMember inserts one non-self member and returns its id.
func (s *Store) CreateMember(ctx context.Context, m domain.MemberProfile) (int64, error) {
	if m.IsSelf {
		return 0, ErrSelfMember
	}
	if m.Role == "" {
		m.Role = "adult"
	}
	res, err := s.db.Execute(ctx, `INSERT INTO member_profiles
		(name, role, is_self, height_cm, weight_kg, age, sex, activity_level,
		 target_weight_kg, body_fat_pct, life_stage, allergens, health_stages,
		 health_tags, health_note, nutrition_goal, nutrition_goals,
		 calorie_target, updated_at)
		VALUES(?,?,0,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.Name, m.Role,
		nullablePositiveInt(m.HeightCm), nullablePositiveInt(m.WeightKg),
		nullablePositiveInt(m.Age), nullString(m.Sex),
		nullString(m.ActivityLevel), nullablePositiveInt(m.TargetWeightKg),
		nullablePositiveFloat(m.BodyFatPct), nullString(m.LifeStage),
		nullString(m.Allergens), nullString(m.HealthStages),
		m.HealthTags, nullString(m.HealthNote), nullString(m.NutritionGoal),
		nullString(m.NutritionGoals), nullablePositiveInt(m.CalorieTarget),
		time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertID, nil
}

// UpdateMember saves one member card. The self card is written through
// SaveSichuProfileWithSelf, so this refuses it to keep a single writer.
func (s *Store) UpdateMember(ctx context.Context, m domain.MemberProfile) error {
	if m.ID == 0 {
		return ErrMemberNotFound
	}
	if m.IsSelf {
		return ErrSelfMember
	}
	res, err := s.db.Execute(ctx, `UPDATE member_profiles SET
			name = ?, role = ?, height_cm = ?, weight_kg = ?, age = ?, sex = ?,
			activity_level = ?, target_weight_kg = ?, body_fat_pct = ?,
			life_stage = ?, allergens = ?, health_stages = ?, health_tags = ?,
			health_note = ?, nutrition_goal = ?, nutrition_goals = ?,
			calorie_target = ?, updated_at = ?
		WHERE id = ? AND is_self = 0`,
		m.Name, m.Role,
		nullablePositiveInt(m.HeightCm), nullablePositiveInt(m.WeightKg),
		nullablePositiveInt(m.Age), nullString(m.Sex),
		nullString(m.ActivityLevel), nullablePositiveInt(m.TargetWeightKg),
		nullablePositiveFloat(m.BodyFatPct), nullString(m.LifeStage),
		nullString(m.Allergens), nullString(m.HealthStages),
		m.HealthTags, nullString(m.HealthNote), nullString(m.NutritionGoal),
		nullString(m.NutritionGoals), nullablePositiveInt(m.CalorieTarget),
		time.Now().Unix(), m.ID)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// DeleteMember removes one non-self member card.
func (s *Store) DeleteMember(ctx context.Context, id int64) error {
	res, err := s.db.Execute(ctx,
		"DELETE FROM member_profiles WHERE id = ? AND is_self = 0", id)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return ErrMemberNotFound
	}
	return nil
}

// nullablePositiveInt maps the questionnaire's "not filled" zero to NULL, so
// an empty field reads back as missing rather than as a real zero.
func nullablePositiveInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullablePositiveFloat(v float64) any {
	if v <= 0 {
		return nil
	}
	return v
}
