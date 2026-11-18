//go:build !js

// The database Stores (Spec code-central-1032): one shared schema of
// document rows, which SQLite and Postgres both hold, behind a small
// dialect. Each record is the JSON document of its file shape
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
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/usage"
)

// dialect is what differs between the database engines: the column type
// of a document, how a row numbers itself, the placeholders, and how an
// Update's transaction is ordered against other services'.
type dialect struct {
	// json is the column type a document is kept in.
	json string
	// serial is the column definition of a self-numbering key.
	serial string
	// rebind turns a statement's ? placeholders into the engine's.
	rebind func(string) string
	// begin opens an Update's transaction: taking the engine's write lock
	// at once, or at the isolation that orders it against other services'.
	begin string
	// view opens a View's transaction, one read-only snapshot for all its
	// reads; empty runs them outside one, under the process-wide lock.
	view string
	// schemaBegin opens prepare's transaction, the one that creates the
	// schema and checks the format: at the isolation that reads what
	// another service commits while this one waits for lockSchema.
	schemaBegin string
	// lockSchema is run first in that transaction, so two services opening
	// a fresh Store at once create it in turn; empty for none.
	lockSchema string
	// shared says other services write the Store too, so an Update runs
	// without the process-wide lock and is tried again when retryable says
	// its attempt collided with another service's.
	shared    bool
	retryable func(error) bool
}

// querier runs statements: a connection, a transaction or the pool.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLStore is a Store kept in a database: SQLite (OpenSQLite) or Postgres
// (OpenPostgres). An Update runs in one transaction, which a failed
// Update rolls back whole, so it leaves nothing. On SQLite, View and
// Update share one process-wide read-write mutex, as the other Kinds do;
// on Postgres, which other services write too, the engine orders each
// Update's transaction against theirs instead, and one that collides is
// tried again (ADR 0068).
type SQLStore struct {
	db *sql.DB
	d  dialect
	mu sync.RWMutex
	// path is where the Store is, for messages and reopening: the SQLite
	// file, or the Postgres address.
	path string
	// url is the Postgres Store's URL, with its credential, kept for
	// reopening in tests and never shown.
	url string
	// format is the store format this service reads and writes: Format,
	// except in tests.
	format int
	lock   *storeLock
	usage  usage.Store
	// migrated is the Migration this open ran, if any.
	migrated *Migration
}

// Path is where the database is: the SQLite file, or the Postgres
// address (host:port/database).
func (s *SQLStore) Path() string { return s.path }

// Migrated is the Migration the open ran, or nil if the database was
// already at this build's format. A Postgres Store's names no Backup: it
// ran in one transaction, and the database's own backups are its backup.
func (s *SQLStore) Migrated() *Migration { return s.migrated }

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

// Usage is the Store's Usage (ADR 0069): a table written on its own
// connection, outside any Update.
func (s *SQLStore) Usage() usage.Store { return s.usage }

// Services are the Store's Registrations and Watcher Leases, in tables of
// their own, written on the pool's connections outside every Update.
func (s *SQLStore) Services() Services { return sqlServices{s} }

// Changes never delivers: no other service shares a SQLite Store, and a
// Postgres Store's notices come with Ticket code-central-1048.
func (s *SQLStore) Changes(func(Change)) {}

// View runs fn over a Reader on one of the pool's connections, in the
// dialect's read-only transaction when it has one, so every read sees
// the same moment. A read that failed inside fn, which the Reader can't
// report, is its error, whatever fn returned, as in Update.
func (s *SQLStore) View(fn func(Reader) error) error {
	if !s.d.shared {
		s.mu.RLock()
		defer s.mu.RUnlock()
	}
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if s.d.view != "" {
		if _, err := conn.ExecContext(ctx, s.d.view); err != nil {
			return err
		}
		// Ended before the connection goes back to the pool, whatever fn
		// did; a read-only transaction has nothing to keep.
		defer conn.ExecContext(ctx, "ROLLBACK")
	}
	var failed error
	err = fn(sqlReader{q: conn, d: s.d, err: &failed})
	if failed != nil {
		return failed
	}
	return err
}

// How a Shared Store retries an Update whose attempt collided with another
// service's: conflictAttempts in all, with conflictBackoff before the
// second, doubled before each later one and jittered, so two services
// retrying at once don't collide again at the same moment.
const (
	conflictAttempts = 5
	conflictBackoff  = 10 * time.Millisecond
)

