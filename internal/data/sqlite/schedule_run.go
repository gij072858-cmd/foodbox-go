package sqlite

import (
	"context"

	"github.com/gij072858-cmd/foodbox-go/internal/domain"
)

// runSessionKey is the app_meta key the run session lives under. app_meta is
// not part of a backup: a pending run is device state, not user data.
const runSessionKey = "schedule.run"

// SaveRunSession persists a cooking run's progress so closing the app mid-run
// does not lose the place. An empty session clears the key.
func (s *Store) SaveRunSession(ctx context.Context, session domain.RunSession) error {
	if session.Empty() {
		return s.ClearRunSession(ctx)
	}
	return s.MetaSetJSON(ctx, runSessionKey, session)
}

// LoadRunSession reads the persisted run progress. A missing or empty session
// reports false.
func (s *Store) LoadRunSession(ctx context.Context) (domain.RunSession, bool, error) {
	var session domain.RunSession
	ok, err := s.MetaGetJSON(ctx, runSessionKey, &session)
	if err != nil {
		return domain.RunSession{}, false, err
	}
	if !ok || session.Empty() {
		return domain.RunSession{}, false, nil
	}
	if session.Step < 0 {
		session.Step = 0
	}
	if session.PausedFor < 0 {
		session.PausedFor = 0
	}
	return session, true, nil
}

// ClearRunSession drops the run progress: the run finished, was settled, or
// was abandoned.
func (s *Store) ClearRunSession(ctx context.Context) error {
	return s.MetaDelete(ctx, runSessionKey)
}
