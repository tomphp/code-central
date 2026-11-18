package core

import (
	"context"
	"slices"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// WaitTimeout is how long one Wait holds before returning the State
// unchanged, on the injected clock.
const WaitTimeout = 60 * time.Second

// WaitResult is a Ticket's State when a Wait returns, with its last move.
type WaitResult struct {
	Ticket   string       `json:"ticket"`
	State    domain.State `json:"state"`
	LastMove *MoveView    `json:"last_move,omitempty"`
}

// Events is the bus core sends a change notice on after every write.
func (c *Core) Events() *events.Bus { return c.events }

// Wait holds until the Ticket is no longer in State from, for at most
// WaitTimeout or until ctx ends, and returns its State. It returns at
// once if the Ticket has already left from. It is a read: no Claim ID.
func (c *Core) Wait(ctx context.Context, key string, ref Ref, from domain.State) (WaitResult, error) {
	if !from.Valid() {
		return WaitResult{}, invalid("from", "from must be a State, such as %q", domain.NeedsAnswer)
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
				// A project-wide notice may cover the Ticket: an Import, or a
				// Shared Store that may have missed Changes. Refetch.
				woken = e.Project == key && (e.Type == events.ProjectChanged || e.Type == events.TicketChanged && e.ID == res.Ticket)
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
		t, err := resolveTicket(r, key, "wait", "ticket", ref, store.Moves)
		if err != nil {
			return err
		}
		res = WaitResult{Ticket: DisplayID(key, t.Number), State: t.State}
		if n := len(t.Moves); n > 0 {
			m := t.Moves[n-1]
			mv := moveView(m)
			res.LastMove = &mv
		}
		return nil
	})
	return res, err
}

// notifying wraps a store so every successful Update sends a change
// notice for the record it wrote, after the write is durable, and wakes
// the Project's open next_factory_wake calls when the write is one the
// Factory's poll acts on (wakesTheFactory). It announces the same, as a
// store.Change, to the other services sharing the store, and gives each
// Change they commit the notice and wake a write here would.
type notifying struct {
	store.Store
	bus   *events.Bus
	wakes *factoryWakes
}

// newNotifying wraps s, hearing the Changes other services commit.
func newNotifying(s store.Store, bus *events.Bus, wakes *factoryWakes) notifying {
	n := notifying{Store: s, bus: bus, wakes: wakes}
	s.Changes(n.deliver)
	return n
}

func (n notifying) Update(fn func(store.Writer) error) error {
	var change *store.Change
	err := n.Store.Update(func(sw store.Writer) error {
		// Afresh on each attempt, so a retried one's notice never fires.
		w := &noticeWriter{Writer: sw}
		change = nil
		if err := fn(w); err != nil {
			return err
		}
		if w.notice != nil {
			change = &store.Change{Project: w.notice.Project, Type: w.notice.Type, ID: w.notice.ID, Wakes: w.wake != ""}
			sw.Announce(*change)
		}
		return nil
	})
	if err == nil && change != nil {
		n.deliver(*change)
	}
	return err
}

// deliver sends ch's change notice, and wakes its Project's open
// next_factory_wake calls when it wakes the Factories.
func (n notifying) deliver(ch store.Change) {
	n.bus.Publish(events.Event{Type: ch.Type, Project: ch.Project, ID: ch.ID})
	if ch.Wakes {
		n.wakes.wake(ch.Project)
	}
}

type noticeWriter struct {
	store.Writer
	notice *events.Event
	// wake is the Project whose Factories the write wakes, if any.
	wake string
}

// PutProject, PutSpec, PutTicket and PutImport read the records as they
// were before writing over them, to tell whether the write wakes the
// Factories.
func (w *noticeWriter) PutProject(p domain.Project) error {
	wakes := projectWakesTheFactory(w.Writer, p)
	return w.wakeIf(wakes, p.Key, w.put(w.Writer.PutProject(p), events.Event{Type: events.ProjectChanged, Project: p.Key}))
}

func (w *noticeWriter) PutSpec(key string, s domain.Spec) error {
	wakes := specWakesTheFactory(w.Writer, key, s)
	return w.wakeIf(wakes, key, w.put(w.Writer.PutSpec(key, s), events.Event{Type: events.SpecChanged, Project: key, ID: DisplayID(key, s.Number)}))
}

func (w *noticeWriter) PutTicket(key string, t domain.Ticket, parts ...store.Part) error {
	wakes := wakesTheFactory(w.Writer, key, t)
	return w.wakeIf(wakes, key, w.put(w.Writer.PutTicket(key, t, parts...), events.Event{Type: events.TicketChanged, Project: key, ID: DisplayID(key, t.Number)}))
}

// PutImport sends one project.changed notice for the whole Import, so a
// large one can't overrun a listener's buffer; listeners refetch. It
// wakes the Factories when it lands new records, which may be work, or
// when a record it rewrites would on its own, as implement's do.
func (w *noticeWriter) PutImport(key string, im store.Import) error {
	wakes := len(im.Specs) > 0 || len(im.Tickets) > 0 ||
		slices.ContainsFunc(im.Rewritten, func(t domain.Ticket) bool { return wakesTheFactory(w.Writer, key, t) }) ||
		slices.ContainsFunc(im.RewrittenSpecs, func(s domain.Spec) bool { return specWakesTheFactory(w.Writer, key, s) })
	return w.wakeIf(wakes, key, w.put(w.Writer.PutImport(key, im), events.Event{Type: events.ProjectChanged, Project: key}))
}

// wakeIf marks the write as waking Project key's Factories when wakes
// and the write succeeded.
func (w *noticeWriter) wakeIf(wakes bool, key string, err error) error {
	if err == nil && wakes {
		w.wake = key
	}
	return err
}

// PutOptions sends options.changed, which names no Project and no ID.
func (w *noticeWriter) PutOptions(o domain.Options) error {
	return w.put(w.Writer.PutOptions(o), events.Event{Type: events.OptionsChanged})
}

// PutCIRuns sends ci.changed, which names the Project only.
func (w *noticeWriter) PutCIRuns(key string, runs []domain.CIRun) error {
	return w.put(w.Writer.PutCIRuns(key, runs), events.Event{Type: events.CIChanged, Project: key})
}

func (w *noticeWriter) put(err error, e events.Event) error {
	if err == nil {
		w.notice = &e
	}
	return err
}
