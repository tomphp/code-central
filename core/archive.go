package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Archived Projects (ticket 39). The human archives a Project once every
// Ticket in it is finished; it then refuses every write, from any Actor,
// until the human unarchives it. Reads keep working.
//
// Two layers enforce it. Every write operation checks writable before
// anything else, so the refusal is the first thing an Actor hears. And
// archiveGuard wraps the store so a Put on an Archived Project fails
// whatever the operation, which keeps "every write" true for operations
// added later.

func archivedError(key string) *Error {
	return &Error{
		Code:           ActionNotAllowed,
		Message:        fmt.Sprintf("Project %s is archived, so it takes no writes; only the human can bring it back, with unarchive_project", key),
		AllowedActions: []string{"unarchive_project"},
	}
}

// writable is the check every write makes on its Project first: it
// exists and isn't archived.
func writable(r store.Reader, key string) error {
	p, ok := r.Project(key)
	if !ok {
		return unknownProject(key)
	}
	if !p.Archived.IsZero() {
		return archivedError(key)
	}
	return nil
}

// ArchiveProject is the human's archive_project: it puts away a Project
// whose Tickets are all finished.
func (c *Core) ArchiveProject(ctx context.Context, key string) (ProjectView, error) {
	if err := humanOnly(ctx, "archive_project"); err != nil {
		return ProjectView{}, err
	}
	var v ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		if open := unfinishedTickets(w, key); len(open) > 0 {
			return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf(
				"Project %s still has unfinished work, so it can't be archived until every Ticket is Done or Cancelled: %s",
				key, strings.Join(open, ", "))}
		}
		p, _ := w.Project(key)
		p.Archived = c.now()
		v = c.projectView(p)
		return w.PutProject(p)
	})
	return v, err
}

// UnarchiveProject is the human's unarchive_project: the Project takes
// writes again. Unarchiving a Project that isn't archived changes nothing.
func (c *Core) UnarchiveProject(ctx context.Context, key string) (ProjectView, error) {
	if err := humanOnly(ctx, "unarchive_project"); err != nil {
		return ProjectView{}, err
	}
	var v ProjectView
	err := c.store.Update(func(w store.Writer) error {
		p, ok := w.Project(key)
		if !ok {
			return unknownProject(key)
		}
		if p.Archived.IsZero() {
			v = c.projectView(p)
			return nil
		}
		p.Archived = time.Time{}
		v = c.projectView(p)
		return w.PutProject(p)
	})
	return v, err
}

// unfinishedTickets names each unfinished Ticket in the Project with its
// State, in number order.
func unfinishedTickets(r store.Reader, key string) []string {
	tickets := r.Tickets(key)
	slices.SortFunc(tickets, func(a, b store.Ticket) int { return a.Number - b.Number })
	var out []string
	for _, t := range tickets {
		if !finished(t.State) {
			out = append(out, fmt.Sprintf("%s (%s)", DisplayID(key, t.Number), t.State))
		}
	}
	return out
}

func humanOnly(ctx context.Context, op string) error {
	if a := ActorOf(ctx); a != Human {
		return &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is a human action; this request is made as %s", op, a)}
	}
	return nil
}

// archiveGuard wraps a store so no write lands on an Archived Project.
// A Project's own record may change only by being unarchived.
type archiveGuard struct{ store.Store }

func (g archiveGuard) Update(fn func(store.Writer) error) error {
	return g.Store.Update(func(w store.Writer) error { return fn(guardedWriter{w}) })
}

type guardedWriter struct{ store.Writer }

func (w guardedWriter) PutProject(p store.Project) error {
	if old, ok := w.Project(p.Key); ok && !old.Archived.IsZero() && !p.Archived.IsZero() {
		return archivedError(p.Key)
	}
	return w.Writer.PutProject(p)
}

func (w guardedWriter) PutSpec(key string, s store.Spec) error {
	if err := writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutSpec(key, s)
}

func (w guardedWriter) PutTicket(key string, t store.Ticket) error {
	if err := writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutTicket(key, t)
}

func (w guardedWriter) PutImport(key string, im store.Import) error {
	if err := writable(w, key); err != nil {
		return err
	}
	return w.Writer.PutImport(key, im)
}
