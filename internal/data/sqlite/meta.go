package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
)

// MetaGet reads one app_meta value. A missing key returns "" and no error,
// so callers can treat absence as the default.
func (s *Store) MetaGet(ctx context.Context, key string) (string, error) {
	rows, err := s.Query(ctx, "SELECT value FROM app_meta WHERE key = ?", key)
	if err != nil {
		return "", err
	}
	if len(rows.Values) == 0 || len(rows.Values[0]) == 0 {
		return "", nil
	}
	return asString(rows.Values[0][0]), nil
}

// MetaSet writes one app_meta value, replacing it when present.
func (s *Store) MetaSet(ctx context.Context, key, value string) error {
	return s.Execute(ctx,
		"INSERT INTO app_meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value)
}

// MetaDelete removes one key.
func (s *Store) MetaDelete(ctx context.Context, key string) error {
	return s.Execute(ctx, "DELETE FROM app_meta WHERE key = ?", key)
}

// MetaGetJSON decodes a JSON value into dst. A missing key leaves dst
// untouched and reports false.
func (s *Store) MetaGetJSON(ctx context.Context, key string, dst any) (bool, error) {
	raw, err := s.MetaGet(ctx, key)
	if err != nil || raw == "" {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return false, fmt.Errorf("app_meta[%s]: %w", key, err)
	}
	return true, nil
}

// MetaSetJSON encodes v as JSON and stores it.
func (s *Store) MetaSetJSON(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("app_meta[%s]: %w", key, err)
	}
	return s.MetaSet(ctx, key, string(b))
}
