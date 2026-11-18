package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The base a Factory reports under Pull Request (code-central-394) and
// Merge + Push (code-central-425): on each poll it fetches the Push
// remote and brings the local base level with the remote's, then says
// whether the base is current or has diverged. Code Central
// keeps the latest report of each Project in memory only, like the
// Heartbeat, and drops it when the Project's Factories all go offline.
// With several Repositories (Spec code-central-848) it reports each
// Repository's base apart, and keeps the latest report of each.

// BaseState is the state of the local base a Factory reports.
type BaseState string

const (
	BaseCurrent  BaseState = "current"
	BaseDiverged BaseState = "diverged"
	// BaseMissing is a base that exists neither locally nor on the Push
	// remote, or in a local-only Repository, never looked for on a
	// remote, doesn't exist locally; it carries no commits.
	BaseMissing BaseState = "missing"
)

// BaseStates are the states of a reported base.
func BaseStates() []BaseState { return []BaseState{BaseCurrent, BaseDiverged, BaseMissing} }

// BaseView is a Project's reported base, as its Factory Status carries it;
// absent while no Factory is reporting.
type BaseView struct {
	State BaseState `json:"state"`
	// Local and Remote are the local and remote base commits.
	Local  string `json:"local"`
	Remote string `json:"remote"`
	// Branch is the base branch's name, when the Factory reported it.
	Branch string    `json:"branch,omitempty"`
	At     time.Time `json:"at"`
}

// BaseDivergedView is a Base diverged item's detail: both commits and the
// base branch, and the Repository whose base it is while the Project has
// several.
type BaseDivergedView struct {
	Repository string `json:"repository,omitempty"`
	Base       string `json:"base,omitempty"`
	Local      string `json:"local"`
	Remote     string `json:"remote"`
}

// BaseMissingView is a Base missing item's detail: the branch and the fix,
// and the Repository whose base it is while the Project has several.
type BaseMissingView struct {
	Repository string `json:"repository,omitempty"`
	Base       string `json:"base"`
	Fix        string `json:"fix"`
}

