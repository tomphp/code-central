package core

import (
	"context"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// WaitTimeout is how long one Wait holds before returning the State
// unchanged, on the injected clock.
const WaitTimeout = 60 * time.Second

// WaitResult is a Ticket's State when a Wait returns, with its last move.
type WaitResult struct {
	Ticket   string      `json:"ticket"`
	State    store.State `json:"state"`
	LastMove *MoveView   `json:"last_move,omitempty"`
}

// Events is the bus core sends a change notice on after every write.
func (c *Core) Events() *events.Bus { return c.events }

// Wait holds until the Ticket is no longer in State from, for at most
// WaitTimeout or until ctx ends, and returns its State. It returns at
// once if the Ticket has already left from. It is a read: no Claim ID.
func (c *Core) Wait(ctx context.Context, key string, ref Ref, from store.State) (WaitResult, error) {
	if !from.Valid() {
		return WaitResult{}, invalid("from", "from must be a State, such as %q", store.NeedsAnswer)
	}
	notices, stop := c.events.Subscribe()
	defer stop()
	var timeout <-chan time.Time
	for {
		res, err := c.waitResult(key, ref)
		if err != nil || res.State != from {
			return res, err
		}
		if timeout == nil {
			timeout = c.clock.After(WaitTimeout)
		}
		for woken := false; !woken; {
			select {
			case e := <-notices:
				woken = e.Type == events.TicketChanged && e.Project == key && e.ID == res.Ticket
			case <-timeout:
				return res, nil
			case <-ctx.Done():
				return res, nil
			}
		}
	}
}

func (c *Core) waitResult(key string, ref Ref) (WaitResult, error) {
	var res WaitResult
	err := c.store.View(func(r store.Reader) error {
		if _, ok := r.Project(key); !ok {
			return unknownProject(key)
		}
		t, err := resolveTicket(r, key, "wait", "ticket", ref)
		if err != nil {
			return err
		}
		res = WaitResult{Ticket: DisplayID(key, t.Number), State: t.State}
		if n := len(t.Moves); n > 0 {
			m := t.Moves[n-1]
			res.LastMove = &MoveView{From: m.From, To: m.To, Actor: m.Actor, Time: m.Time, Claim: m.Claim, Via: m.Via, Commit: m.Commit}
		}
		return nil
	})
	return res, err
}

// notifying wraps a store so every successful Update sends a change
// notice for the record it wrote, after the write is durable.
type notifying struct {
	store.Store
	bus *events.Bus
}

func (n notifying) Update(fn func(store.Writer) error) error {
	w := &noticeWriter{}
	err := n.Store.Update(func(sw store.Writer) error {
		w.Writer = sw
		return fn(w)
	})
	if err == nil && w.notice != nil {
		n.bus.Publish(*w.notice)
	}
	return err
}

type noticeWriter struct {
	store.Writer
	notice *events.Event
}

func (w *noticeWriter) PutProject(p store.Project) error {
	return w.put(w.Writer.PutProject(p), events.Event{Type: events.ProjectChanged, Project: p.Key})
}

func (w *noticeWriter) PutSpec(key string, s store.Spec) error {
	return w.put(w.Writer.PutSpec(key, s), events.Event{Type: events.SpecChanged, Project: key, ID: DisplayID(key, s.Number)})
}

func (w *noticeWriter) PutTicket(key string, t store.Ticket) error {
	return w.put(w.Writer.PutTicket(key, t), events.Event{Type: events.TicketChanged, Project: key, ID: DisplayID(key, t.Number)})
}

// PutImport sends one project.changed notice for the whole Import, so a
// large one can't overrun a listener's buffer; listeners refetch.
func (w *noticeWriter) PutImport(key string, im store.Import) error {
	return w.put(w.Writer.PutImport(key, im), events.Event{Type: events.ProjectChanged, Project: key})
}

func (w *noticeWriter) put(err error, e events.Event) error {
	if err == nil {
		w.notice = &e
	}
	return err
}
