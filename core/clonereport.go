package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The Factory's clone report (Spec code-central-848): on each poll the
// Factory clones each Repository it has no clone of, adopts a matching
// one, and reports each Repository's clone as ready or missing. Code
// Central keeps the latest report of each Project with the base report,
// where every service sharing the Store reads it, and drops it when the
// Project's Factories all go offline.

// ReportRepositories is the Factory's report_repositories. The report
// replaces the Project's earlier one, in the runtime state a Store shares; a missing
// Repository shows in Needs Me while the latest says so and a Factory is
// still online.
func (c *Core) ReportRepositories(_ context.Context, key string, in api.ReportRepositoriesInput) (api.Written[api.ProjectView], error) {
	now := c.now()
	report := map[string]api.CloneView{}
	for _, r := range in.Repositories {
		name, path, problem := strings.TrimSpace(r.Name), strings.TrimSpace(r.Path), strings.TrimSpace(r.Problem)
		switch {
		case name == "":
			return api.Written[api.ProjectView]{}, api.InvalidField("repositories", "each Repository needs its name")
		case path == "":
			return api.Written[api.ProjectView]{}, api.InvalidField("repositories", "Repository %s needs the path of its clone", name)
		case !slices.Contains(api.CloneStates(), r.State):
			return api.Written[api.ProjectView]{}, api.InvalidField("repositories", "Repository %s's state is ready or missing, not %q", name, r.State)
		case r.State == api.CloneMissing && problem == "":
			return api.Written[api.ProjectView]{}, api.InvalidField("repositories", "Repository %s is missing, so needs the problem", name)
		}
		if _, dup := report[name]; dup {
			return api.Written[api.ProjectView]{}, api.InvalidField("repositories", "Repository %s is reported twice", name)
		}
		if r.State == api.CloneReady {
			problem = ""
		}
		report[name] = api.CloneView{State: r.State, Path: path, Problem: problem, Since: now}
	}
	var before api.ProjectView
	var project domain.Project
	if err := c.store.View(func(r store.Reader) error {
		if err := foundation.Writable(r, key); err != nil {
			return err
		}
		project, _ = r.Project(key)
		before = c.projectView(project)
		return nil
	}); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	if c.factories.RecordClones(key, report, now) {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return acknowledgeProject(before, c.projectView(project), in.Full, now)
}

// fillClones fills in each of rs's clones from the latest clone report of
// the Project key.
func (c *Core) fillClones(key string, rs []api.RepositoryView) {
	now := c.clock.Now()
	for i := range rs {
		if v, ok := c.factories.ReportedClone(key, rs[i].Name, now); ok {
			rs[i].Clone = &v
		}
	}
}

// repositoryMissingRows are p's Repository missing items: one per listed
// Repository its Factory reports missing, in landing order, with no
// Ticket.
func (c *Core) repositoryMissingRows(p domain.Project) []api.NeedsMeRow {
	if !p.Archived.IsZero() {
		return nil
	}
	var out []api.NeedsMeRow
	rs := repositoriesView(p)
	c.fillClones(p.Key, rs)
	for _, r := range rs {
		if r.Clone == nil || r.Clone.State != api.CloneMissing {
			continue
		}
		var fix string
		if r.CloneURL == "" {
			// Local-only: the Factory never clones it.
			fix = fmt.Sprintf("Put Repository %s's git repository at %s, or set its local path to where it is in Project Options", r.Name, r.Clone.Path)
		} else {
			fix = fmt.Sprintf("Clone %s to %s by hand, set Repository %s's local path to a clone of it in Project Options, or move what's at %s aside for the Factory to clone it there", r.CloneURL, r.Clone.Path, r.Name, r.Clone.Path)
		}
		out = append(out, api.NeedsMeRow{
			Project: p.Key, Title: "Repository missing: " + r.Name, Reason: api.ReasonRepositoryMissing, Since: r.Clone.Since,
			RepositoryMissing: &api.RepositoryMissingView{Repository: r.Name, Path: r.Clone.Path, CloneURL: r.CloneURL, Problem: r.Clone.Problem, Fix: fix},
		})
	}
	return out
}
