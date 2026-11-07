// Package legacy imports a Flutter/Drift FoodBox database once, read-only.
//
// The Flutter app stored the same ten tables in the same SQLite shapes, so
// the import reads each table by column name (never by position), fills
// defaults for columns an older schema lacks, and writes everything in one
// transaction. The source file is opened read-only and never modified.
package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
)

// Summary counts what a legacy database holds.
type Summary struct {
	Path           string
	Items          int
	Recipes        int
	Steps          int
	ScheduleItems  int
	Records        int
	Profiles       int
	UserProfiles   int
	SichuProfiles  int
	MemberProfiles int
	CookJournal    int
	Tables         []string
}

// Empty reports whether the file looks like a FoodBox database at all.
func (s Summary) Empty() bool { return s.Recipes == 0 && s.Items == 0 }

// Counts is the short "N 道菜 · M 样食材" line the settings page shows.
func (s Summary) Counts() string {
	return fmt.Sprintf("%d 道菜 · %d 个步骤 · %d 样食材 · %d 条记录",
		s.Recipes, s.Steps, s.Items, s.Records)
}

// knownTables is the import order's table set. Only tables present in the
// source are replaced; absent ones keep the current data.
var knownTables = []string{
	"items", "recipes", "steps", "schedule_items", "records",
	"profiles", "user_profiles", "sichu_profiles", "member_profiles", "cook_journal",
}

// Inspect reads the legacy file's counts without writing anything.
func Inspect(ctx context.Context, path string) (Summary, error) {
	src, err := sqlite.OpenReadOnly(ctx, path)
	if err != nil {
		return Summary{}, err
	}
	defer src.Close()
	return inspect(ctx, src, path)
}

func inspect(ctx context.Context, src *sqlite.Store, path string) (Summary, error) {
	sum := Summary{Path: path}
	rows, err := src.Query(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
	if err != nil {
		return sum, err
	}
	exists := make(map[string]bool, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) == 0 {
			continue
		}
		name, ok := v[0].(string)
		if !ok {
			continue
		}
		exists[name] = true
	}
	for _, table := range knownTables {
		if !exists[table] {
			continue
		}
		sum.Tables = append(sum.Tables, table)
		n, err := count(ctx, src, table)
		if err != nil {
			return sum, err
		}
		switch table {
		case "items":
			sum.Items = n
		case "recipes":
			sum.Recipes = n
		case "steps":
			sum.Steps = n
		case "schedule_items":
			sum.ScheduleItems = n
		case "records":
			sum.Records = n
		case "profiles":
			sum.Profiles = n
		case "user_profiles":
			sum.UserProfiles = n
		case "sichu_profiles":
			sum.SichuProfiles = n
		case "member_profiles":
			sum.MemberProfiles = n
		case "cook_journal":
			sum.CookJournal = n
		}
	}
	return sum, nil
}

func count(ctx context.Context, src *sqlite.Store, table string) (int, error) {
	rows, err := src.Query(ctx, "SELECT COUNT(*) FROM "+table)
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return 0, nil
	}
	return int(toInt(rows.Values[0][0])), nil
}

// Store is the one call the migration makes: a batch in a transaction.
type Store interface {
	Run(ctx context.Context, stmts []sqlite.Statement) error
}

// Import replaces the ten tables' contents with the legacy file's, in one
// transaction. It reports what it found; a non-FoodBox file is rejected
// before anything is written.
func Import(ctx context.Context, path string, dst Store) (Summary, error) {
	src, err := sqlite.OpenReadOnly(ctx, path)
	if err != nil {
		return Summary{}, err
	}
	defer src.Close()

	sum, err := inspect(ctx, src, path)
	if err != nil {
		return sum, err
	}
	if sum.Empty() {
		return sum, errors.New("不是食匣旧版数据库：找不到 recipes 或 items 表")
	}

	stmts := make([]sqlite.Statement, 0, sum.Recipes+sum.Steps+sum.Items+64)
	for _, table := range knownTables {
		if !hasTable(sum.Tables, table) {
			continue
		}
		rows, err := readTable(ctx, src, table)
		if err != nil {
			return sum, err
		}
		inserts, err := buildInsert(table, rows)
		if err != nil {
			return sum, err
		}
		if len(inserts) == 0 {
			continue
		}
		stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM " + table})
		stmts = append(stmts, inserts...)
	}
	if len(stmts) == 0 {
		return sum, errors.New("旧版数据库里没有可导入的数据")
	}
	stmts = append(stmts, sqlite.Statement{
		SQL:  "INSERT OR REPLACE INTO app_meta(key, value) VALUES('legacy_imported_at', ?)",
		Args: []any{fmt.Sprint(time.Now().Unix())},
	})
	if err := dst.Run(ctx, stmts); err != nil {
		return sum, err
	}
	return sum, nil
}

