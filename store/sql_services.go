//go:build !js

package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"
)

// sqlServices are a database Store's Services: a row per Registration,
// per Watcher Lease and per piece of runtime state, each written in a
// statement of its own, on the pool's connections, outside every Update.
// On SQLite the one service holding the lock is the only one registered;
// on Postgres every service sharing the Store is.
type sqlServices struct{ s *SQLStore }

func (v sqlServices) exec(q string, args ...any) error {
	_, err := v.s.db.ExecContext(context.Background(), v.s.d.rebind(q), args...)
	return err
}

// Register writes r, then reads the format, so a Registration a
// Migration held back reads the format the Migration left. It deletes
// the Registrations no longer live, which a service that stopped without
// deregistering left.
func (v sqlServices) Register(r Registration) error {
	if err := v.exec(`DELETE FROM registrations WHERE renewed <= ? AND id <> ?`, unixNanos(r.Renewed.Add(-ServiceLife)), r.ID); err != nil {
		return fmt.Errorf("%s: registering: %w", v.s.path, err)
	}
	if err := v.exec(`INSERT INTO registrations (id, host, version, format, renewed) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET host = excluded.host, version = excluded.version, format = excluded.format, renewed = excluded.renewed`,
		r.ID, r.Host, r.Version, r.Format, unixNanos(r.Renewed)); err != nil {
		return fmt.Errorf("%s: registering: %w", v.s.path, err)
	}
	stored, err := storedFormat(context.Background(), v.s.db, v.s.d)
	if err != nil {
		return fmt.Errorf("%s: reading the format: %w", v.s.path, err)
	}
	if stored != r.Format {
		return &FormatChangedError{Where: v.s.path, Stored: stored, Own: r.Format}
	}
	return nil
}

