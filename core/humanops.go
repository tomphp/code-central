package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The human's own operations beside Needs Me (ticket 32): moving a
// Ticket by hand, renaming a Project and listing every Project.

// MoveInput is the human's move(ticket, to_state).
type MoveInput struct {
	Ticket  Ref         `json:"ticket"`
	ToState store.State `json:"to_state"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// Move is the human's move(to_state): a Ticket goes to any State but its
// own, Needs Answer and Failed. Leaving a Claim State ends the Claim;
// leaving Needs Answer also dismisses its open Questions.
func (c *Core) Move(ctx context.Context, key string, in MoveInput) (Written[TicketView], error) {
	return c.updateTicket(key, "move", in.Ticket, in.Full, func(t *store.Ticket) (bool, error) {
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
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		if p.DisplayName == name {
			v = c.projectView(p)
			return nil
		}
		p.DisplayName = name
		v = c.projectView(p)
		return w.PutProject(p)
	})
	return v, err
}

// SetLandingMethodInput is the human's set_landing_method(method, push,
// remote, land_per_spec).
type SetLandingMethodInput struct {
	Method store.LandingMethod `json:"method"`
	Push   bool                `json:"push"`
	Remote string              `json:"remote"`
	// LandPerSpec, when left out, keeps the stored value.
	LandPerSpec *bool `json:"land_per_spec,omitempty"`
	// PATEnv names the environment variable holding the Azure Repos PAT
	// under Pull Request; left out, it keeps the stored name.
	PATEnv string `json:"pat_env,omitempty"`
	// Base is the branch work is cut from and lands on; left out, it keeps
	// the stored one. Only its name is checked, never whether it exists.
	Base *string `json:"base,omitempty"`
}

// SetLandingMethod is the human's set_landing_method: how the Project's
// accepted work lands. Code Central never sees git, so whether the remote
// exists is the Factory's check.
func (c *Core) SetLandingMethod(ctx context.Context, key string, in SetLandingMethodInput) (ProjectView, error) {
	if err := humanOnly(ctx, "set_landing_method"); err != nil {
		return ProjectView{}, err
	}
	if !in.Method.Valid() {
		return ProjectView{}, invalid("method", "Landing Method %q is not %q, %q or %q", in.Method, store.LandingMerge, store.LandingBranch, store.LandingPullRequest)
	}
	// Push is implied under Pull Request, so the flag is ignored and not stored.
	l := store.Landing{Method: in.Method, Push: in.Push, Remote: strings.TrimSpace(in.Remote)}.Normalized()
	if l.Remote == "" {
		if l.Push {
			return ProjectView{}, invalid("remote", "Push needs a git remote to push to, such as %q", store.DefaultLandingRemote)
		}
		l.Remote = store.DefaultLandingRemote
	}
	patEnv := strings.TrimSpace(in.PATEnv)
	if patEnv != "" && !patEnvPattern.MatchString(patEnv) {
		return ProjectView{}, invalid("pat_env", "%q is not an environment variable's name, such as %s", patEnv, store.DefaultPATEnv)
	}
	if in.Base != nil {
		if err := checkBranchName(strings.TrimSpace(*in.Base)); err != nil {
			return ProjectView{}, invalid("base", "%s", err)
		}
	}
	if err := c.checkPullRequest(ctx, key, store.Landing{Method: in.Method, PATEnv: patEnv}); err != nil {
		return ProjectView{}, err
	}
	var v ProjectView
	var accepted, left []int
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		l.LandPerSpec = p.Landing.LandPerSpec
		l.PATEnv = p.Landing.PATEnv
		l.Base = p.Landing.Base
		if in.Base != nil {
			l.Base = strings.TrimSpace(*in.Base)
		}
		if patEnv != "" {
			l.PATEnv = patEnv
		}
		if l.Method == store.LandingPullRequest {
			if err := checkPullRequestHost(p.CI.PushRemoteURL); err != nil {
				return err
			}
		}
		if in.LandPerSpec != nil {
			l.LandPerSpec = *in.LandPerSpec
		}
		if p.Landing == l {
			v = c.projectView(p)
			return nil
		}
		if p.Landing.Method != l.Method {
			for _, t := range w.Tickets(key) {
				if t.State == store.Accepted {
					accepted = append(accepted, t.Number)
				}
			}
		}
		if p.Landing.Method == store.LandingPullRequest && l.Method != store.LandingPullRequest {
			// What the Pull Request Watcher said was about the old method.
			c.watchers.reset(watcherKey{WatcherPullRequests, key})
			for _, t := range w.Tickets(key) {
				if t.State == store.Accepted && pullRequestOpen(t) {
					left = append(left, t.Number)
				}
			}
		}
		p.Landing = l
		v = c.projectView(p)
		return w.PutProject(p)
	})
	if err != nil {
		return v, err
	}
	// The pull requests stay on the host, so each Ticket says to close its own.
	for _, n := range left {
		if err := c.store.Update(func(w store.Writer) error {
			t, ok := w.Ticket(key, n)
			if !ok || t.PullRequest == nil {
				return nil
			}
			addComment(&t, Human, c.now(), 0, fmt.Sprintf("The Landing Method is no longer Pull Request, so its pull request %s is left open on the host. Close it there when you no longer want it.", t.PullRequest.Link))
			t.Updated = c.now()
			return w.PutTicket(key, t)
		}); err != nil {
			return v, err
		}
	}
	if err := c.store.View(func(r store.Reader) error {
		v.Landing.OpenPullRequests = openPullRequests(r, key, v.Landing.Method)
		return nil
	}); err != nil {
		return v, err
	}
	// The method decides an Accepted Ticket's Needs Me reason (To merge,
	// or Accepted but not landed), so each one changed without a write.
	for _, n := range append(accepted, left...) {
		c.events.Publish(events.Event{Type: events.TicketChanged, Project: key, ID: DisplayID(key, n)})
	}
	return v, nil
}

// ProjectRow is one list_projects row: the Project and how many of its
// Tickets need the human.
type ProjectRow struct {
	ProjectView
	NeedsMe int `json:"needs_me"`
	// Unfinished counts the Tickets not yet Done or Cancelled; archiving
	// waits for it to reach zero.
	Unfinished int `json:"unfinished"`
}

// ListProjects is list_projects: every Project, archived ones too, by key.
func (c *Core) ListProjects(context.Context) ([]ProjectRow, error) {
	out := []ProjectRow{}
	err := c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			row := ProjectRow{ProjectView: c.projectView(p)}
			row.Landing.OpenPullRequests = openPullRequests(r, p.Key, p.Landing.Method)
			if _, ok := c.baseDivergedRow(p); ok {
				row.NeedsMe++
			}
			if _, ok := c.baseMissingRow(p); ok {
				row.NeedsMe++
			}
			for _, t := range r.Tickets(p.Key) {
				if reason, _ := c.reasonOf(r, p.Key, t); reason != "" {
					row.NeedsMe++
				}
				if !finished(t.State) {
					row.Unfinished++
				}
			}
			out = append(out, row)
		}
		return nil
	})
	return out, err
}
