//go:build !js

// The database Stores (Spec code-central-1032): one shared schema of
// document rows, which SQLite holds today and Postgres will, behind a
// small dialect. Each record is the JSON document of its file shape
// (jsondoc.go), beside the columns the queries filter and order on, and
// side tables hold the multi-valued filters. A Ticket keeps its head and
// each History part in a column of its own, so a read selects only the
// parts it names, and its derived head fields in another, worked out on
// every write.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/tomoram/code-central/usage"
)

// dialect is what differs between the database engines: the column type
// of a document, how a row numbers itself, and the placeholders.
type dialect struct {
	// json is the column type a document is kept in.
	json string
	// serial is the column definition of a self-numbering key.
	serial string
	// rebind turns a statement's ? placeholders into the engine's.
	rebind func(string) string
	// begin opens an Update's transaction, taking the engine's write lock
	// at once.
	begin string
}

// querier runs statements: a connection, a transaction or the pool.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLStore is a Store kept in a database: SQLite today (OpenSQLite).
// View and Update share one process-wide read-write mutex, as the other
// Kinds do; an Update runs in one transaction, which a failed Update rolls
// back whole, so it leaves nothing.
type SQLStore struct {
	db *sql.DB
	d  dialect
	mu sync.RWMutex
	// path is where the Store is, for messages and reopening.
	path  string
	lock  *storeLock
	usage usage.Store
}

// Path is where the database is.
func (s *SQLStore) Path() string { return s.path }

// Migrated is the migration Open ran: none yet, a database Store at an
// older format is refused until its Migrations land (Ticket
// code-central-1047).
func (s *SQLStore) Migrated() *Migration { return nil }

// Close closes the database and releases the Store's lock.
func (s *SQLStore) Close() error {
	var errs []error
	if s.db != nil {
		errs = append(errs, s.db.Close())
		s.db = nil
	}
	if s.lock != nil {
		errs = append(errs, s.lock.release())
		s.lock = nil
	}
	return errors.Join(errs...)
}

// Usage is the Store's Usage (ADR 0066): a table written on its own
// connection, outside any Update.
func (s *SQLStore) Usage() usage.Store { return s.usage }

// Changes never delivers: no other service shares a SQLite Store.
func (s *SQLStore) Changes(func(Change)) {}

// View runs fn over a Reader on one of the pool's connections. A read
// that failed inside fn, which the Reader can't report, is its error.
func (s *SQLStore) View(fn func(Reader) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var failed error
	if err := fn(sqlReader{q: conn, d: s.d, err: &failed}); err != nil {
		return err
	}
	return failed
}

// Update runs fn over a Writer in one transaction under the store-wide
// lock: its write and side-table rows land together, or, when fn fails
// or panics, not at all.
func (s *SQLStore) Update(fn func(Writer) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, s.d.begin); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			// Rolled back before the connection goes back to the pool,
			// whatever fn did, so no later Update finds it mid-transaction.
			conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	var failed error
	w := &sqlWriter{sqlReader: sqlReader{q: conn, d: s.d, err: &failed}}
	err = fn(w)
	if err == nil {
		err = failed
	}
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

