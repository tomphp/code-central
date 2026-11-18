package core

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Auto-fix opening fix Tickets (Spec code-central-1022, ADR 0066): with
// Auto-fix on, one core sweep per Project opens a fix Ticket on the CI
// Fixes Spec for each Auto-fix ready CI failed item, on the human's
// behalf. It runs after each write that can make an item ready:
// report_ci_runs, report_pushed_commit, set_auto_fix and cancel, so it
// acts on items already waiting too. Recording the fix Ticket on the
// commit's response keeps the item out of Needs Me, as for a manual one.

// AutoFixMarkView is an auto fix Ticket's Auto-fix mark as its view
// carries it.
type AutoFixMarkView struct {
	// Attempt is its Auto-fix attempt, 1 for the first in its chain.
	Attempt int `json:"attempt"`
	// Repository is the Repository Name of the commit it was opened for,
	// left out while the Project has the Home Repository alone.
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit"`
	// Covers are the Tickets its CI failed item listed when it was
	// opened.
	Covers []string `json:"covers"`
	// Previous is the previous attempt's fix Ticket, left out for attempt
	// 1.
	Previous string `json:"previous,omitempty"`
}

// autoFixMarkView is m's view in p, nil for none.
func autoFixMarkView(p domain.Project, m *domain.AutoFixMark) *AutoFixMarkView {
	if m == nil {
		return nil
	}
	v := &AutoFixMarkView{Attempt: m.Attempt, Commit: m.Commit, Covers: displayIDs(p.Key, m.Covers)}
	if severalRepositories(p) {
		v.Repository = recordRepository(p.Key, m.Repository)
	}
	if m.Previous != 0 {
		v.Previous = DisplayID(p.Key, m.Previous)
	}
	return v
}

// lockAutoFix takes key's Auto-fix lock, which serialises its sweeps and
// New fix Ticket, and returns its unlock.
func (c *Core) lockAutoFix(key string) func() {
	l, _ := c.autoFixLocks.LoadOrStore(key, &sync.Mutex{})
	mu := l.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// autoFixPlan is what the sweep does for one Auto-fix ready item: open a
// fix Ticket for failed, the first Ticket it lists, in the Repository
// name with body and mark, or, when an auto fix Ticket marked for its
// commit already exists, record that one.
type autoFixPlan struct {
	failed   domain.Ticket
	name     string
	where    string
	body     string
	mark     domain.AutoFixMark
	existing int
}

// itemKey names one CI item: its Repository Name and pushed commit.
type itemKey struct{ name, commit string }

// sweepAutoFix is the Auto-fix sweep for key: it opens a fix Ticket for
// each Auto-fix ready item, one Project's sweeps one at a time, each item
// read afresh before its writes. Its comments' Actor is integration.
func (c *Core) sweepAutoFix(ctx context.Context, key string) error {
	defer c.lockAutoFix(key)()
	var ready []itemKey
	err := c.store.View(func(r store.Reader) error {
		p, ok := r.Project(key)
		if !ok || !autoFixActs(p) {
			return nil
		}
		e := c.evaluate(r, p)
		for _, it := range e.evaluated().ciItems {
			if _, ok := e.autoFixPlan(it); ok {
				ready = append(ready, itemKey{it.repository, it.commit})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	ctx = WithActor(ctx, Integration)
	for _, k := range ready {
		if err := c.autoFixItem(ctx, key, k); err != nil {
			return err
		}
	}
	return nil
}

// autoFixItem carries out k's plan, read afresh: nothing when k is no
// longer Auto-fix ready.
func (c *Core) autoFixItem(ctx context.Context, key string, k itemKey) error {
	var plan autoFixPlan
	ready := false
	err := c.store.View(func(r store.Reader) error {
		p, _ := r.Project(key)
		if !autoFixActs(p) {
			return nil
		}
		e := c.evaluate(r, p)
		if i := slices.IndexFunc(e.evaluated().ciItems, func(it ciItem) bool { return it.repository == k.name && it.commit == k.commit }); i >= 0 {
			plan, ready = e.autoFixPlan(e.ciItems[i])
		}
		return nil
	})
	if err != nil || !ready {
		return err
	}
	if plan.existing != 0 {
		return c.recordFixTicket(ctx, key, plan.failed, plan.name, plan.existing)
	}
	_, err = c.openFixTicket(ctx, key, plan.failed, plan.name, plan.where, plan.body, &plan.mark, false)
	return err
}

// autoFixActs reports whether Auto-fix acts on p: it is on, and so is CI
// Tracking.
func autoFixActs(p domain.Project) bool {
	return p.CI.AutoFix.On && ciTrackingOn(p.CI)
}

// autoFixPlan is it's plan while it is Auto-fix ready: CI failed and
// offering New fix Ticket, with no Land anyway, its commit's response
// naming no fix Ticket (a manual, Done or Cancelled one leaves the commit
// to the human), and its next attempt within the limit. An auto fix
// Ticket already marked for its commit, left unrecorded by a crash
// between the two writes, is recorded whatever the limit.
func (e *evaluation) autoFixPlan(it ciItem) (autoFixPlan, bool) {
	p := e.p
	if it.reason != ReasonCIFailed || !slices.Contains(ciItemOptions(e.r, p, it), OptionNewFixTicket) {
		return autoFixPlan{}, false
	}
	record := recordName(p.Key, it.repository)
	if res := responseOf(p, record, it.commit); !res.LandAnyway.IsZero() || res.FixTicket != 0 {
		return autoFixPlan{}, false
	}
	plan := autoFixPlan{failed: it.tickets[0], name: it.repository, where: inRepository(p, it.repository)}
	for _, t := range e.tickets() {
		if m := t.AutoFix; m != nil && m.Repository == record && m.Commit == it.commit {
			plan.existing = t.Number
			return plan, true
		}
	}
	mark := domain.AutoFixMark{Attempt: 1, Repository: record, Commit: it.commit}
	for _, t := range it.tickets {
		mark.Covers = append(mark.Covers, t.Number)
		if t.AutoFix != nil && t.AutoFix.Attempt >= mark.Attempt {
			mark.Attempt, mark.Previous = t.AutoFix.Attempt+1, t.Number
		}
	}
	limit := p.CI.AutoFix.AttemptLimit()
	if mark.Attempt > limit {
		return autoFixPlan{}, false
	}
	plan.mark = mark
	plan.body = e.newFixNote(it) + fmt.Sprintf("\n\nAuto-fix attempt %d of %d.", mark.Attempt, limit)
	return plan, true
}
