// Package seed imports the shipped JSON into SQLite on first launch.
package seed

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gij072858-cmd/foodbox-go/internal/assets"
	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// SeedVersion follows FoodBox's kSeedVersion. Bump it when the JSON changes.
const SeedVersion = 18

// Stage is a step of the import, for the startup screen.
type Stage string

const (
	StageRead  Stage = "read"
	StageParse Stage = "parse"
	StageWrite Stage = "write"
	StageDone  Stage = "done"
)

// Progress reports how far the import is.
type Progress struct {
	Stage Stage
	Done  int
	Total int
}

// Summary is what one import did.
type Summary struct {
	Skipped      bool
	Recipes      int
	Steps        int
	CatalogItems int
	SeedVersion  int
}

// Import reads the embedded JSON and writes recipes and steps. It is
// idempotent: if app_meta.seed_version is current it returns Skipped.
func Import(ctx context.Context, store *sqlite.Store, progress func(Progress)) (Summary, error) {
	report := func(p Progress) {
		if progress != nil {
			progress(p)
		}
	}

	report(Progress{Stage: StageRead, Done: 0, Total: 1})
	recipesJSON, err := assets.Data("recipes.json")
	if err != nil {
		return Summary{}, err
	}
	itemsJSON, err := assets.Data("food_items.json")
	if err != nil {
		return Summary{}, err
	}
	report(Progress{Stage: StageRead, Done: 1, Total: 1})

	current, err := metaInt(ctx, store, "seed_version")
	if err != nil {
		return Summary{}, err
	}
	if current >= SeedVersion {
		summary := Summary{Skipped: true, SeedVersion: int(current)}
		summary.Recipes, summary.Steps = counts(ctx, store)
		return summary, nil
	}

	report(Progress{Stage: StageParse, Done: 0, Total: 1})
	var catalog []domain.FoodItem
	if err := json.Unmarshal(itemsJSON, &catalog); err != nil {
		return Summary{}, fmt.Errorf("food_items.json: %w", err)
	}
	recipes, err := parseRecipes(recipesJSON)
	if err != nil {
		return Summary{}, err
	}
	report(Progress{Stage: StageParse, Done: 1, Total: 1})

	stmts := make([]sqlite.Statement, 0, len(recipes)*2+2)
	for i := range recipes {
		r := &recipes[i]
		seasonMonths, _ := json.Marshal(r.SeasonMonths)
		ingredients, _ := json.Marshal(r.Ingredients)
		var platform, author, url string
		if r.Source != nil {
			platform, author, url = r.Source.Platform, r.Source.Author, r.Source.URL
		}
		stmts = append(stmts, sqlite.Statement{
			SQL: `INSERT OR REPLACE INTO recipes
				(id, name, cuisine, region, main_category, duration_minutes, difficulty,
				 base_servings, season_months, ingredients, intro, version_group,
				 source_platform, source_author, source_url, is_dish)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			Args: []any{
				r.ID, r.Name, r.Cuisine, r.Region, r.MainCategory, nullableFloat(r.DurationMinutes), string(r.Difficulty),
				r.BaseServings, string(seasonMonths), string(ingredients), r.Intro, r.VersionGroup,
				platform, author, url, r.IsDish,
			},
		})
		for j := range r.Steps {
			s := &r.Steps[j]
			resources, _ := json.Marshal(s.Resources)
			photos, _ := json.Marshal(s.PhotoPaths)
			stmts = append(stmts, sqlite.Statement{
				SQL: `INSERT INTO steps
					(recipe_id, order_index, instruction, duration_minutes, resources,
					 safety_tag, photo_paths, note, video_path, completion_cue)
					VALUES (?,?,?,?,?,?,?,?,?,?)`,
				Args: []any{
					r.ID, s.OrderIndex, s.Instruction, nullableFloat(s.DurationMinutes), string(resources),
					s.SafetyTag, string(photos), s.Note, s.VideoPath, s.CompletionCue,
				},
			})
		}
		if i%100 == 0 {
			report(Progress{Stage: StageWrite, Done: i, Total: len(recipes)})
		}
	}
	stmts = append(stmts, sqlite.Statement{
		SQL:  "INSERT OR REPLACE INTO app_meta(key, value) VALUES('seed_version', ?)",
		Args: []any{fmt.Sprint(SeedVersion)},
	})

	report(Progress{Stage: StageWrite, Done: 0, Total: len(recipes)})
	if err := store.Run(ctx, stmts); err != nil {
		return Summary{}, fmt.Errorf("seed import: %w", err)
	}
	summary := Summary{
		Recipes:      len(recipes),
		Steps:        countSteps(recipes),
		CatalogItems: len(catalog),
		SeedVersion:  SeedVersion,
	}
	report(Progress{Stage: StageDone, Done: len(recipes), Total: len(recipes)})
	return summary, nil
}

func parseRecipes(data []byte) ([]domain.Recipe, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("recipes.json: %w", err)
	}
	recipes := make([]domain.Recipe, len(raws))
	for i, raw := range raws {
		if err := json.Unmarshal(raw, &recipes[i]); err != nil {
			return nil, fmt.Errorf("recipes.json[%d]: %w", i, err)
		}
		var probe struct {
			IsDish *bool `json:"isDish"`
		}
		if err := json.Unmarshal(raw, &probe); err == nil {
			recipes[i].IsDish = probe.IsDish == nil || *probe.IsDish
		}
		recipes[i].ID = int64(i + 1)
		for j := range recipes[i].Steps {
			recipes[i].Steps[j].RecipeID = recipes[i].ID
		}
	}
	return recipes, nil
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

func countSteps(recipes []domain.Recipe) int {
	n := 0
	for i := range recipes {
		n += len(recipes[i].Steps)
	}
	return n
}

func metaInt(ctx context.Context, store *sqlite.Store, key string) (int64, error) {
	rows, err := store.Query(ctx, "SELECT value FROM app_meta WHERE key = ?", key)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	switch v := rows.Values[0][0].(type) {
	case int64:
		return v, nil
	case string:
		var n int64
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n, nil
		}
	}
	return 0, nil
}

func counts(ctx context.Context, store *sqlite.Store) (int, int) {
	recipes, steps := 0, 0
	if rows, err := store.Query(ctx, "SELECT COUNT(*) FROM recipes"); err == nil && len(rows.Values) > 0 {
		recipes = int(asInt64(rows.Values[0][0]))
	}
	if rows, err := store.Query(ctx, "SELECT COUNT(*) FROM steps"); err == nil && len(rows.Values) > 0 {
		steps = int(asInt64(rows.Values[0][0]))
	}
	return recipes, steps
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}
