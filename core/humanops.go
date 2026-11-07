package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// The human's own operations beside Needs Me (ticket 32): moving a
// Ticket by hand, renaming a Project and listing every Project.

// MoveInput is the human's move(ticket, to_state).
type MoveInput struct {
	Ticket  Ref         `json:"ticket"`
	ToState store.State `json:"to_state"`
}

// Move is the human's move(to_state): a Ticket goes to any State but its
// own, Needs Answer and Failed. Leaving a Claim State ends the Claim;
// leaving Needs Answer also dismisses its open Questions.
func (c *Core) Move(ctx context.Context, key string, in MoveInput) (TicketView, error) {
	return c.updateTicket(key, in.Ticket, func(t *store.Ticket) (bool, error) {
		to := in.ToState
		row := Workflow[slices.IndexFunc(Workflow, func(a Action) bool { return a.Name == "move" })]
		switch {
		case !to.Valid():
			return false, invalid("to_state", "unknown State %q", to)
		case slices.Contains(row.Except, to):
			return false, invalid("to_state", "%s is entered only by posting a Round or a Failure Report", to)
		case to == t.State:
			return false, invalid("to_state", "%s is already %s", DisplayID(key, t.Number), to)
		}
		return true, transitionTo(ctx, t, key, "move", "", to, c.now())
	})
}

// RenameProjectInput is the human's rename_project(display_name).
type RenameProjectInput struct {
	DisplayName string `json:"display_name"`
}

// RenameProject is the human's rename_project: it changes the display
// name only. The key, and every ID built from it, never changes.
func (c *Core) RenameProject(ctx context.Context, key string, in RenameProjectInput) (ProjectView, error) {
	if a := ActorOf(ctx); a != Human {
		return ProjectView{}, &Error{Code: ActionNotAllowed, Message: fmt.Sprintf(
			"rename_project is a human action; this request is made as %s", a)}
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return ProjectView{}, invalid("display_name", "a Project needs a display name")
	}
	var v ProjectView
	err := c.store.Update(func(w store.Writer) error {
		p, ok := w.Project(key)
		if !ok {
			return unknownProject(key)
		}
		if p.DisplayName == name {
			v = projectView(p)
			return nil
		}
		p.DisplayName = name
		v = projectView(p)
		return w.PutProject(p)
	})
	return v, err
}

// ProjectRow is one list_projects row: the Project and how many of its
// Tickets need the human.
type ProjectRow struct {
	ProjectView
	NeedsMe int `json:"needs_me"`
}

// ListProjects is list_projects: every Project, archived ones too, by key.
func (c *Core) ListProjects(context.Context) ([]ProjectRow, error) {
	out := []ProjectRow{}
	err := c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			row := ProjectRow{ProjectView: projectView(p)}
			for _, t := range r.Tickets(p.Key) {
				if reason, _ := c.reasonOf(r, p.Key, t); reason != "" {
					row.NeedsMe++
				}
			}
			out = append(out, row)
		}
		return nil
	})
	return out, err
}
