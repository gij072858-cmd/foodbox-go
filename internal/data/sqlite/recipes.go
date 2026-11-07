package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// ListAll reads every recipe, without steps.
func (s *Store) ListAll(ctx context.Context) ([]domain.Recipe, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, cuisine, region, main_category,
		duration_minutes, difficulty, base_servings, season_months, ingredients,
		intro, version_group, source_platform, source_author, source_url, is_dish
		FROM recipes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recipe, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) < 16 {
			return nil, fmt.Errorf("foodbox: recipes row has %d columns", len(v))
		}
		r := domain.Recipe{
			ID:              asInt(v[0]),
			Name:            asString(v[1]),
			Cuisine:         asString(v[2]),
			Region:          asString(v[3]),
			MainCategory:    asString(v[4]),
			DurationMinutes: asFloatPtr(v[5]),
			Difficulty:      domain.Difficulty(asString(v[6])),
			BaseServings:    int(asInt(v[7])),
			Intro:           asString(v[10]),
			VersionGroup:    asString(v[11]),
			IsDish:          asBool(v[15]),
		}
		_ = json.Unmarshal([]byte(asString(v[8])), &r.SeasonMonths)
		_ = json.Unmarshal([]byte(asString(v[9])), &r.Ingredients)
		platform, author, url := asString(v[12]), asString(v[13]), asString(v[14])
		if platform != "" || author != "" || url != "" {
			r.Source = &domain.RecipeSource{Platform: platform, Author: author, URL: url}
		}
		out = append(out, r)
	}
	return out, nil
}

// AllSteps reads every step, grouped by recipe.
func (s *Store) AllSteps(ctx context.Context) (map[int64][]domain.Step, error) {
	rows, err := s.db.Query(ctx, `SELECT id, recipe_id, order_index, instruction,
		duration_minutes, resources, safety_tag, photo_paths, note, video_path, completion_cue
		FROM steps ORDER BY recipe_id, order_index`)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]domain.Step, 944)
	for _, v := range rows.Values {
		if len(v) < 11 {
			return nil, fmt.Errorf("foodbox: steps row has %d columns", len(v))
		}
		st := domain.Step{
			ID:              asInt(v[0]),
			RecipeID:        asInt(v[1]),
			OrderIndex:      int(asInt(v[2])),
			Instruction:     asString(v[3]),
			DurationMinutes: asFloatPtr(v[4]),
			SafetyTag:       asString(v[6]),
			Note:            asString(v[8]),
			VideoPath:       asString(v[9]),
			CompletionCue:   asString(v[10]),
		}
		_ = json.Unmarshal([]byte(asString(v[5])), &st.Resources)
		_ = json.Unmarshal([]byte(asString(v[7])), &st.PhotoPaths)
		out[st.RecipeID] = append(out[st.RecipeID], st)
	}
	return out, nil
}

// RecipeCount returns the number of recipes.
func (s *Store) RecipeCount(ctx context.Context) (int, error) {
	rows, err := s.db.Query(ctx, "SELECT COUNT(*) FROM recipes")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	return int(asInt(rows.Values[0][0])), nil
}
