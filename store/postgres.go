//go:build !js

// The Postgres Store Kind (Spec code-central-1032, ADR 0068): the shared
// schema in a Postgres database that several services, on several
// machines, open at once, on pgx. No service locks it. Each Update is a
// serializable transaction the engine orders against the others', run
// again when two collide. Each service registers itself in the Store,
// and holds the Watcher Leases of the Projects whose Watchers it runs
// (sql_services.go); its notices to the others go over LISTEN and NOTIFY
// (postgres_changes.go).

package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

// postgres is Postgres's dialect: documents are JSONB, placeholders are
// numbered, a View is one repeatable-read snapshot, and every Update is a
// SERIALIZABLE transaction, run again on a serialization failure or a
// deadlock, with no process-wide lock, since other services write the
// Store too.
var postgres = dialect{
	size:   postgresSize,
	json:   "JSONB",
	serial: "BIGSERIAL PRIMARY KEY",
	rebind: numberPlaceholders,
	begin:  "BEGIN ISOLATION LEVEL SERIALIZABLE",
	view:   "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY",
	// Read committed whatever the role's default, so a service that waited
	// at the lock sees the schema the other committed; and one key for the
	// whole schema, so two services creating it at once take turns.
	schemaBegin: "BEGIN ISOLATION LEVEL READ COMMITTED",
	lockSchema:  "SELECT pg_advisory_xact_lock(" + strconv.Itoa(schemaLockKey) + ")",
	shared:      true,
	retryable:   isSerializationFailure,
}

// schemaLockKey is the advisory lock's key, code-central's own in the
// database: the service's port, which nothing else locks on.
const schemaLockKey = 7733

// connectTimeout bounds opening a connection when the URL sets no
// connect_timeout, so an unreachable database fails serve rather than
// hangs it.
const connectTimeout = 10 * time.Second

// OpenPostgres opens the Postgres Store at url: a postgres:// URL, or a
// key=value string, as pgx reads them, holding the credential. It creates
// the schema if needed, migrates a database at an older format in one
// transaction (postgres_migrate.go), while no other service is
// registered on it, and refuses one at a newer format by name. It takes
// no lock: the Store is shared by design.
func OpenPostgres(url string) (*SQLStore, error) { return openPostgres(url, Format, migrations) }

// openPostgres is OpenPostgres as a build whose format is format and
// whose migration chain is steps: Format and migrations, except in tests.
func openPostgres(url string, format int, steps []MigrationStep) (*SQLStore, error) {
	cfg, location, err := parsePostgres(url)
	if err != nil {
		return nil, err
	}
	addr := location.Path
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = connectTimeout
	}
	db := stdlib.OpenDB(*cfg)
	// A few connections serve the Views and Updates in flight, and Usage's
	// appends beside them.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	s := &SQLStore{db: db, d: postgres, path: addr, url: url, location: location, format: format, service: newServiceID()}
	s.usage = &sqlUsage{db: db, d: postgres}
	older, err := prepare(context.Background(), db, postgres, format, addr)
	if err == nil && older > 0 {
		err = s.migrateShared(older, format, steps, time.Now())
	}
	if err != nil {
		s.Close()
		var connect *pgconn.ConnectError
		if errors.As(err, &connect) {
			// pgx's message names the user it tried as; the address will do.
			err = fmt.Errorf("%s: cannot connect: %w", addr, connect.Unwrap())
		}
		return nil, err
	}
	return s, nil
}

// PostgresLocation is the Location of the Postgres Store at url. It
// shows the Store's address (host:port/database) and keeps the URL, with
// its credential, for OpenAt alone.
func PostgresLocation(url string) (Location, error) {
	_, l, err := parsePostgres(url)
	return l, err
}

// postgresSize is the database's size, as Postgres counts it.
func postgresSize(s *SQLStore) (int64, error) {
	var size int64
	err := s.db.QueryRowContext(context.Background(), `SELECT pg_database_size(current_database())`).Scan(&size)
	return size, err
}

// parsePostgres reads url as pgx does and gives the Store's Location,
// which names its address. The error repeats no password: pgx masks it.
func parsePostgres(url string) (*pgx.ConnConfig, Location, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, Location{}, fmt.Errorf("postgres: %w", err)
	}
	db := cfg.Database
	if db == "" {
		db = cfg.User // Postgres's default database is the user's.
	}
	return cfg, Location{Kind: Postgres, Path: fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, db),
		Host: cfg.Host, Port: int(cfg.Port), Database: db, url: url}, nil
}

// numberPlaceholders is q with its ? placeholders numbered $1, $2 and so
// on; a ? inside quoted text stays.
func numberPlaceholders(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 16)
	n, quoted := 0, false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case c == '\'':
			quoted = !quoted
		case c == '?' && !quoted:
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// isSerializationFailure is whether err is Postgres refusing a transaction
// it couldn't order against a concurrent one (SQLSTATE 40001) or one that
// deadlocked with another (40P01): a fresh attempt may succeed.
func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}
