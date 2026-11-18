//go:build !js

// A Postgres Store's Change notices (Spec code-central-1032): an Update's
// Announce is a pg_notify in its transaction, so Postgres sends it only
// if it commits, and Changes listens on a connection of its own for the
// other services'. Each Store handle is a service, with an ID of its own
// in every notice it sends, so it can leave its own out.

package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tomoram/code-central/events"
)

// changesChannel is the expression naming the channel a Store's notices
// go out on: one per schema, since a channel is the database's, so two
// Stores in one database never hear each other. Hashed, as a channel
// name is at most 63 bytes and a schema's name may be that long alone.
const changesChannel = `'code_central_' || md5(current_schema())`

// How a lost LISTEN connection is made again: after relistenBackoff,
// doubled after each failed attempt up to relistenBackoffMax.
const (
	relistenBackoff    = 100 * time.Millisecond
	relistenBackoffMax = 10 * time.Second
)

// listenCheck is how long the LISTEN connection waits for a notice before
// it pings the database, so a connection that died without a word is
// found and made again.
const listenCheck = 30 * time.Second

// notice is a Change as it goes over the channel, with the ID of the
// service that sent it.
type notice struct {
	Service string `json:"service"`
	Project string `json:"project"`
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Wakes   bool   `json:"wakes,omitempty"`
}

// newServiceID is a fresh ID for a Store handle's notices.
func newServiceID() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// changesApplication is the application_name of service's LISTEN
// connection, which names it among the database's connections.
func changesApplication(service string) string {
	return "code-central changes " + service
}

// announce sends ch to the other services as part of the Update's
// transaction. A failure fails the Update, as Postgres aborts the
// transaction anyway.
func (w *sqlWriter) announce(ch Change) {
	payload, err := json.Marshal(notice{Service: w.service, Project: ch.Project, Type: ch.Type, ID: ch.ID, Wakes: ch.Wakes})
	if err == nil {
		err = w.exec(`SELECT pg_notify(`+changesChannel+`, ?)`, string(payload))
	}
	if err != nil && *w.err == nil {
		*w.err = err
	}
}

// listen delivers the Changes the other services commit, from a LISTEN
// connection of its own, until the Store closes. The first connection is
// made before it returns, so a Change committed after Changes returns is
// heard. Whenever the connection is made again after losing it, or after
// failing to make it at first, the Store may have missed Changes, so it
// delivers a project-wide Change for every Project, which wakes the
// Factories, and every waiter refetches.
func (s *SQLStore) listen(deliver func(Change)) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.stops = append(s.stops, func() {
		cancel()
		<-done
	})
	conn, _ := s.listenConn(ctx)
	go func() {
		defer close(done)
		missed := conn == nil
		wait := relistenBackoff
		for {
			if conn == nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
				wait = min(wait*2, relistenBackoffMax)
				conn, _ = s.listenConn(ctx)
				if conn == nil {
					continue
				}
			}
			if !missed || s.deliverEveryProject(deliver) {
				missed = false
				wait = relistenBackoff
				s.hear(ctx, conn, deliver)
			}
			conn.Close(context.Background())
			conn, missed = nil, true
			if ctx.Err() != nil {
				return
			}
		}
	}()
}

// listenConn is a fresh connection listening on the Store's channel, or
// nil and the error that stopped it.
func (s *SQLStore) listenConn(ctx context.Context) (*pgx.Conn, error) {
	cfg, _, err := parsePostgres(s.url)
	if err != nil {
		return nil, err
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = connectTimeout
	}
	cfg.RuntimeParams["application_name"] = changesApplication(s.service)
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var channel string
	err = conn.QueryRow(ctx, `SELECT `+changesChannel).Scan(&channel)
	if err == nil {
		_, err = conn.Exec(ctx, `LISTEN `+pgx.Identifier{channel}.Sanitize())
	}
	if err != nil {
		conn.Close(context.Background())
		return nil, err
	}
	return conn, nil
}

// deliverEveryProject delivers a project-wide Change, waking the
// Factories, for every Project in the Store, and whether it could read
// them.
func (s *SQLStore) deliverEveryProject(deliver func(Change)) bool {
	var keys []string
	if err := s.View(func(r Reader) error {
		for _, p := range r.Projects() {
			keys = append(keys, p.Key)
		}
		return nil
	}); err != nil {
		return false
	}
	for _, key := range keys {
		deliver(Change{Project: key, Type: events.ProjectChanged, Wakes: true})
	}
	return true
}

// hear delivers the other services' notices on conn until it fails or
// ctx ends. A notice that isn't one of a Store's is left out.
func (s *SQLStore) hear(ctx context.Context, conn *pgx.Conn, deliver func(Change)) {
	for {
		waiting, stop := context.WithTimeout(ctx, listenCheck)
		n, err := conn.WaitForNotification(waiting)
		stop()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if waiting.Err() == nil || !alive(ctx, conn) {
				return
			}
			continue
		}
		var got notice
		if json.Unmarshal([]byte(n.Payload), &got) != nil || got.Type == "" || got.Service == s.service {
			continue
		}
		deliver(Change{Project: got.Project, Type: got.Type, ID: got.ID, Wakes: got.Wakes})
	}
}

// alive is whether conn answers a ping within connectTimeout.
func alive(ctx context.Context, conn *pgx.Conn) bool {
	ctx, stop := context.WithTimeout(ctx, connectTimeout)
	defer stop()
	return conn.Ping(ctx) == nil
}
