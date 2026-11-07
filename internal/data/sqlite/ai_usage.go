package sqlite

import "context"

// AIUsageKey is the app_meta key holding the daily AI request counter.
//
// It lives in app_meta on purpose: the counter is local bookkeeping, not user
// data, so it stays out of backups along with every other app_meta key. The
// value is a small JSON document the services/ai package owns; this file only
// stores the opaque text, so the quota rule has one implementation.
const AIUsageKey = "ai.usage"

// GetAIUsageJSON returns the raw stored usage JSON, or "" when the counter has
// never been written.
func (s *Store) GetAIUsageJSON(ctx context.Context) (string, error) {
	return s.MetaGet(ctx, AIUsageKey)
}

// SaveAIUsageJSON writes the raw usage JSON. It never holds an API key.
func (s *Store) SaveAIUsageJSON(ctx context.Context, raw string) error {
	return s.MetaSet(ctx, AIUsageKey, raw)
}

// ClearAIUsage drops the counter, e.g. after the user clears the key.
func (s *Store) ClearAIUsage(ctx context.Context) error {
	return s.MetaDelete(ctx, AIUsageKey)
}
