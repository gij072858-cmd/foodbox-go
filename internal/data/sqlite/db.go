// Package sqlite owns FoodBox's database: it is the only package that
// imports MyGo's SQLite plugin. Pages and services use repositories.
package sqlite

import (
	"context"
	"time"

	msqlite "github.com/egoist/mygo/plugins/sqlite"
)

// Store wraps the MyGo SQLite connection.
type Store struct {
	db *msqlite.DB
}

// Open opens path, creates it if missing, and turns on WAL.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := msqlite.Open(ctx, path, msqlite.OpenOptions{BusyTimeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	// The plugin opens with foreign keys on; WAL is the one pragma it has
	// no option for.
	if _, err := db.Execute(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the connection.
func (s *Store) Close() error { return s.db.Close() }

// DB is the raw connection, for the store's own statements and for tests
// that inspect PRAGMA or seed rows directly. The app never sees it: it talks
// to Repository.
func (s *Store) DB() *msqlite.DB { return s.db }

// QueryResult is a plain result set, so packages outside sqlite can read
// the database without importing the MyGo plugin.
type QueryResult struct {
	Columns []string
	Values  [][]any
}

// Query runs a read-only query.
func (s *Store) Query(ctx context.Context, sql string, args ...any) (QueryResult, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return QueryResult{}, err
	}
	return QueryResult{Columns: rows.Columns, Values: rows.Values}, nil
}

// Execute runs one statement.
func (s *Store) Execute(ctx context.Context, sql string, args ...any) error {
	_, err := s.db.Execute(ctx, sql, args...)
	return err
}

// Statement is one statement of a transaction.
type Statement struct {
	SQL  string
	Args []any
}

// Run executes statements in one transaction.
func (s *Store) Run(ctx context.Context, stmts []Statement) error {
	if len(stmts) == 0 {
		return nil
	}
	batch := make([]msqlite.Statement, len(stmts))
	for i, st := range stmts {
		batch[i] = msqlite.Statement{SQL: st.SQL, Args: st.Args}
	}
	_, err := s.db.Transaction(ctx, batch)
	return err
}
