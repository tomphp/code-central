//go:build !js

// Migrating a Postgres Store (Spec code-central-1032, ADR 0068) runs the
// File Store's chain of steps, as a SQLite Store's Migration does, but in
// the database itself and in one transaction: the records are exported as
// a File Store layout into a temporary folder, the File Store migrates
// that, and the record tables are dropped, made again in this build's
// layout and loaded with the result. Nothing is kept aside: the
// transaction commits whole or not at all, so the Store, and any backup or
// point in time of it, is at one format or the other. It runs only while
// no other service is registered on the Store, since one would go on
// writing the old format, and it holds the schema lock throughout, so a
// service opening the Store meanwhile waits and then finds it migrated.

package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// failPostgresMigrationBeforeCommit, set only by tests, fails a Migration
// once it has rewritten every record, just before it commits.
var failPostgresMigrationBeforeCommit error

// recordTables are the tables that hold records and their side tables,
// which a Migration drops and makes again: every table of the schema but
// meta and the services'. A table a newer format adds is made by the
// schema; one it drops goes here, to be dropped.
var recordTables = []string{"projects", "options", "ci_runs", "specs", "tickets", "labels", "tags",
	"pushed_commits", "blockers", "replacements", "claims", "usage_records"}

// migrateShared brings the Shared Store from format from up to format, in
// one transaction, unless a service is registered on it at now.
func (s *SQLStore) migrateShared(from, format int, steps []MigrationStep, now time.Time) error {
	ctx := context.Background()
	// The staged File Store works out its own chain; checked here first, a
	// missing step names the Store rather than the staged store.yaml.
	if _, err := migrationChain(from, format, steps); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	failed := func(what string, err error) error {
		return fmt.Errorf("migrating %s from format %d: %s: %w (the store is untouched)", s.path, from, what, err)
	}
	stage, err := os.MkdirTemp("", "code-central-migrating-")
	if err != nil {
		return failed("making a folder to stage it in", err)
	}
	defer os.RemoveAll(stage)

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return failed("connecting", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, s.d.begin); err != nil {
		return failed("beginning", err)
	}
	committed := false
	defer func() {
		if !committed {
			conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	exec := func(q string, args ...any) error {
		_, err := conn.ExecContext(ctx, s.d.rebind(q), args...)
		return err
	}
	// The schema lock first, as prepare takes it: a service opening the
	// Store waits for the Migration. Then the Registrations' table, so no
	// service registers or renews until it commits.
	if err := exec(s.d.lockSchema); err != nil {
		return failed("locking the schema", err)
	}
	if err := exec(`LOCK TABLE registrations IN SHARE MODE`); err != nil {
		return failed("locking the registrations", err)
	}
	stored, err := storedFormat(ctx, conn, s.d)
	switch {
	case err != nil:
		return failed("reading the format", err)
	case stored == format:
		return nil // another service migrated it while this one waited
	case stored != from:
		return fmt.Errorf("%s: unknown format %d (this code-central understands format %d)", s.path, stored, format)
	}
	regs, err := liveRegistrations(ctx, conn, s.d, now)
	if err != nil {
		return failed("reading the registrations", err)
	}
	if len(regs) > 0 {
		return fmt.Errorf("cannot migrate %s from format %d to %d while another service is registered on it: %s; "+
			"stop it and start this one again, or wait %s for one that stopped without saying so (the store is untouched)",
			s.path, from, format, registered(regs), ServiceLife)
	}

	files := filepath.Join(stage, "store")
	if err := exportFiles(ctx, conn, s.d, files, from); err != nil {
		return failed("staging it as files", err)
	}
	staged, err := open(files, format, steps)
	if err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	defer staged.Close()
	for _, table := range recordTables {
		if err := exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			return failed("dropping "+table, err)
		}
	}
	for _, stmt := range s.d.schema() {
		if err := exec(stmt); err != nil {
			return failed("making the tables", err)
		}
	}
	if err := importFiles(ctx, conn, s.d, staged); err != nil {
		return failed("loading the migrated records", err)
	}
	if err := exec(`UPDATE meta SET value = ? WHERE key = 'format'`, strconv.Itoa(format)); err != nil {
		return failed("writing the format", err)
	}
	if failPostgresMigrationBeforeCommit != nil {
		return failed("committing", failPostgresMigrationBeforeCommit)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return failed("committing", err)
	}
	committed = true
	s.migrated = &Migration{From: from, To: format}
	return nil
}

// registered names each service in regs by its host and version.
func registered(regs []Registration) string {
	names := make([]string, 0, len(regs))
	for _, r := range regs {
		names = append(names, fmt.Sprintf("%s (%s)", r.Host, r.Version))
	}
	return strings.Join(names, ", ")
}
