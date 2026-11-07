package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	msqlite "github.com/egoist/mygo/plugins/sqlite"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// The recipe write path. A dish and its steps are one thing: creating or
// editing a recipe replaces its steps in the same transaction, so a reader
// never sees a half-saved dish. The steps table's foreign key cascades a
// delete, so removing a recipe removes its steps with it.

// recipeColumns is the read projection, matching scanRecipe's order.
const recipeColumns = `id, name, cuisine, region, main_category, duration_minutes,
	difficulty, base_servings, season_months, ingredients, intro, version_group,
	source_platform, source_author, source_url, is_dish`

const insertRecipeSQL = `INSERT INTO recipes
	(name, cuisine, region, main_category, duration_minutes, difficulty,
	 base_servings, season_months, ingredients, intro, version_group,
	 source_platform, source_author, source_url, is_dish, created_at, updated_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const updateRecipeSQL = `UPDATE recipes SET
	name = ?, cuisine = ?, region = ?, main_category = ?, duration_minutes = ?,
	difficulty = ?, base_servings = ?, season_months = ?, ingredients = ?,
	intro = ?, version_group = ?, source_platform = ?, source_author = ?,
	source_url = ?, is_dish = ?, updated_at = ?
	WHERE id = ?`

const deleteRecipeSQL = `DELETE FROM recipes WHERE id = ?`

const deleteStepsSQL = `DELETE FROM steps WHERE recipe_id = ?`

const stepColumns = `id, recipe_id, order_index, instruction, duration_minutes,
	resources, safety_tag, photo_paths, note, video_path, completion_cue`

// insertStepSQL inserts one step with an explicit recipe id.
const insertStepSQL = `INSERT INTO steps
	(recipe_id, order_index, instruction, duration_minutes, resources,
	 safety_tag, photo_paths, note, video_path, completion_cue)
	VALUES (?,?,?,?,?,?,?,?,?,?)`

// insertStepForNewRecipeSQL inserts one step into the recipe written just
// before it in the same transaction. The recipe's rowid is the largest one
// in the table, because SQLite assigns max(rowid)+1 to a fresh row and the
// transaction holds the write lock; last_insert_rowid() cannot be used here,
// as each inserted step would replace it for the next one.
const insertStepForNewRecipeSQL = `INSERT INTO steps
	(recipe_id, order_index, instruction, duration_minutes, resources,
	 safety_tag, photo_paths, note, video_path, completion_cue)
	SELECT (SELECT max(id) FROM recipes), ?,?,?,?,?,?,?,?,?`

// CreateRecipe inserts rec with its steps in one transaction and returns the
// new id. rec.ID is ignored; rec.Steps may be nil.
func (s *Store) CreateRecipe(ctx context.Context, rec domain.Recipe) (int64, error) {
	now := time.Now().Unix()
	stmts := make([]msqlite.Statement, 0, len(rec.Steps)+1)
	stmts = append(stmts, msqlite.Statement{SQL: insertRecipeSQL, Args: recipeInsertArgs(rec, now)})
	for i, st := range rec.Steps {
		stmts = append(stmts, msqlite.Statement{
			SQL:  insertStepForNewRecipeSQL,
			Args: stepValueArgs(i, st),
		})
	}
	results, err := s.db.Transaction(ctx, stmts)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, fmt.Errorf("foodbox: create recipe wrote no row")
	}
	return results[0].LastInsertID, nil
}

// UpdateRecipe writes rec's editable columns. When rec.Steps is non-nil its
// steps are replaced in the same transaction; nil leaves the stored steps
// alone, so a caller that only edits the dish does not erase its steps.
func (s *Store) UpdateRecipe(ctx context.Context, rec domain.Recipe) error {
	now := time.Now().Unix()
	stmts := make([]msqlite.Statement, 0, len(rec.Steps)+2)
	stmts = append(stmts, msqlite.Statement{SQL: updateRecipeSQL, Args: recipeUpdateArgs(rec, now)})
	if rec.Steps != nil {
		stmts = append(stmts, msqlite.Statement{SQL: deleteStepsSQL, Args: []any{rec.ID}})
		for i, st := range rec.Steps {
			stmts = append(stmts, msqlite.Statement{SQL: insertStepSQL, Args: stepInsertArgs(rec.ID, i, st)})
		}
	}
	results, err := s.db.Transaction(ctx, stmts)
	if err != nil {
		return err
	}
	if len(results) > 0 && results[0].Changes == 0 {
		return fmt.Errorf("foodbox: recipe %d not found", rec.ID)
	}
	return nil
}

// DeleteRecipe removes a recipe; the steps foreign key cascades. Records of
// past meals deliberately survive (records carry no foreign key).
func (s *Store) DeleteRecipe(ctx context.Context, id int64) error {
	return s.Execute(ctx, deleteRecipeSQL, id)
}

// ReplaceSteps replaces a recipe's steps in one transaction.
func (s *Store) ReplaceSteps(ctx context.Context, recipeID int64, steps []domain.Step) error {
	stmts := make([]Statement, 0, len(steps)+1)
	stmts = append(stmts, Statement{SQL: deleteStepsSQL, Args: []any{recipeID}})
	for i, st := range steps {
		stmts = append(stmts, Statement{SQL: insertStepSQL, Args: stepInsertArgs(recipeID, i, st)})
	}
	return s.Run(ctx, stmts)
}

// RecipeByID reads one recipe together with its steps.
func (s *Store) RecipeByID(ctx context.Context, id int64) (domain.Recipe, error) {
	rows, err := s.Query(ctx, "SELECT "+recipeColumns+" FROM recipes WHERE id = ?", id)
	if err != nil {
		return domain.Recipe{}, err
	}
	if len(rows.Values) == 0 {
		return domain.Recipe{}, fmt.Errorf("foodbox: recipe %d not found", id)
	}
	rec, err := scanRecipe(rows.Values[0])
	if err != nil {
		return domain.Recipe{}, err
	}
	steps, err := s.stepsByRecipe(ctx, id)
	if err != nil {
		return domain.Recipe{}, err
	}
	rec.Steps = steps
	return rec, nil
}

// RecipesByVersionGroup reads every dish in one version group, in seed
// order and without steps.
func (s *Store) RecipesByVersionGroup(ctx context.Context, group string) ([]domain.Recipe, error) {
	rows, err := s.Query(ctx,
		"SELECT "+recipeColumns+" FROM recipes WHERE version_group = ? ORDER BY id", group)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recipe, 0, len(rows.Values))
	for _, v := range rows.Values {
		rec, err := scanRecipe(v)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (s *Store) stepsByRecipe(ctx context.Context, recipeID int64) ([]domain.Step, error) {
	rows, err := s.Query(ctx,
		"SELECT "+stepColumns+" FROM steps WHERE recipe_id = ? ORDER BY order_index, id", recipeID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Step, 0, len(rows.Values))
	for _, v := range rows.Values {
		st, err := scanStep(v)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func scanRecipe(v []any) (domain.Recipe, error) {
	if len(v) < 16 {
		return domain.Recipe{}, fmt.Errorf("foodbox: recipes row has %d columns", len(v))
	}
	rec := domain.Recipe{
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
	_ = json.Unmarshal([]byte(asString(v[8])), &rec.SeasonMonths)
	_ = json.Unmarshal([]byte(asString(v[9])), &rec.Ingredients)
	platform, author, url := asString(v[12]), asString(v[13]), asString(v[14])
	if platform != "" || author != "" || url != "" {
		rec.Source = &domain.RecipeSource{Platform: platform, Author: author, URL: url}
	}
	return rec, nil
}

func scanStep(v []any) (domain.Step, error) {
	if len(v) < 11 {
		return domain.Step{}, fmt.Errorf("foodbox: steps row has %d columns", len(v))
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
	return st, nil
}

func recipeInsertArgs(rec domain.Recipe, now int64) []any {
	return []any{
		rec.Name, nullString(rec.Cuisine), nullString(rec.Region),
		nullString(rec.MainCategory), nullableFloat(rec.DurationMinutes),
		nullString(string(rec.Difficulty)), rec.BaseServings,
		jsonInts(rec.SeasonMonths), jsonIngredients(rec.Ingredients),
		nullString(rec.Intro), nullString(rec.VersionGroup),
		nullString(sourceField(rec, func(s *domain.RecipeSource) string { return s.Platform })),
		nullString(sourceField(rec, func(s *domain.RecipeSource) string { return s.Author })),
		nullString(sourceField(rec, func(s *domain.RecipeSource) string { return s.URL })),
		rec.IsDish, now, now,
	}
}

func recipeUpdateArgs(rec domain.Recipe, now int64) []any {
	args := recipeInsertArgs(rec, now)[:15]
	args = append(args, now, rec.ID)
	return args
}

// stepInsertArgs builds the values of one step, including its recipe id.
func stepInsertArgs(recipeID int64, orderIndex int, st domain.Step) []any {
	return append([]any{recipeID}, stepValueArgs(orderIndex, st)...)
}

// stepValueArgs builds a step's columns except its recipe id. orderIndex is
// the step's position; a caller's own OrderIndex is ignored so stored steps
// always read back in the order the editor showed them.
func stepValueArgs(orderIndex int, st domain.Step) []any {
	safety := st.SafetyTag
	if safety == "" {
		safety = "none"
	}
	return []any{
		orderIndex + 1, st.Instruction, nullableFloat(st.DurationMinutes),
		jsonStrings(st.Resources), safety, jsonStrings(st.PhotoPaths),
		nullString(st.Note), nullString(st.VideoPath), nullString(st.CompletionCue),
	}
}

// jsonInts stores a month list as compact JSON; nil becomes [].
func jsonInts(v []int) string {
	if len(v) == 0 {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// jsonStrings stores a string list as compact JSON; nil becomes [].
func jsonStrings(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// jsonIngredients stores the ingredient list, the same shape the seed wrote.
func jsonIngredients(v []domain.Ingredient) string {
	if len(v) == 0 {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// sourceField reads one attribution column, "" when there is no source.
func sourceField(rec domain.Recipe, get func(*domain.RecipeSource) string) string {
	if rec.Source == nil {
		return ""
	}
	return get(rec.Source)
}