func hasTable(tables []string, want string) bool {
	for _, t := range tables {
		if t == want {
			return true
		}
	}
	return false
}

// readTable returns every row as column-name -> value, so a schema with
// fewer columns still maps.
func readTable(ctx context.Context, src *sqlite.Store, table string) ([]map[string]any, error) {
	res, err := src.Query(ctx, "SELECT * FROM "+table)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(res.Values))
	for i, vals := range res.Values {
		m := make(map[string]any, len(res.Columns))
		for j, col := range res.Columns {
			if j < len(vals) {
				m[col] = vals[j]
			}
		}
		out[i] = m
	}
	return out, nil
}

// buildInsert maps one table's rows to INSERT statements with our column
// list. Missing columns fall back to the schema's defaults.
func buildInsert(table string, rows []map[string]any) ([]sqlite.Statement, error) {
	switch table {
	case "items":
		return build(rows, `INSERT OR REPLACE INTO items
			(id, name, category, kind, quantity_mode, quantity, unit, granular_level,
			 stored_at, shelf_life_days, expire_at, storage_location, opened, barcode,
			 image_path, intro_what, intro_how, intro_store, intro_tip, is_custom,
			 is_depleted, note, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), str(m, "name"), defStr(m, "category", "未分类"),
					defStr(m, "kind", "ingredient"), defStr(m, "quantity_mode", "granular"),
					fptr(m, "quantity"), sptr(m, "unit"), sptr(m, "granular_level"),
					iptr(m, "stored_at"), iptr(m, "shelf_life_days"), iptr(m, "expire_at"),
					defStr(m, "storage_location", "fridge"), bptr(m, "opened"),
					sptr(m, "barcode"), sptr(m, "image_path"), sptr(m, "intro_what"),
					sptr(m, "intro_how"), sptr(m, "intro_store"), sptr(m, "intro_tip"),
					b(m, "is_custom"), b(m, "is_depleted"), sptr(m, "note"),
					defInt(m, "created_at", time.Now().Unix()), defInt(m, "updated_at", time.Now().Unix()),
				}
			})
	case "recipes":
		return build(rows, `INSERT OR REPLACE INTO recipes
			(id, name, cuisine, region, main_category, duration_minutes, difficulty,
			 base_servings, season_months, ingredients, intro, version_group,
			 source_platform, source_author, source_url, is_dish)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), str(m, "name"), sptr(m, "cuisine"), sptr(m, "region"),
					sptr(m, "main_category"), fptr(m, "duration_minutes"),
					defStr(m, "difficulty", "easy"), defInt(m, "base_servings", 2),
					defJSON(m, "season_months", "[]"), defJSON(m, "ingredients", "[]"),
					sptr(m, "intro"), sptr(m, "version_group"),
					sptr(m, "source_platform"), sptr(m, "source_author"),
					sptr(m, "source_url"), defBool(m, "is_dish", true),
				}
			})
	case "steps":
		return build(rows, `INSERT OR REPLACE INTO steps
			(id, recipe_id, order_index, instruction, duration_minutes, resources,
			 safety_tag, photo_paths, note, video_path, completion_cue)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defInt(m, "recipe_id", 0), defInt(m, "order_index", 0),
					defStr(m, "instruction", ""), fptr(m, "duration_minutes"),
					defJSON(m, "resources", "[]"), defStr(m, "safety_tag", "none"),
					defJSON(m, "photo_paths", "[]"), sptr(m, "note"),
					sptr(m, "video_path"), sptr(m, "completion_cue"),
				}
			})
	case "schedule_items":
		return build(rows, `INSERT OR REPLACE INTO schedule_items
			(id, recipe_id, added_at, servings, status, planned_for)
			VALUES (?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defInt(m, "recipe_id", 0),
					defInt(m, "added_at", time.Now().Unix()),
					defInt(m, "servings", 2), defStr(m, "status", "pending"),
					iptr(m, "planned_for"),
				}
			})
	case "records":
		return build(rows, `INSERT OR REPLACE INTO records
			(id, recipe_id, completed_at, note, xp_gained, session_seconds)
			VALUES (?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defInt(m, "recipe_id", 0),
					defInt(m, "completed_at", time.Now().Unix()),
					sptr(m, "note"), defInt(m, "xp_gained", 10), iptr(m, "session_seconds"),
				}
			})
	case "profiles":
		return build(rows, `INSERT OR REPLACE INTO profiles
			(id, nickname, avatar_path, background_path, xp, total_meals,
			 favorite_recipe_ids, cooked_recipe_ids, wanted_recipe_ids, reminder_mode)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defStr(m, "nickname", "食匣用户"), sptr(m, "avatar_path"),
					sptr(m, "background_path"), defInt(m, "xp", 0),
					defInt(m, "total_meals", 0),
					defJSON(m, "favorite_recipe_ids", "[]"),
					defJSON(m, "cooked_recipe_ids", "[]"),
					defJSON(m, "wanted_recipe_ids", "[]"),
					defStr(m, "reminder_mode", "vibrate"),
				}
			})
	case "user_profiles":
		return build(rows, `INSERT OR REPLACE INTO user_profiles
			(id, stove_count, cookware, hometown, age_range, spicy, saltiness,
			 restrictions, allergy_note, household_size, household_mix, onboarded, version)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defInt(m, "stove_count", 2), defJSON(m, "cookware", "{}"),
					sptr(m, "hometown"), sptr(m, "age_range"), sptr(m, "spicy"),
					sptr(m, "saltiness"), defStr(m, "restrictions", ""),
					sptr(m, "allergy_note"), defInt(m, "household_size", 1),
					defJSON(m, "household_mix", `{"adult":1,"kid":0,"elder":0}`),
					defBool(m, "onboarded", false), defInt(m, "version", 1),
				}
			})
	case "sichu_profiles":
		return build(rows, `INSERT OR REPLACE INTO sichu_profiles
			(id, height_cm, weight_kg, age, sex, activity_level, target_weight_kg,
			 body_fat_pct, life_stage, allergens, health_stages, health_tags,
			 health_note, nutrition_goal, nutrition_goals, calorie_target,
			 restrictions, allergy_note, explore_ratio, target_meals)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), iptr(m, "height_cm"), iptr(m, "weight_kg"), iptr(m, "age"),
					sptr(m, "sex"), sptr(m, "activity_level"), iptr(m, "target_weight_kg"),
					fptr(m, "body_fat_pct"), sptr(m, "life_stage"), sptr(m, "allergens"),
					sptr(m, "health_stages"), defStr(m, "health_tags", ""),
					sptr(m, "health_note"), sptr(m, "nutrition_goal"),
					sptr(m, "nutrition_goals"), iptr(m, "calorie_target"),
					sptr(m, "restrictions"), sptr(m, "allergy_note"),
					defFloat(m, "explore_ratio", 0.3), sptr(m, "target_meals"),
				}
			})
	case "member_profiles":
		return build(rows, `INSERT OR REPLACE INTO member_profiles
			(id, name, role, is_self, height_cm, weight_kg, age, sex, activity_level,
			 target_weight_kg, body_fat_pct, life_stage, allergens, health_tags,
			 health_stages, health_note, nutrition_goal, nutrition_goals, calorie_target)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defStr(m, "name", "本人"), defStr(m, "role", "adult"),
					defBool(m, "is_self", false), iptr(m, "height_cm"), iptr(m, "weight_kg"),
					iptr(m, "age"), sptr(m, "sex"), sptr(m, "activity_level"),
					iptr(m, "target_weight_kg"), fptr(m, "body_fat_pct"),
					sptr(m, "life_stage"), sptr(m, "allergens"),
					defStr(m, "health_tags", ""), sptr(m, "health_stages"),
					sptr(m, "health_note"), sptr(m, "nutrition_goal"),
					sptr(m, "nutrition_goals"), iptr(m, "calorie_target"),
				}
			})
	case "cook_journal":
		return build(rows, `INSERT OR REPLACE INTO cook_journal
			(id, date, photo_paths, record_ids, note, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?)`,
			func(m map[string]any) []any {
				return []any{
					id(m), defStr(m, "date", ""), defJSON(m, "photo_paths", "[]"),
					defJSON(m, "record_ids", "[]"), sptr(m, "note"),
					defInt(m, "created_at", time.Now().Unix()),
					defInt(m, "updated_at", time.Now().Unix()),
				}
			})
	default:
		return nil, fmt.Errorf("legacy: unsupported table %q", table)
	}
}

func build(rows []map[string]any, sql string, args func(map[string]any) []any) ([]sqlite.Statement, error) {
	out := make([]sqlite.Statement, 0, len(rows))
	for _, m := range rows {
		out = append(out, sqlite.Statement{SQL: sql, Args: args(m)})
	}
	return out, nil
}

// Value helpers: the legacy file stores the same primitive types Drift does.

func id(m map[string]any) any {
	if v, ok := m["id"]; ok {
		return toInt(v)
	}
	return nil
}

func str(m map[string]any, key string) string { return toStr(m[key]) }

func defStr(m map[string]any, key, fallback string) string {
	if v := str(m, key); v != "" {
		return v
	}
	return fallback
}

func sptr(m map[string]any, key string) any {
	if v := str(m, key); v != "" {
		return v
	}
	return nil
}

func defInt(m map[string]any, key string, fallback int64) any {
	if _, ok := m[key]; !ok || m[key] == nil {
		return fallback
	}
	return toInt(m[key])
}

func defFloat(m map[string]any, key string, fallback float64) any {
	if _, ok := m[key]; !ok || m[key] == nil {
		return fallback
	}
	return toFloat(m[key])
}

func iptr(m map[string]any, key string) any {
	if _, ok := m[key]; !ok || m[key] == nil {
		return nil
	}
	return toInt(m[key])
}

func fptr(m map[string]any, key string) any {
	if _, ok := m[key]; !ok || m[key] == nil {
		return nil
	}
	return toFloat(m[key])
}

func b(m map[string]any, key string) bool {
	if _, ok := m[key]; !ok || m[key] == nil {
		return false
	}
	return toInt(m[key]) != 0
}

func bptr(m map[string]any, key string) any {
	if _, ok := m[key]; !ok || m[key] == nil {
		return nil
	}
	return b(m, key)
}

func defBool(m map[string]any, key string, fallback bool) any {
	if _, ok := m[key]; !ok || m[key] == nil {
		if fallback {
			return 1
		}
		return 0
	}
	if b(m, key) {
		return 1
	}
	return 0
}

// defJSON keeps a NOT NULL JSON column valid: a legacy value that is already
// JSON text passes through, anything else becomes the default.
func defJSON(m map[string]any, key, fallback string) string {
	raw := str(m, key)
	if raw == "" {
		return fallback
	}
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return fallback
	}
	return raw
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case bool:
		if n {
			return 1
		}
	case string:
		var out int64
		if _, err := fmt.Sscan(strings.TrimSpace(n), &out); err == nil {
			return out
		}
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case string:
		var out float64
		if _, err := fmt.Sscan(strings.TrimSpace(n), &out); err == nil {
			return out
		}
	}
	return 0
}

func toStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case int64:
		return fmt.Sprint(s)
	case float64:
		return fmt.Sprint(s)
	}
	return ""
}
