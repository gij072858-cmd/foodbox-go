package sqlite

import (
	"context"
	"errors"
	"fmt"

	msqlite "github.com/egoist/mygo/plugins/sqlite"
)

// SchemaVersion is the newest schema this app knows.
const SchemaVersion = 3

// ErrSchemaTooNew is returned when a database was written by a newer app.
var ErrSchemaTooNew = errors.New("foodbox: database schema is newer than this app")

var schemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS app_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		category TEXT NOT NULL DEFAULT '未分类',
		kind TEXT NOT NULL DEFAULT 'ingredient',
		quantity_mode TEXT NOT NULL DEFAULT 'granular',
		quantity REAL,
		unit TEXT,
		granular_level TEXT,
		stored_at INTEGER,
		shelf_life_days INTEGER,
		expire_at INTEGER,
		storage_location TEXT NOT NULL DEFAULT 'fridge',
		opened INTEGER,
		barcode TEXT,
		image_path TEXT,
		intro_what TEXT,
		intro_how TEXT,
		intro_store TEXT,
		intro_tip TEXT,
		is_custom INTEGER NOT NULL DEFAULT 0,
		is_depleted INTEGER NOT NULL DEFAULT 0,
		note TEXT,
		created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`,
	`CREATE TABLE IF NOT EXISTS recipes (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		cuisine TEXT,
		region TEXT,
		main_category TEXT,
		duration_minutes INTEGER,
		difficulty TEXT NOT NULL DEFAULT 'easy',
		base_servings INTEGER NOT NULL DEFAULT 2,
		season_months TEXT NOT NULL DEFAULT '[]',
		ingredients TEXT NOT NULL DEFAULT '[]',
		intro TEXT,
		version_group TEXT,
		source_platform TEXT,
		source_author TEXT,
		source_url TEXT,
		is_dish INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`,
	`CREATE TABLE IF NOT EXISTS steps (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
		order_index INTEGER NOT NULL,
		instruction TEXT NOT NULL,
		duration_minutes INTEGER,
		resources TEXT NOT NULL DEFAULT '[]',
		safety_tag TEXT NOT NULL DEFAULT 'none',
		photo_paths TEXT NOT NULL DEFAULT '[]',
		note TEXT,
		video_path TEXT,
		completion_cue TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_items_name ON items(name)`,
	`CREATE INDEX IF NOT EXISTS idx_recipes_name ON recipes(name)`,
	`CREATE INDEX IF NOT EXISTS idx_recipes_region ON recipes(region)`,
	`CREATE INDEX IF NOT EXISTS idx_steps_recipe ON steps(recipe_id, order_index)`,
}

// schemaV2 adds the user-generated half of the database (scheduling,
// records, profiles, journals) and the recipe columns the Flutter v19
// schema grew after its first release. v1 stays untouched: append only.
var schemaV2 = []string{
	// Columns appended to recipes in Flutter v7–v12. They are added here
	// rather than in v1 because v1 is released once this milestone lands.
	`ALTER TABLE recipes ADD COLUMN cover_image_path TEXT`,
	`ALTER TABLE recipes ADD COLUMN source TEXT NOT NULL DEFAULT 'official'`,
	`ALTER TABLE recipes ADD COLUMN sharer_name TEXT`,
	`ALTER TABLE recipes ADD COLUMN mood_tag TEXT`,
	`ALTER TABLE recipes ADD COLUMN review_status TEXT NOT NULL DEFAULT 'accepted'`,
	`ALTER TABLE recipes ADD COLUMN shared_at INTEGER`,

	`CREATE TABLE IF NOT EXISTS schedule_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
		added_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
		servings INTEGER NOT NULL DEFAULT 2,
		status TEXT NOT NULL DEFAULT 'pending',
		planned_for INTEGER
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS schedule_items_recipe_unique
		ON schedule_items(recipe_id)`,

	// Records deliberately have no foreign key: a finished meal stays in
	// the history after the dish is edited or removed.
	`CREATE TABLE IF NOT EXISTS records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		recipe_id INTEGER NOT NULL,
		completed_at INTEGER NOT NULL,
		note TEXT,
		xp_gained INTEGER NOT NULL DEFAULT 0,
		session_seconds INTEGER
	)`,
	`CREATE INDEX IF NOT EXISTS idx_records_completed_at ON records(completed_at)`,

	`CREATE TABLE IF NOT EXISTS profiles (
		id INTEGER PRIMARY KEY DEFAULT 1,
		nickname TEXT NOT NULL DEFAULT '食匣用户',
		avatar_path TEXT,
		background_path TEXT,
		xp INTEGER NOT NULL DEFAULT 0,
		total_meals INTEGER NOT NULL DEFAULT 0,
		favorite_recipe_ids TEXT NOT NULL DEFAULT '[]',
		cooked_recipe_ids TEXT NOT NULL DEFAULT '[]',
		wanted_recipe_ids TEXT NOT NULL DEFAULT '[]',
		reminder_mode TEXT NOT NULL DEFAULT 'vibrate'
	)`,

	`CREATE TABLE IF NOT EXISTS user_profiles (
		id INTEGER PRIMARY KEY DEFAULT 1,
		stove_count INTEGER NOT NULL DEFAULT 2,
		cookware TEXT NOT NULL DEFAULT '{}',
		hometown TEXT,
		age_range TEXT,
		spicy TEXT,
		saltiness TEXT,
		restrictions TEXT NOT NULL DEFAULT '',
		allergy_note TEXT,
		household_size INTEGER NOT NULL DEFAULT 1,
		household_mix TEXT NOT NULL DEFAULT '{"adult":1,"kid":0,"elder":0}',
		onboarded INTEGER NOT NULL DEFAULT 0,
		version INTEGER NOT NULL DEFAULT 1,
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`,

	`CREATE TABLE IF NOT EXISTS sichu_profiles (
		id INTEGER PRIMARY KEY DEFAULT 1,
		height_cm INTEGER,
		weight_kg INTEGER,
		age INTEGER,
		sex TEXT,
		activity_level TEXT,
		target_weight_kg INTEGER,
		body_fat_pct REAL,
		life_stage TEXT,
		allergens TEXT,
		health_stages TEXT,
		health_tags TEXT NOT NULL DEFAULT '',
		health_note TEXT,
		nutrition_goal TEXT,
		nutrition_goals TEXT,
		calorie_target INTEGER,
		restrictions TEXT,
		allergy_note TEXT,
		explore_ratio REAL NOT NULL DEFAULT 0.3,
		target_meals TEXT,
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`,

	`CREATE TABLE IF NOT EXISTS member_profiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		role TEXT NOT NULL,
		is_self INTEGER NOT NULL DEFAULT 0,
		height_cm INTEGER,
		weight_kg INTEGER,
		age INTEGER,
		sex TEXT,
		activity_level TEXT,
		target_weight_kg INTEGER,
		body_fat_pct REAL,
		life_stage TEXT,
		allergens TEXT,
		health_tags TEXT NOT NULL DEFAULT '',
		health_stages TEXT,
		health_note TEXT,
		nutrition_goal TEXT,
		nutrition_goals TEXT,
		calorie_target INTEGER,
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`,

	`CREATE TABLE IF NOT EXISTS cook_journal (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date TEXT NOT NULL,
		photo_paths TEXT NOT NULL DEFAULT '[]',
		record_ids TEXT NOT NULL DEFAULT '[]',
		note TEXT,
		created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
		updated_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS cook_journal_date_unique
		ON cook_journal(date)`,

	// Indexes used by the pantry and recipe filters.
	`CREATE INDEX IF NOT EXISTS idx_items_category ON items(category)`,
	`CREATE INDEX IF NOT EXISTS idx_recipes_main_category ON recipes(main_category)`,
	`CREATE INDEX IF NOT EXISTS idx_recipes_cuisine ON recipes(cuisine)`,

	// The Flutter app seeds exactly one account row and one self member.
	`INSERT OR IGNORE INTO profiles(id) VALUES(1)`,
	`INSERT INTO member_profiles(name, role, is_self)
		SELECT '本人', 'adult', 1
		WHERE NOT EXISTS (SELECT 1 FROM member_profiles)`,
}

// schemaV3 gives each queued dish its meal group, so breakfast, lunch and
// dinner reach 掌序 as separate groups instead of one flat queue. Existing
// rows default to the ungrouped table.
var schemaV3 = []string{
	`ALTER TABLE schedule_items ADD COLUMN meal TEXT NOT NULL DEFAULT ''`,
}

// Migrate brings the database to SchemaVersion, in one transaction per
// version. Migrations are append-only: never edit a released version.
func (s *Store) Migrate(ctx context.Context) error {
	version, err := s.version(ctx)
	if err != nil {
		return err
	}
	if version > SchemaVersion {
		return ErrSchemaTooNew
	}
	for version < SchemaVersion {
		from := version
		version++
		stmts, ok := migrations[version]
		if !ok {
			return fmt.Errorf("foodbox: no migration to schema v%d", version)
		}
		batch := make([]msqlite.Statement, 0, len(stmts)+1)
		for _, sql := range stmts {
			batch = append(batch, msqlite.Statement{SQL: sql})
		}
		batch = append(batch, msqlite.Statement{SQL: fmt.Sprintf("PRAGMA user_version = %d", version)})
		if _, err := s.db.Transaction(ctx, batch); err != nil {
			return fmt.Errorf("foodbox: migrate from %d to %d: %w", from, version, err)
		}
	}
	return nil
}

func (s *Store) version(ctx context.Context) (int64, error) {
	rows, err := s.db.Query(ctx, "PRAGMA user_version")
	if err != nil {
		return 0, err
	}
	if len(rows.Values) == 1 && len(rows.Values[0]) == 1 {
		if v, ok := rows.Values[0][0].(int64); ok {
			return v, nil
		}
	}
	return 0, nil
}

// migrations maps a schema version to the statements that bring a database
// from version-1 to it. Released entries are never edited.
var migrations = map[int64][]string{
	1: schemaV1,
	2: schemaV2,
	3: schemaV3,
}
