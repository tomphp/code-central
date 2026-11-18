//go:build !js

// The Postgres Store Kind (Spec code-central-1032, ADR 0068): the shared
// schema in a Postgres database that several services, on several
// machines, open at once, on pgx. No service locks it. Each Update is a
// serializable transaction the engine orders against the others', run
// again when two collide; a service's place in the Store is the Service
// Registration of Ticket code-central-1049, and its notices to the
// others come with code-central-1048.

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
	json:   "JSONB",
	serial: "BIGSERIAL PRIMARY KEY",
	rebind: numberPlaceholders,
	begin:  "BEGIN ISOLATION LEVEL SERIALIZABLE",
	view:   "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY",
	// One key for the whole schema: two services creating it at once take
	// turns, and the second finds it made.
	lockSchema: "SELECT pg_advisory_xact_lock(" + strconv.Itoa(schemaLockKey) + ")",
	shared:     true,
	retryable:  isSerializationFailure,
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
// the schema if needed, and refuses a database at another format by name:
// a newer one as every Kind does, and an older one until a Postgres
// Store's Migrations land (Ticket code-central-1049). It takes no lock:
// the Store is shared by design.
func OpenPostgres(url string) (*SQLStore, error) {
	cfg, addr, err := parsePostgres(url)
	if err != nil {
		return nil, err
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = connectTimeout
	}
	db := stdlib.OpenDB(*cfg)
	// A few connections serve the Views and Updates in flight, and Usage's
	// appends beside them.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	s := &SQLStore{db: db, d: postgres, path: addr, url: url}
	s.usage = &sqlUsage{db: db, d: postgres}
	older, err := prepare(context.Background(), db, postgres, Format, addr)
	if err == nil && older > 0 {
		err = fmt.Errorf("%s: format %d: this code-central cannot yet migrate a postgres store (it understands format %d)", addr, older, Format)
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
	_, addr, err := parsePostgres(url)
	if err != nil {
		return Location{}, err
	}
	return Location{Kind: Postgres, Path: addr, url: url}, nil
}

// parsePostgres reads url as pgx does and names the Store's address. The
// error repeats no password: pgx masks it.
func parsePostgres(url string) (*pgx.ConnConfig, string, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, "", fmt.Errorf("postgres: %w", err)
	}
	db := cfg.Database
	if db == "" {
		db = cfg.User // Postgres's default database is the user's.
	}
	return cfg, fmt.Sprintf("%s:%d/%s", cfg.Host, cfg.Port, db), nil
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
