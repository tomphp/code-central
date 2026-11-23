package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
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

// ArchiveProject is the human's archive_project: it puts away a Project
// whose Tickets are all finished.
func (c *Core) ArchiveProject(ctx context.Context, key string) (api.ProjectView, error) {
	if err := foundation.HumanOnly(ctx, "archive_project"); err != nil {
		return api.ProjectView{}, err
	}
	var v api.ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		if open := unfinishedTickets(w, key); len(open) > 0 {
			return &api.Error{Code: api.ActionNotAllowed, Message: fmt.Sprintf(
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
func (c *Core) UnarchiveProject(ctx context.Context, key string) (api.ProjectView, error) {
	if err := foundation.HumanOnly(ctx, "unarchive_project"); err != nil {
		return api.ProjectView{}, err
	}
	var v api.ProjectView
	err := c.store.Update(func(w store.Writer) error {
		p, ok := w.Project(key)
		if !ok {
			return api.UnknownProjectError(key)
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
	var out []string
	for _, t := range r.FindTickets(key, store.TicketFilter{States: unfinished}) {
		out = append(out, fmt.Sprintf("%s (%s)", domain.DisplayID(key, t.Number), t.State))
	}
	return out
}
