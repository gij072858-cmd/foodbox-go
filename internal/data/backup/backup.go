// Package backup writes and reads FoodBox's portable JSON snapshot.
//
// Format 3 keeps Flutter's v2 envelope (camelCase table keys, rows keyed by
// camelCase column names) so a file can move between both apps. Rows are
// mapped by column name on import, not by position: an older file may lack
// columns, and those are filled by the schema defaults.
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gij072858-cmd/foodbox-go/internal/data/sqlite"
)

// Store is the two calls a backup makes: read a table and write one batch
// in a transaction. The SQLite store and the app's Repository both have
// them, so the backup never needs the concrete connection.
type Store interface {
	Query(ctx context.Context, sql string, args ...any) (sqlite.QueryResult, error)
	Run(ctx context.Context, stmts []sqlite.Statement) error
}

// AppTag identifies a FoodBox backup.
const AppTag = "foodbox"

// FormatVersion is this writer's format. Flutter writes 2; we read both.
const FormatVersion = 3

// CompatSchemaVersion is the Flutter schema level this project mirrors.
// A file newer than this may contain tables or columns we do not know.
const CompatSchemaVersion = 19

// MinSchemaVersion is the oldest Flutter backup the importer accepts.
const MinSchemaVersion = 12

// tableKeys are the backup's table keys, in the order Flutter writes them.
var tableKeys = []string{
	"items", "recipes", "steps", "scheduleItems", "records",
	"profiles", "userProfiles", "sichuProfiles", "memberProfiles", "cookJournal",
}

// tableNames maps a backup key to a SQLite table.
var tableNames = map[string]string{
	"items":          "items",
	"recipes":        "recipes",
	"steps":          "steps",
	"scheduleItems":  "schedule_items",
	"records":        "records",
	"profiles":       "profiles",
	"userProfiles":   "user_profiles",
	"sichuProfiles":  "sichu_profiles",
	"memberProfiles": "member_profiles",
	"cookJournal":    "cook_journal",
}

// insertOrder is parent-before-child, so foreign keys hold.
var insertOrder = []string{
	"items", "recipes", "profiles", "userProfiles", "sichuProfiles",
	"memberProfiles", "steps", "scheduleItems", "records", "cookJournal",
}

// clearOrder is child-before-parent.
var clearOrder = []string{
	"steps", "scheduleItems", "records", "cookJournal", "recipes",
	"memberProfiles", "sichuProfiles", "userProfiles", "profiles", "items",
}

// Snapshot is one backup file.
type Snapshot struct {
	App           string                      `json:"app"`
	FormatVersion int                         `json:"formatVersion"`
	SchemaVersion int                         `json:"schemaVersion"`
	EngineVersion int                         `json:"engineSchemaVersion,omitempty"`
	AppVersion    string                      `json:"appVersion,omitempty"`
	ExportedAt    time.Time                   `json:"exportedAt"`
	Counts        map[string]int              `json:"counts"`
	Tables        map[string][]map[string]any `json:"tables"`
}

// ErrTooNew is returned for a backup written by a newer application.
var ErrTooNew = errors.New("foodbox: backup schema is newer than this app")

// ErrNotFoodBox is returned for a JSON file that is not a FoodBox backup.
var ErrNotFoodBox = errors.New("foodbox: not a FoodBox backup")

// Export reads every backed-up table. app_meta (seed version, AI usage) is
// deliberately not included: it is device state, and never a secret store.
func Export(ctx context.Context, store Store, appVersion string) (Snapshot, error) {
	snap := Snapshot{
		App:           AppTag,
		FormatVersion: FormatVersion,
		SchemaVersion: CompatSchemaVersion,
		EngineVersion: sqlite.SchemaVersion,
		AppVersion:    appVersion,
		ExportedAt:    time.Now().UTC(),
		Counts:        make(map[string]int, len(tableKeys)),
		Tables:        make(map[string][]map[string]any, len(tableKeys)),
	}
	for _, key := range tableKeys {
		rows, err := store.Query(ctx, "SELECT * FROM "+tableNames[key])
		if err != nil {
			return Snapshot{}, fmt.Errorf("backup: read %s: %w", key, err)
		}
		out := make([]map[string]any, 0, len(rows.Values))
		for _, values := range rows.Values {
			row := make(map[string]any, len(rows.Columns))
			for i, col := range rows.Columns {
				if i >= len(values) {
					break
				}
				row[camel(col)] = exportValue(tableNames[key], col, values[i])
			}
			out = append(out, row)
		}
		snap.Tables[key] = out
		snap.Counts[key] = len(out)
	}
	return snap, nil
}

// WriteFile writes the snapshot atomically: a .tmp beside the target, then
// rename, so a crash never leaves a half-written backup at the real path.
func WriteFile(path string, snap Snapshot) error {
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ReadFile parses and validates a backup file.
func ReadFile(path string) (Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	return Decode(b)
}

// Decode parses and validates a backup.
func Decode(b []byte) (Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("backup: parse: %w", err)
	}
	if snap.App != AppTag {
		return Snapshot{}, ErrNotFoodBox
	}
	if snap.SchemaVersion == 0 {
		snap.SchemaVersion = MinSchemaVersion // format 1 files predate the field
	}
	if snap.SchemaVersion > CompatSchemaVersion {
		return Snapshot{}, ErrTooNew
	}
	if snap.SchemaVersion < MinSchemaVersion {
		return Snapshot{}, fmt.Errorf("foodbox: backup schema %d is too old (minimum %d)", snap.SchemaVersion, MinSchemaVersion)
	}
	if snap.FormatVersion == 0 {
		snap.FormatVersion = 1
	}
	if snap.FormatVersion > FormatVersion {
		return Snapshot{}, ErrTooNew
	}
	// Format 1 had eight tables; missing tables are restored as empty.
	if snap.Tables == nil {
		snap.Tables = make(map[string][]map[string]any)
	}
	return snap, nil
}