// ReportBaseInput is the Factory's report_base(state, local, remote,
// repository?, full?).
type ReportBaseInput struct {
	State BaseState `json:"state"`
	// Local and Remote are the local and remote base commits.
	// They are required for current and diverged, and ignored for missing.
	Local  string `json:"local"`
	Remote string `json:"remote"`
	// Repository is the Repository Name whose base this is; blank for the
	// Home Repository, as a Factory from before Repositories reports it
	// (Spec code-central-848).
	Repository string `json:"repository,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportBase is the Factory's report_base. The report replaces the
// Repository's earlier one, in memory only, each Repository's base kept
// apart; Base diverged and Base missing show in Needs Me, one item per
// Repository, while the latest says so and a Factory is still online.
func (c *Core) ReportBase(_ context.Context, key string, in ReportBaseInput) (Written[ProjectView], error) {
	local, remote := strings.TrimSpace(in.Local), strings.TrimSpace(in.Remote)
	if in.State == BaseMissing {
		local, remote = "", ""
	}
	switch {
	case in.State != BaseCurrent && in.State != BaseDiverged && in.State != BaseMissing:
		return Written[ProjectView]{}, invalid("state", "state is current, diverged or missing, not %q", in.State)
	case in.State != BaseMissing && local == "":
		return Written[ProjectView]{}, invalid("local", "local is required: the local base commit")
	case in.State != BaseMissing && remote == "":
		return Written[ProjectView]{}, invalid("remote", "remote is required: the remote base commit")
	}
	var before ProjectView
	var project store.Project
	var repository store.Repository
	if err := c.store.View(func(r store.Reader) error {
		if err := writable(r, key); err != nil {
			return err
		}
		project, _ = r.Project(key)
		var err error
		if repository, err = repositoryField(project, in.Repository); err != nil {
			return err
		}
		before = c.projectView(project)
		return nil
	}); err != nil {
		return Written[ProjectView]{}, err
	}
	now := c.now()
	hs := c.heartbeats
	hs.mu.Lock()
	if hs.base[key] == nil {
		hs.base[key] = map[string]BaseView{}
	}
	prev, seen := hs.base[key][repository.Name]
	hs.base[key][repository.Name] = BaseView{State: in.State, Local: local, Remote: remote, Branch: baseOfRepository(project, repository), At: now}
	hs.mu.Unlock()
	if !seen || prev.State != in.State || prev.Local != local || prev.Remote != remote {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return acknowledgeProject(before, c.projectView(project), in.Full, now)
}

// repositoryField is the Repository of p that an operation's optional
// repository field names: name, or the Home Repository for none. It
// refuses a name p hasn't, as invalid in that field.
func repositoryField(p store.Project, name string) (store.Repository, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = p.Key
	}
	rs := p.RepositoryList()
	i := slices.IndexFunc(rs, func(r store.Repository) bool { return r.Name == name })
	if i < 0 {
		return store.Repository{}, invalid("repository", "%q is not one of this Project's Repositories, which are %s", name, strings.Join(repositoryNames(rs), ", "))
	}
	return rs[i], nil
}

// baseOfRepository is r's Base: the Project's for the Home Repository.
func baseOfRepository(p store.Project, r store.Repository) string {
	if r.Home {
		return p.Base()
	}
	return r.Base
}

// baseView is the reported base of project's Repository name at now, or
// nil when none is current: no report, or no Factory online or draining.
// hs.mu is held.
func (hs *heartbeats) baseView(project, name string, now time.Time) *BaseView {
	b, ok := hs.base[project][name]
	if !ok || !hs.reporting(project, now) {
		return nil
	}
	return &b
}

// reporting reports whether a Factory of project's is online or draining
// at now, so its reports stand. hs.mu is held.
func (hs *heartbeats) reporting(project string, now time.Time) bool {
	for k, h := range hs.all {
		if k.project == project && h.status(now) != FactoryOffline {
			return true
		}
	}
	return false
}

// reportedBase is the reported base of p's Repository r while it is in
// state and p isn't archived.
func (c *Core) reportedBase(p store.Project, r store.Repository, state BaseState) (BaseView, bool) {
	if !p.Archived.IsZero() {
		return BaseView{}, false
	}
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	b := hs.baseView(p.Key, r.Name, c.clock.Now())
	if b == nil || b.State != state {
		return BaseView{}, false
	}
	return *b, true
}

// divergedBase is the reported base of p's Repository r while it is
// diverged and r's Landing Policy is Pull Request or Merge + Push: never
// in a local-only Repository.
func (c *Core) divergedBase(p store.Project, r store.Repository) (BaseView, bool) {
	if !landingPolicyIn(p, r.Name).KeepsBaseLevel() {
		return BaseView{}, false
	}
	return c.reportedBase(p, r, BaseDiverged)
}

// baseDivergedRows are p's Base diverged items: one per Repository whose
// base has diverged, in landing order, each with no Ticket.
func (c *Core) baseDivergedRows(p store.Project) []NeedsMeRow {
	var out []NeedsMeRow
	for _, r := range p.RepositoryList() {
		if b, ok := c.divergedBase(p, r); ok {
			out = append(out, NeedsMeRow{
				Project: p.Key, Title: "Base diverged" + baseItemOf(p, r), Reason: ReasonBaseDiverged, Since: b.At,
				BaseDiverged: &BaseDivergedView{Repository: baseRepository(p, r), Base: baseOfRepository(p, r), Local: b.Local, Remote: b.Remote},
			})
		}
	}
	return out
}

// baseMissingRows are p's Base missing items: one per Repository whose
// base is missing, under any Landing Method, in landing order, each with
// no Ticket.
func (c *Core) baseMissingRows(p store.Project) []NeedsMeRow {
	var out []NeedsMeRow
	for _, r := range p.RepositoryList() {
		b, ok := c.reportedBase(p, r, BaseMissing)
		if !ok {
			continue
		}
		base := baseOfRepository(p, r)
		fix := "Create the branch " + base + ", or change Base branch in Project Options"
		if !r.Home {
			fix = fmt.Sprintf("Create the branch %s in Repository %s, or change its Base in Project Options", base, r.Name)
		}
		out = append(out, NeedsMeRow{
			Project: p.Key, Title: "Base missing" + baseItemOf(p, r), Reason: ReasonBaseMissing, Since: b.At,
			BaseMissing: &BaseMissingView{Repository: baseRepository(p, r), Base: base, Fix: fix},
		})
	}
	return out
}

// baseRepository is the Repository a Base item of p's names: r, while p
// has several, as only then do reads name Repositories; "" otherwise.
func baseRepository(p store.Project, r store.Repository) string {
	if !severalRepositories(p) {
		return ""
	}
	return r.Name
}

// baseItemOf ends a Base item's title with its Repository, while p has
// several.
func baseItemOf(p store.Project, r store.Repository) string {
	if name := baseRepository(p, r); name != "" {
		return ": " + name
	}
	return ""
}

// brokenBase says why the base of p's Repository name holds back the
// work touching it, while its Factory reports it missing or diverged; ""
// otherwise.
func (c *Core) brokenBase(p store.Project, name string) string {
	r, err := repositoryField(p, name)
	if err != nil {
		return ""
	}
	if _, ok := c.reportedBase(p, r, BaseMissing); ok {
		return fmt.Sprintf("its base, %s, is missing", baseOfRepository(p, r))
	}
	if _, ok := c.divergedBase(p, r); ok {
		return fmt.Sprintf("its base, %s, has diverged from the remote's", baseOfRepository(p, r))
	}
	return ""
}