func (v sqlServices) Deregister(id string) error {
	if err := v.exec(`DELETE FROM watcher_leases WHERE holder = ?`, id); err != nil {
		return fmt.Errorf("%s: deregistering: %w", v.s.path, err)
	}
	if err := v.exec(`DELETE FROM registrations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("%s: deregistering: %w", v.s.path, err)
	}
	return nil
}

func (v sqlServices) Registrations(now time.Time) ([]Registration, error) {
	return liveRegistrations(context.Background(), v.s.db, v.s.d, now)
}

// HoldLease writes id's Lease unless another service's is there and
// unexpired, in one statement the engine runs atomically against another
// service's, then reads what is there.
func (v sqlServices) HoldLease(project, id string, now time.Time) (Lease, error) {
	if err := v.exec(`INSERT INTO watcher_leases (project, holder, expires) VALUES (?, ?, ?)
		ON CONFLICT (project) DO UPDATE SET holder = excluded.holder, expires = excluded.expires
		WHERE watcher_leases.holder = excluded.holder OR watcher_leases.expires <= ?`,
		project, id, unixNanos(now.Add(ServiceLife)), unixNanos(now)); err != nil {
		return Lease{}, fmt.Errorf("%s: holding %s's watcher lease: %w", v.s.path, project, err)
	}
	l := Lease{Project: project}
	var expires int64
	err := v.s.db.QueryRowContext(context.Background(), v.s.d.rebind(`SELECT l.holder, COALESCE(r.host, ''), l.expires
		FROM watcher_leases l LEFT JOIN registrations r ON r.id = l.holder WHERE l.project = ?`), project).Scan(&l.Holder, &l.Host, &expires)
	if err != nil {
		return Lease{}, fmt.Errorf("%s: reading %s's watcher lease: %w", v.s.path, project, err)
	}
	l.Expires = fromUnixNanos(expires)
	return l, nil
}

// The runtime state's rows are each written whole in one statement, the
// newest write winning, and listed sorted here, as the engines' collations
// would order them apart.

func (v sqlServices) PutHeartbeat(h Heartbeat) error {
	if err := v.exec(`INSERT INTO heartbeats (project, factory, factory_host, doc, service, service_host, expires) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project, factory, factory_host) DO UPDATE SET doc = excluded.doc, service = excluded.service, service_host = excluded.service_host, expires = excluded.expires`,
		h.Project, h.Factory, h.FactoryHost, string(h.Doc), h.Service, h.ServiceHost, unixNanos(h.Expires)); err != nil {
		return fmt.Errorf("%s: writing %s's heartbeat: %w", v.s.path, h.Factory, err)
	}
	return nil
}

func (v sqlServices) Heartbeats() ([]Heartbeat, error) {
	out, err := queryRows(v, `SELECT project, factory, factory_host, doc, service, service_host, expires FROM heartbeats`,
		func(rows *sql.Rows) (Heartbeat, error) {
			var h Heartbeat
			var doc string
			var expires int64
			err := rows.Scan(&h.Project, &h.Factory, &h.FactoryHost, &doc, &h.Service, &h.ServiceHost, &expires)
			h.Doc, h.Expires = json.RawMessage(doc), fromUnixNanos(expires)
			return h, err
		})
	if err != nil {
		return nil, fmt.Errorf("%s: reading the heartbeats: %w", v.s.path, err)
	}
	slices.SortFunc(out, byHeartbeat)
	return out, nil
}

func (v sqlServices) PruneHeartbeats(now time.Time) error {
	if err := v.exec(`DELETE FROM heartbeats WHERE expires <= ?`, unixNanos(now)); err != nil {
		return fmt.Errorf("%s: pruning the heartbeats: %w", v.s.path, err)
	}
	return nil
}

func (v sqlServices) PutFactoryReports(r FactoryReports) error {
	if err := v.exec(`INSERT INTO factory_reports (project, doc) VALUES (?, ?) ON CONFLICT (project) DO UPDATE SET doc = excluded.doc`,
		r.Project, string(r.Doc)); err != nil {
		return fmt.Errorf("%s: writing %s's factory reports: %w", v.s.path, r.Project, err)
	}
	return nil
}

func (v sqlServices) FactoryReports() ([]FactoryReports, error) {
	out, err := queryRows(v, `SELECT project, doc FROM factory_reports`, func(rows *sql.Rows) (FactoryReports, error) {
		var r FactoryReports
		var doc string
		err := rows.Scan(&r.Project, &doc)
		r.Doc = json.RawMessage(doc)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("%s: reading the factory reports: %w", v.s.path, err)
	}
	slices.SortFunc(out, func(a, b FactoryReports) int { return cmp.Compare(a.Project, b.Project) })
	return out, nil
}

func (v sqlServices) PutPing(p Ping) error {
	if err := v.exec(`INSERT INTO pings (session_id, pinged, service, service_host) VALUES (?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET pinged = excluded.pinged, service = excluded.service, service_host = excluded.service_host`,
		p.Session, unixNanos(p.At), p.Service, p.ServiceHost); err != nil {
		return fmt.Errorf("%s: writing %s's ping: %w", v.s.path, p.Session, err)
	}
	return nil
}

func (v sqlServices) Pings() ([]Ping, error) {
	out, err := queryRows(v, `SELECT session_id, pinged, service, service_host FROM pings`, func(rows *sql.Rows) (Ping, error) {
		var p Ping
		var at int64
		err := rows.Scan(&p.Session, &at, &p.Service, &p.ServiceHost)
		p.At = fromUnixNanos(at)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("%s: reading the pings: %w", v.s.path, err)
	}
	slices.SortFunc(out, func(a, b Ping) int { return cmp.Compare(a.Session, b.Session) })
	return out, nil
}

func (v sqlServices) PrunePings(before time.Time) error {
	if err := v.exec(`DELETE FROM pings WHERE pinged < ?`, unixNanos(before)); err != nil {
		return fmt.Errorf("%s: pruning the pings: %w", v.s.path, err)
	}
	return nil
}

func (v sqlServices) PutTranscriptHolder(h TranscriptHolder) error {
	if err := v.exec(`INSERT INTO transcript_holders (project, ticket, claim, service, service_host, since) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (project, ticket, claim) DO UPDATE SET service = excluded.service, service_host = excluded.service_host, since = excluded.since`,
		h.Project, h.Ticket, h.Claim, h.Service, h.ServiceHost, unixNanos(h.At)); err != nil {
		return fmt.Errorf("%s: writing %s-%d claim %d's transcript holder: %w", v.s.path, h.Project, h.Ticket, h.Claim, err)
	}
	return nil
}

func (v sqlServices) TranscriptHolder(project string, ticket, claim int) (TranscriptHolder, bool, error) {
	h := TranscriptHolder{Project: project, Ticket: ticket, Claim: claim}
	var at int64
	err := v.s.db.QueryRowContext(context.Background(), v.s.d.rebind(`SELECT service, service_host, since FROM transcript_holders
		WHERE project = ? AND ticket = ? AND claim = ?`), project, ticket, claim).Scan(&h.Service, &h.ServiceHost, &at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return TranscriptHolder{}, false, nil
	case err != nil:
		return TranscriptHolder{}, false, fmt.Errorf("%s: reading %s-%d claim %d's transcript holder: %w", v.s.path, project, ticket, claim, err)
	}
	h.At = fromUnixNanos(at)
	return h, true, nil
}

func (v sqlServices) PruneTranscriptHolders(before time.Time) error {
	if err := v.exec(`DELETE FROM transcript_holders WHERE since < ?`, unixNanos(before)); err != nil {
		return fmt.Errorf("%s: pruning the transcript holders: %w", v.s.path, err)
	}
	return nil
}

// queryRows runs q on v's pool and scans each row it returns.
func queryRows[T any](v sqlServices, q string, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := v.s.db.QueryContext(context.Background(), v.s.d.rebind(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// liveRegistrations reads the Registrations q's database holds live at
// now, in host, then ID, order.
func liveRegistrations(ctx context.Context, q querier, d dialect, now time.Time) ([]Registration, error) {
	rows, err := q.QueryContext(ctx, d.rebind(`SELECT id, host, version, format, renewed FROM registrations WHERE renewed > ?`),
		unixNanos(now.Add(-ServiceLife)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Registration{}
	for rows.Next() {
		var r Registration
		var renewed int64
		if err := rows.Scan(&r.ID, &r.Host, &r.Version, &r.Format, &renewed); err != nil {
			return nil, err
		}
		r.Renewed = fromUnixNanos(renewed)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, byHost)
	return out, nil
}

// storedFormat is the format in q's database's meta row.
func storedFormat(ctx context.Context, q querier, d dialect) (int, error) {
	var stored string
	if err := q.QueryRowContext(ctx, d.rebind(`SELECT value FROM meta WHERE key = 'format'`)).Scan(&stored); err != nil {
		return 0, err
	}
	f, err := strconv.Atoi(stored)
	if err != nil {
		return 0, fmt.Errorf("unknown format %s", stored)
	}
	return f, nil
}

// fromUnixNanos is the time a column unixNanos wrote holds.
func fromUnixNanos(n int64) time.Time { return time.Unix(0, n).UTC() }
