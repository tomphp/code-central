//go:build !js

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tomoram/code-central/usage"
)

// sqlUsage is a database Store's Usage (ADR 0066): one row per record,
// keyed by Project and local day, written on the pool's own connections
// outside any Update, so an append never waits on a write.
type sqlUsage struct {
	db *sql.DB
	d  dialect
}

func (u *sqlUsage) Append(project, day string, r usage.Record) error {
	var typ string
	var v any
	switch {
	case r.Call != nil:
		typ, v = usage.TypeCall, r.Call
	case r.Response != nil:
		typ, v = usage.TypeResponse, r.Response
	default:
		return errors.New("usage: a record holds neither a call nor a response")
	}
	doc, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = u.db.ExecContext(context.Background(), u.d.rebind(`INSERT INTO usage_records (project, day, type, doc) VALUES (?, ?, ?, ?)`), project, day, typ, string(doc))
	if err != nil {
		return fmt.Errorf("%s's usage for %s: %w", project, day, err)
	}
	return nil
}

func (u *sqlUsage) Read(project string, days []string) (usage.Records, error) {
	var out usage.Records
	var errs []error
	for _, day := range days {
		errs = append(errs, u.readDay(project, day, &out))
	}
	return out, errors.Join(errs...)
}

// readDay adds the records of day, of project or of every Project, to
// out, skipping and counting any that doesn't parse.
func (u *sqlUsage) readDay(project, day string, out *usage.Records) error {
	q := `SELECT project, type, doc FROM usage_records WHERE day = ?`
	args := []any{day}
	if project != "" {
		q += ` AND project = ?`
		args = append(args, project)
	}
	rows, err := u.db.QueryContext(context.Background(), u.d.rebind(q+` ORDER BY project, id`), args...)
	if err != nil {
		return fmt.Errorf("cannot read usage for %s: %w", day, err)
	}
	defer rows.Close()
	skipped := 0
	for rows.Next() {
		var p, typ string
		var doc []byte
		if err := rows.Scan(&p, &typ, &doc); err != nil {
			return fmt.Errorf("stopped reading usage for %s: %w", day, err)
		}
		switch typ {
		case usage.TypeCall:
			var c usage.Call
			if err := json.Unmarshal(doc, &c); err != nil {
				skipped++
				continue
			}
			out.Calls = append(out.Calls, c)
		case usage.TypeResponse:
			var r usage.Response
			if err := json.Unmarshal(doc, &r); err != nil {
				skipped++
				continue
			}
			r.Project, r.Day = p, day
			out.Responses = append(out.Responses, r)
		default:
			// A type this build doesn't know, from a newer one: not an error.
		}
	}
	if skipped > 0 {
		out.Skipped = append(out.Skipped, usage.Skipped{Where: "usage for " + day, Lines: skipped})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("stopped reading usage for %s: %w", day, err)
	}
	return nil
}

func (u *sqlUsage) Prune(before string) error {
	_, err := u.db.ExecContext(context.Background(), u.d.rebind(`DELETE FROM usage_records WHERE day < ?`), before)
	if err != nil {
		return fmt.Errorf("cannot delete usage before %s: %w", before, err)
	}
	return nil
}
