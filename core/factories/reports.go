package factories

import (
	"maps"
	"time"

	"github.com/tomoram/code-central/api"
)

// FactoryStatus is project's Factory Status at now.
func (c *Factories) FactoryStatus(project string, now time.Time) api.FactoryStatusView {
	return c.heartbeats.view(project, c.f.Service().ID, now)
}

// RecordBase keeps the reported base of project's Repository name as of
// now, and tells whether it differs from the last one reported, in what the
// human sees of it.
func (c *Factories) RecordBase(project, name string, v api.BaseView) (changed bool) {
	hs := c.heartbeats
	hs.mu.Lock()
	if hs.base[project] == nil {
		hs.base[project] = map[string]api.BaseView{}
	}
	prev, seen := hs.base[project][name]
	hs.base[project][name] = v
	hs.reportsAt[project] = v.At
	hs.mu.Unlock()
	c.recordReports(project)
	return !seen || prev.State != v.State || prev.Local != v.Local || prev.Remote != v.Remote
}

// RecordClones replaces project's reported clones, by Repository name, as
// of now, and tells whether they differ from the last ones reported. A
// clone in the same state, with the same problem, is still waiting since
// it was first reported.
func (c *Factories) RecordClones(project string, report map[string]api.CloneView, now time.Time) (changed bool) {
	hs := c.heartbeats
	hs.mu.Lock()
	prev := hs.clones[project]
	for name, v := range report {
		if p, ok := prev[name]; ok && p.State == v.State && p.Path == v.Path && p.Problem == v.Problem {
			report[name] = p
		}
	}
	hs.clones[project] = report
	hs.reportsAt[project] = now
	hs.mu.Unlock()
	c.recordReports(project)
	return !maps.Equal(prev, report)
}

// ReportedBase is the reported base of project's Repository name at now,
// or false when none is current: no report, or no Factory online or
// draining.
func (c *Factories) ReportedBase(project, name string, now time.Time) (api.BaseView, bool) {
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	b, ok := hs.base[project][name]
	if !ok || !hs.reporting(project, now) {
		return api.BaseView{}, false
	}
	return b, true
}

// ReportedClone is the clone of project's Repository name at now, or false
// when none is current: no report of it, or no Factory online or draining.
func (c *Factories) ReportedClone(project, name string, now time.Time) (api.CloneView, bool) {
	hs := c.heartbeats
	hs.mu.Lock()
	defer hs.mu.Unlock()
	v, ok := hs.clones[project][name]
	if !ok || !hs.reporting(project, now) {
		return api.CloneView{}, false
	}
	return v, true
}

// reporting reports whether a Factory of project's is online or draining
// at now, so its reports stand. hs.mu is held.
func (hs *heartbeats) reporting(project string, now time.Time) bool {
	for k, h := range hs.all {
		if k.project == project && h.status(now) != api.FactoryOffline {
			return true
		}
	}
	return false
}

// baseView is the reported base of project's Repository name at now, or
// nil when none is current. hs.mu is held.
func (hs *heartbeats) baseView(project, name string, now time.Time) *api.BaseView {
	b, ok := hs.base[project][name]
	if !ok || !hs.reporting(project, now) {
		return nil
	}
	return &b
}
