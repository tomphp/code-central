//go:build !js

package store

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"
)

// sqlServices are a database Store's Services: a row per Registration
// and per Watcher Lease, each written in a statement of its own, on the
// pool's connections, outside every Update. On SQLite the one service
// holding the lock is the only one registered; on Postgres every service
// sharing the Store is.
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
