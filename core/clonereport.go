package core

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The Factory's clone report (Spec code-central-848): on each poll the
// Factory clones each Repository it has no clone of, adopts a matching
// one, and reports each Repository's clone as ready or missing. Code
// Central keeps the latest report of each Project in memory only, like
// the base report, and drops it when the Project's Factories all go
// offline.

// ReportRepositories is the Factory's report_repositories. The report
// replaces the Project's earlier one, in memory only; a missing
// Repository shows in Needs Me while the latest says so and a Factory is
// still online.
func (c *Core) ReportRepositories(_ context.Context, key string, in ReportRepositoriesInput) (Written[ProjectView], error) {
	now := c.now()
	report := map[string]CloneView{}
	for _, r := range in.Repositories {
		name, path, problem := strings.TrimSpace(r.Name), strings.TrimSpace(r.Path), strings.TrimSpace(r.Problem)
		switch {
		case name == "":
			return Written[ProjectView]{}, invalid("repositories", "each Repository needs its name")
		case path == "":
			return Written[ProjectView]{}, invalid("repositories", "Repository %s needs the path of its clone", name)
		case !slices.Contains(CloneStates(), r.State):
			return Written[ProjectView]{}, invalid("repositories", "Repository %s's state is ready or missing, not %q", name, r.State)
		case r.State == CloneMissing && problem == "":
			return Written[ProjectView]{}, invalid("repositories", "Repository %s is missing, so needs the problem", name)
		}
		if _, dup := report[name]; dup {
			return Written[ProjectView]{}, invalid("repositories", "Repository %s is reported twice", name)
		}
		if r.State == CloneReady {
			problem = ""
		}
		report[name] = CloneView{State: r.State, Path: path, Problem: problem, Since: now}
	}
	var before ProjectView
	var project domain.Project
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		project, _ = r.Project(key)
		before = c.projectView(project)
		return nil
	}); err != nil {
		return Written[ProjectView]{}, err
	}
	hs := c.heartbeats
	hs.mu.Lock()
	prev := hs.clones[key]
	for name, v := range report {
		// A clone in the same state, with the same problem, is still
		// waiting since it was first reported.
		if p, ok := prev[name]; ok && p.State == v.State && p.Path == v.Path && p.Problem == v.Problem {
			report[name] = p
		}
	}
	hs.clones[key] = report
	hs.mu.Unlock()
	if !maps.Equal(prev, report) {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return acknowledgeProject(before, c.projectView(project), in.Full, now)
}

// cloneView is the clone of project's Repository name at now, or nil
// when none is current: no report of it, or no Factory online or
// draining. hs.mu is held.
func (hs *heartbeats) cloneView(project, name string, now time.Time) *CloneView {
	v, ok := hs.clones[project][name]
	if !ok || !hs.reporting(project, now) {
		return nil
	}
	return &v
}

// fillClones fills in each of rs's clones from the latest clone report of
// the Project key.
func (c *Core) fillClones(key string, rs []RepositoryView) {
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	now := c.clock.Now()
	for i := range rs {
		rs[i].Clone = hs.cloneView(key, rs[i].Name, now)
	}
}

// repositoryMissingRows are p's Repository missing items: one per listed
// Repository its Factory reports missing, in landing order, with no
// Ticket.
func (c *Core) repositoryMissingRows(p domain.Project) []NeedsMeRow {
	if !p.Archived.IsZero() {
		return nil
	}
	var out []NeedsMeRow
	rs := repositoriesView(p)
	c.fillClones(p.Key, rs)
	for _, r := range rs {
		if r.Clone == nil || r.Clone.State != CloneMissing {
			continue
		}
		var fix string
		if r.CloneURL == "" {
			// Local-only: the Factory never clones it.
			fix = fmt.Sprintf("Put Repository %s's git repository at %s, or set its local path to where it is in Project Options", r.Name, r.Clone.Path)
		} else {
			fix = fmt.Sprintf("Clone %s to %s by hand, set Repository %s's local path to a clone of it in Project Options, or move what's at %s aside for the Factory to clone it there", r.CloneURL, r.Clone.Path, r.Name, r.Clone.Path)
		}
		out = append(out, NeedsMeRow{
			Project: p.Key, Title: "Repository missing: " + r.Name, Reason: ReasonRepositoryMissing, Since: r.Clone.Since,
			RepositoryMissing: &RepositoryMissingView{Repository: r.Name, Path: r.Clone.Path, CloneURL: r.CloneURL, Problem: r.Clone.Problem, Fix: fix},
		})
	}
	return out
}