// Import replaces every backed-up table in one transaction. It is the
// destructive half of a restore: callers export the current database
// first, then call Import, then show the result.
func Import(ctx context.Context, store Store, snap Snapshot) error {
	if snap.App != AppTag {
		return ErrNotFoodBox
	}
	if snap.SchemaVersion > CompatSchemaVersion {
		return ErrTooNew
	}
	stmts := make([]sqlite.Statement, 0, 64)
	for _, key := range clearOrder {
		stmts = append(stmts, sqlite.Statement{SQL: "DELETE FROM " + tableNames[key]})
	}
	for _, key := range insertOrder {
		rows := snap.Tables[key]
		if len(rows) == 0 {
			continue
		}
		table := tableNames[key]
		columns, err := tableColumns(ctx, store, table)
		if err != nil {
			return err
		}
		for _, row := range rows {
			cols := make([]string, 0, len(row))
			args := make([]any, 0, len(row))
			for _, col := range columns {
				v, ok := row[camel(col)]
				if !ok {
					continue // older file: let the schema default fill it
				}
				cols = append(cols, `"`+col+`"`)
				args = append(args, importValue(table, col, v))
			}
			if len(cols) == 0 {
				continue
			}
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
			stmts = append(stmts, sqlite.Statement{
				SQL:  fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(cols, ","), placeholders),
				Args: args,
			})
		}
	}
	return store.Run(ctx, stmts)
}

func tableColumns(ctx context.Context, store Store, table string) ([]string, error) {
	rows, err := store.Query(ctx, `PRAGMA table_info("`+table+`")`)
	if err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(rows.Values))
	for _, v := range rows.Values {
		if len(v) > 1 {
			if name, ok := v[1].(string); ok {
				cols = append(cols, name)
			}
		}
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("foodbox: backup table %q has no columns", table)
	}
	return cols, nil
}

// camel turns source_platform into sourcePlatform.
func camel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

var boolColumns = map[string]bool{
	"items.opened":            true,
	"items.is_custom":         true,
	"items.is_depleted":       true,
	"recipes.is_dish":         true,
	"user_profiles.onboarded": true,
	"member_profiles.is_self": true,
}

var jsonColumns = map[string]bool{
	"recipes.season_months":           true,
	"recipes.ingredients":             true,
	"steps.resources":                 true,
	"steps.photo_paths":               true,
	"profiles.favorite_recipe_ids":    true,
	"profiles.cooked_recipe_ids":      true,
	"profiles.wanted_recipe_ids":      true,
	"user_profiles.cookware":          true,
	"user_profiles.household_mix":     true,
	"user_profiles.restrictions":      true,
	"sichu_profiles.allergens":        true,
	"sichu_profiles.health_stages":    true,
	"sichu_profiles.health_tags":      true,
	"sichu_profiles.nutrition_goals":  true,
	"sichu_profiles.target_meals":     true,
	"sichu_profiles.restrictions":     true,
	"member_profiles.allergens":       true,
	"member_profiles.health_stages":   true,
	"member_profiles.health_tags":     true,
	"member_profiles.nutrition_goals": true,
	"cook_journal.photo_paths":        true,
	"cook_journal.record_ids":         true,
}

func key(table, col string) string { return table + "." + col }

func isTimeColumn(col string) bool {
	if col == "date" {
		return false
	}
	return col == "planned_for" || strings.HasSuffix(col, "_at")
}

func exportValue(table, col string, v any) any {
	if v == nil {
		return nil
	}
	switch {
	case boolColumns[key(table, col)]:
		return asInt(v) != 0
	case isTimeColumn(col):
		if n, ok := asIntOK(v); ok {
			return time.Unix(n, 0).UTC().Format(time.RFC3339)
		}
		return v
	case jsonColumns[key(table, col)]:
		if s, ok := v.(string); ok {
			var decoded any
			if err := json.Unmarshal([]byte(s), &decoded); err == nil {
				return decoded
			}
		}
		return v
	}
	return v
}

func importValue(table, col string, v any) any {
	if v == nil {
		// Flutter's v19 schema stores is_dish NULL as "it is a dish"; this
		// project's v1 column is NOT NULL DEFAULT 1, so map the meaning.
		if table == "recipes" && col == "is_dish" {
			return int64(1)
		}
		return nil
	}
	switch {
	case boolColumns[key(table, col)]:
		if b, ok := v.(bool); ok {
			if b {
				return int64(1)
			}
			return int64(0)
		}
		n, _ := asIntOK(v)
		return n
	case isTimeColumn(col):
		if t, ok := asTime(v); ok {
			return t.Unix()
		}
		return v
	case jsonColumns[key(table, col)]:
		if s, ok := v.(string); ok {
			return s
		}
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
	if n, ok := asIntOK(v); ok {
		return n
	}
	return v
}

func asTime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t.UTC(), true
			}
		}
	case int64:
		return unixValue(x), true
	case float64:
		return unixValue(int64(x)), true
	}
	return time.Time{}, false
}

func unixValue(n int64) time.Time {
	// Drift's default serializer uses milliseconds; older files may use
	// seconds. 1e11 seconds is far in the future, so it is unambiguous.
	if n > 1_000_000_000_000 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

func asInt(v any) int64 {
	n, _ := asIntOK(v)
	return n
}

func asIntOK(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		if x == float64(int64(x)) {
			return int64(x), true
		}
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		return n, err == nil
	}
	return 0, false
}