// schema is the tables and indexes, in the dialect's types. Every
// statement is idempotent, so opening an existing database runs them
// harmlessly.
func (d dialect) schema() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS projects (key TEXT PRIMARY KEY, doc ` + d.json + ` NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS options (id INTEGER PRIMARY KEY, doc ` + d.json + ` NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ci_runs (project TEXT PRIMARY KEY, doc ` + d.json + ` NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS specs (project TEXT NOT NULL, number INTEGER NOT NULL, created BIGINT NOT NULL, updated BIGINT NOT NULL,
			link_integration TEXT NOT NULL, link_id TEXT NOT NULL, doc ` + d.json + ` NOT NULL, PRIMARY KEY (project, number))`,
		`CREATE INDEX IF NOT EXISTS specs_link ON specs (project, link_integration, link_id)`,
		`CREATE TABLE IF NOT EXISTS tickets (project TEXT NOT NULL, number INTEGER NOT NULL, state TEXT NOT NULL, spec INTEGER NOT NULL,
			created BIGINT NOT NULL, updated BIGINT NOT NULL, link_integration TEXT NOT NULL, link_id TEXT NOT NULL,
			doc ` + d.json + ` NOT NULL, head ` + d.json + ` NOT NULL, claims ` + d.json + ` NOT NULL, rounds ` + d.json + ` NOT NULL,
			comments ` + d.json + ` NOT NULL, moves ` + d.json + ` NOT NULL, edits ` + d.json + ` NOT NULL, failure_reports ` + d.json + ` NOT NULL,
			PRIMARY KEY (project, number))`,
		`CREATE INDEX IF NOT EXISTS tickets_state ON tickets (project, state)`,
		`CREATE INDEX IF NOT EXISTS tickets_spec ON tickets (project, spec)`,
		`CREATE INDEX IF NOT EXISTS tickets_link ON tickets (project, link_integration, link_id)`,
		`CREATE INDEX IF NOT EXISTS tickets_created ON tickets (project, created, number)`,
		`CREATE TABLE IF NOT EXISTS labels (project TEXT NOT NULL, kind TEXT NOT NULL, number INTEGER NOT NULL, label TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS labels_record ON labels (project, kind, number)`,
		`CREATE INDEX IF NOT EXISTS labels_label ON labels (project, kind, label)`,
		`CREATE TABLE IF NOT EXISTS tags (project TEXT NOT NULL, kind TEXT NOT NULL, number INTEGER NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS tags_record ON tags (project, kind, number)`,
		`CREATE INDEX IF NOT EXISTS tags_key ON tags (project, kind, key, value)`,
		`CREATE TABLE IF NOT EXISTS pushed_commits (project TEXT NOT NULL, number INTEGER NOT NULL, seq INTEGER NOT NULL, repository TEXT NOT NULL, sha TEXT NOT NULL, branch TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS pushed_commits_record ON pushed_commits (project, number)`,
		`CREATE INDEX IF NOT EXISTS pushed_commits_sha ON pushed_commits (project, repository, sha)`,
		`CREATE INDEX IF NOT EXISTS pushed_commits_branch ON pushed_commits (project, repository, branch)`,
		`CREATE TABLE IF NOT EXISTS blockers (project TEXT NOT NULL, number INTEGER NOT NULL, blocker INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS blockers_record ON blockers (project, number)`,
		`CREATE INDEX IF NOT EXISTS blockers_blocker ON blockers (project, blocker)`,
		`CREATE TABLE IF NOT EXISTS replacements (project TEXT NOT NULL, number INTEGER NOT NULL, replacement INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS replacements_record ON replacements (project, number)`,
		`CREATE INDEX IF NOT EXISTS replacements_replacement ON replacements (project, replacement)`,
		`CREATE TABLE IF NOT EXISTS claims (project TEXT NOT NULL, number INTEGER NOT NULL, idx INTEGER NOT NULL, session_id TEXT NOT NULL, started BIGINT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS claims_record ON claims (project, number)`,
		`CREATE INDEX IF NOT EXISTS claims_session ON claims (session_id, started)`,
		`CREATE TABLE IF NOT EXISTS usage_records (id ` + d.serial + `, project TEXT NOT NULL, day TEXT NOT NULL, type TEXT NOT NULL, doc ` + d.json + ` NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS usage_records_day ON usage_records (day, project, id)`,
	}
}

// prepare creates the schema and checks the format: a fresh database
// takes format, and one at any other refuses to open, naming both.
func prepare(ctx context.Context, db *sql.DB, d dialect, format int, what string) error {
	for _, stmt := range d.schema() {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	var stored string
	err := db.QueryRowContext(ctx, d.rebind(`SELECT value FROM meta WHERE key = 'format'`)).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.ExecContext(ctx, d.rebind(`INSERT INTO meta (key, value) VALUES ('format', ?)`), strconv.Itoa(format))
		return err
	}
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if f, err := strconv.Atoi(stored); err != nil || f != format {
		return fmt.Errorf("%s: unknown format %s (this code-central understands format %d)", what, stored, format)
	}
	return nil
}

// placeholders is n comma-separated ? placeholders.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
