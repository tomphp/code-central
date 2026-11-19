package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/tomoram/code-central/api"
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

// autoFixMarkView is m's view in p, nil for none.
func autoFixMarkView(p domain.Project, m *domain.AutoFixMark) *api.AutoFixMarkView {
	if m == nil {
		return nil
	}
	v := &api.AutoFixMarkView{Attempt: m.Attempt, Commit: m.Commit, Covers: displayIDs(p.Key, m.Covers)}
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
// commit already exists, record that one. Its body is written only when
// the sweep opens the fix Ticket (autoFixBody), and a stopped item's mark
// has the attempt past the limit.
type autoFixPlan struct {
	failed   domain.Ticket
	name     string
	where    string
	body     string
	mark     domain.AutoFixMark
	existing int
}

// autoFixStatus is what Auto-fix makes of a CI item.
type autoFixStatus int

const (
	// autoFixLeaves leaves the item to the human: Auto-fix doesn't act on
	// it.
	autoFixLeaves autoFixStatus = iota
	// autoFixReady is an Auto-fix ready item, which the sweep acts on.
	autoFixReady
	// autoFixStopped is an item that would be Auto-fix ready but that its
	// next attempt is past the limit: it stays in Needs Me, with the
	// "Auto-fix stopped" line.
	autoFixStopped
)

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
			if _, status := e.planAutoFix(it); status == autoFixReady {
				ready = append(ready, itemKey{it.repository, it.commit})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	ctx = api.WithActor(ctx, api.Integration)
	for _, k := range ready {
		// An item a Ticket's move changed under the sweep is skipped; a
		// fix Ticket it opened by then is recorded once its item is ready
		// again.
		var e *api.Error
		if err := c.autoFixItem(ctx, key, k); err != nil && !(errors.As(err, &e) && e.Code == api.ActionNotAllowed) {
			return err
		}
	}
	return nil
}

// autoFixItem carries out k's plan, read afresh: nothing when k is no
// longer Auto-fix ready.
func (c *Core) autoFixItem(ctx context.Context, key string, k itemKey) error {
	var plan autoFixPlan
	status := autoFixLeaves
	err := c.store.View(func(r store.Reader) error {
		p, _ := r.Project(key)
		e := c.evaluate(r, p)
		for _, it := range e.evaluated().ciItems {
			if it.repository == k.name && it.commit == k.commit {
				if plan, status = e.planAutoFix(it); status == autoFixReady && plan.existing == 0 {
					plan.body = e.autoFixBody(it, plan.mark)
				}
			}
		}
		return nil
	})
	if err != nil || status != autoFixReady {
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

// planAutoFix is what Auto-fix makes of it, with its plan, but for the
// body, unless it leaves it.
//
// It is Auto-fix ready while Auto-fix acts and it is CI failed and
// offering New fix Ticket, in a Repository not under Pull Request, whose
// CI failures stay the human's (Spec code-central-1022, User Story 29),
// with no Land anyway, its commit's response naming no fix Ticket (a
// manual, Done or Cancelled one leaves the commit to the human), and its
// next attempt within the limit; past the limit it is stopped.
//
// A Cancelled auto fix Ticket hands its failure back, on any commit that
// carries it, so the Tickets it covered don't start again at attempt 1.
// An open one covering it keeps it out of the evaluation's items, so the
// sweep never reaches it. One already marked for its commit, left
// unrecorded by a crash between the two writes, is recorded whatever the
// limit.
func (e *evaluation) planAutoFix(it ciItem) (autoFixPlan, autoFixStatus) {
	p := e.p
	if !autoFixActs(p) || it.reason != api.ReasonCIFailed || !slices.Contains(ciItemOptions(e.r, p, it), api.OptionNewFixTicket) || landingPolicyIn(p, it.repository).LandsByPullRequest() {
		return autoFixPlan{}, autoFixLeaves
	}
	record := recordName(p.Key, it.repository)
	if res := responseOf(p, record, it.commit); !res.LandAnyway.IsZero() || res.FixTicket != 0 {
		return autoFixPlan{}, autoFixLeaves
	}
	plan := autoFixPlan{failed: it.tickets[0], name: it.repository, where: inRepository(p, it.repository)}
	tickets := e.tickets()
	if i := slices.IndexFunc(tickets, func(t domain.Ticket) bool {
		return t.AutoFix != nil && t.AutoFix.Repository == record && t.AutoFix.Commit == it.commit
	}); i >= 0 {
		plan.existing = tickets[i].Number
		return plan, autoFixReady
	}
	k := itemKey{it.repository, it.commit}
	if slices.ContainsFunc(tickets, func(t domain.Ticket) bool { return t.State == domain.Cancelled && e.ci.carriesFailureOf(t, k) }) {
		return autoFixPlan{}, autoFixLeaves
	}
	mark := domain.AutoFixMark{Attempt: 1, Repository: record, Commit: it.commit}
	for _, t := range it.tickets {
		mark.Covers = append(mark.Covers, t.Number)
		// The next attempt is one more than the highest listed.
		if t.AutoFix != nil && t.AutoFix.Attempt+1 > mark.Attempt {
			mark.Attempt, mark.Previous = t.AutoFix.Attempt+1, t.Number
		}
	}
	plan.mark = mark
	if mark.Attempt > p.CI.AutoFix.AttemptLimit() {
		return plan, autoFixStopped
	}
	return plan, autoFixReady
}

// autoFixBody is the body of the fix Ticket Auto-fix opens for it with
// mark: the Run summary, the Fix instructions and its attempt of the
// limit.
func (e *evaluation) autoFixBody(it ciItem, mark domain.AutoFixMark) string {
	return e.newFixNote(it) + fmt.Sprintf("\n\nAuto-fix attempt %d of %d.", mark.Attempt, e.p.CI.AutoFix.AttemptLimit())
}