// Update runs fn over a Writer in one transaction: its write and
// side-table rows land together, or, when fn fails or panics, not at
// all. Under the store-wide lock on SQLite; on a Shared Store, in the
// engine's serializable transaction, run again from the start when it
// collides with another service's, until the attempts run out and it
// fails with ErrConflict.
func (s *SQLStore) Update(fn func(Writer) error) error {
	if !s.d.shared {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.attempt(fn)
	}
	wait := conflictBackoff
	for attempt := 1; ; attempt++ {
		err := s.attempt(fn)
		if err == nil || !s.d.retryable(err) {
			return err
		}
		if attempt == conflictAttempts {
			return fmt.Errorf("%w (%d attempts; the last: %v)", ErrConflict, attempt, err)
		}
		time.Sleep(wait/2 + time.Duration(rand.Int64N(int64(wait))))
		wait *= 2
	}
}

// attempt runs fn once, in one transaction on one of the pool's
// connections, committed only when fn and every read inside it succeeded.
func (s *SQLStore) attempt(fn func(Writer) error) error {
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
	if s.d.shared {
		// Another service may have migrated the Store since this one
		// opened it: its records are then of a format this one doesn't
		// write. Read in the transaction, so a Migration committing
		// beside it is one of the collisions the engine orders.
		stored, err := storedFormat(ctx, conn, s.d)
		if err != nil {
			return err
		}
		if stored != s.format {
			return &FormatChangedError{Where: s.path, Stored: stored, Own: s.format}
		}
	}
	var failed error
	w := &sqlWriter{sqlReader: sqlReader{q: conn, d: s.d, err: &failed}}
	err = fn(w)
	if failed != nil {
		// A statement that failed inside fn is the truer error: whatever
		// fn returned, it decided on that read's empty result.
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

// schema is the tables and indexes, in the dialect's types, each created
// only if it doesn't exist. meta comes first: every format has it, and
// prepare reads the format from it before deciding on the rest.
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
		`CREATE TABLE IF NOT EXISTS registrations (id TEXT PRIMARY KEY, host TEXT NOT NULL, version TEXT NOT NULL, format INTEGER NOT NULL, renewed BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS watcher_leases (project TEXT PRIMARY KEY, holder TEXT NOT NULL, expires BIGINT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS watcher_leases_holder ON watcher_leases (holder)`,
	}
}

// prepare checks the database's format and creates its schema, in one
// transaction on one connection, behind the dialect's schema lock. A
// fresh database takes the schema and format, and one at format is ready
// as it is. One at an older format is left as it is, and its format
// returned as older for its Migration to start from; one at a newer or
// unknown format refuses to open, naming both. The transaction reads
// what is committed as it goes, so a service that waited for another's
// to create the schema sees it.
func prepare(ctx context.Context, db *sql.DB, d dialect, format int, what string) (older int, err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, d.schemaBegin); err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	commit := func() error {
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		committed = true
		return nil
	}
	if d.lockSchema != "" {
		if _, err := conn.ExecContext(ctx, d.lockSchema); err != nil {
			return 0, fmt.Errorf("%s: %w", what, err)
		}
	}
	schema := d.schema()
	if _, err := conn.ExecContext(ctx, schema[0]); err != nil { // meta, which every format has
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	var stored string
	err = conn.QueryRowContext(ctx, d.rebind(`SELECT value FROM meta WHERE key = 'format'`)).Scan(&stored)
	fresh := errors.Is(err, sql.ErrNoRows)
	if err != nil && !fresh {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	if !fresh {
		f, err := strconv.Atoi(stored)
		if err != nil || f < 1 || f > format {
			return 0, fmt.Errorf("%s: unknown format %s (this code-central understands format %d)", what, stored, format)
		}
		if f < format {
			return f, commit()
		}
		return 0, commit()
	}
	for _, stmt := range schema[1:] {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return 0, fmt.Errorf("%s: %w", what, err)
		}
	}
	if _, err := conn.ExecContext(ctx, d.rebind(`INSERT INTO meta (key, value) VALUES ('format', ?)`), strconv.Itoa(format)); err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return 0, commit()
}

// placeholders is n comma-separated ? placeholders.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
