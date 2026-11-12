package core

import (
	"context"
	"strings"
	"time"

	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// The base a Factory reports under Pull Request (code-central-394): on
// each poll it fetches the Push remote and fast-forwards the local base,
// then says whether the base is current or has diverged. Code Central
// keeps the latest report of each Project in memory only, like the
// Heartbeat, and drops it when the Project's Factories all go offline.

// BaseState is the state of the local base a Factory reports.
type BaseState string

const (
	BaseCurrent  BaseState = "current"
	BaseDiverged BaseState = "diverged"
	// BaseMissing is a base that exists neither locally nor on the Push
	// remote; it carries no commits.
	BaseMissing BaseState = "missing"
)

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
// base branch.
type BaseDivergedView struct {
	Base   string `json:"base,omitempty"`
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

// BaseMissingView is a Base missing item's detail: the branch and the fix.
type BaseMissingView struct {
	Base string `json:"base"`
	Fix  string `json:"fix"`
}

// ReportBaseInput is the Factory's report_base(state, local, remote,
// full?).
type ReportBaseInput struct {
	State BaseState `json:"state"`
	// Local and Remote are the local and remote base commits.
	// They are required for current and diverged, and ignored for missing.
	Local  string `json:"local"`
	Remote string `json:"remote"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportBase is the Factory's report_base. The report replaces the
// Project's earlier one, in memory only; Base diverged shows in Needs Me
// while the latest says so and a Factory is still online.
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
	now := c.now()
	hs := c.heartbeats
	hs.mu.Lock()
	prev, seen := hs.base[key]
	hs.base[key] = BaseView{State: in.State, Local: local, Remote: remote, Branch: project.Base(), At: now}
	hs.mu.Unlock()
	if !seen || prev.State != in.State || prev.Local != local || prev.Remote != remote {
		c.events.Publish(events.Event{Type: events.ProjectChanged, Project: key})
	}
	return acknowledgeProject(before, c.projectView(project), in.Full, now)
}

// baseView is project's reported base at now, or nil when none is current:
// no report, or no Factory online or draining. hs.mu is held.
func (hs *heartbeats) baseView(project string, now time.Time) *BaseView {
	b, ok := hs.base[project]
	if !ok {
		return nil
	}
	for k, h := range hs.all {
		if k.project == project && h.status(now) != FactoryOffline {
			return &b
		}
	}
	return nil
}

// divergedBase is project's reported base while it is diverged and the
// Project is under Pull Request or Merge + Push.
func (c *Core) divergedBase(p store.Project) (BaseView, bool) {
	if !p.LandingPolicy().KeepsBaseLevel() || !p.Archived.IsZero() {
		return BaseView{}, false
	}
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	b := hs.baseView(p.Key, c.clock.Now())
	if b == nil || b.State != BaseDiverged {
		return BaseView{}, false
	}
	return *b, true
}

// baseDivergedRow is p's Base diverged item: one per Project, with no
// Ticket.
func (c *Core) baseDivergedRow(p store.Project) (NeedsMeRow, bool) {
	b, ok := c.divergedBase(p)
	if !ok {
		return NeedsMeRow{}, false
	}
	return NeedsMeRow{
		Project: p.Key, Title: "Base diverged", Reason: ReasonBaseDiverged, Since: b.At,
		BaseDiverged: &BaseDivergedView{Base: p.Base(), Local: b.Local, Remote: b.Remote},
	}, true
}

// missingBase is project's reported base while it is missing, under any
// Landing Method, and the Project isn't archived.
func (c *Core) missingBase(p store.Project) (BaseView, bool) {
	if !p.Archived.IsZero() {
		return BaseView{}, false
	}
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	b := hs.baseView(p.Key, c.clock.Now())
	if b == nil || b.State != BaseMissing {
		return BaseView{}, false
	}
	return *b, true
}

// baseMissingRow is p's Base missing item: one per Project, with no
// Ticket.
func (c *Core) baseMissingRow(p store.Project) (NeedsMeRow, bool) {
	b, ok := c.missingBase(p)
	if !ok {
		return NeedsMeRow{}, false
	}
	base := p.Base()
	return NeedsMeRow{
		Project: p.Key, Title: "Base missing", Reason: ReasonBaseMissing, Since: b.At,
		BaseMissing: &BaseMissingView{Base: base, Fix: "Create the branch " + base + ", or change Base branch in Project Options"},
	}, true
}
