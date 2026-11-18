package core

import (
	"slices"

	"github.com/tomoram/code-central/domain"
)

// Auto-fix chains its attempts (Spec code-central-1022, ADR 0066): an
// open auto fix Ticket covers the Tickets its CI failed item listed, so
// a later commit carrying them waits on it out of Needs Me, until the
// fix Ticket's own push has a CI result and its item lists it, which is
// the next attempt.

// CIAutoFixView is what Auto-fix does about a Ticket's CI failure in one
// Repository, worked out in Go for the UI to format (ADR 0063): the auto
// fix Ticket fixing it, the one covering it, or the chain Auto-fix
// stopped at the limit. One of them is set.
type CIAutoFixView struct {
	Fixing   *AutoFixFixingView   `json:"fixing,omitempty"`
	Covering *AutoFixCoveringView `json:"covering,omitempty"`
	Stopped  *AutoFixStoppedView  `json:"stopped,omitempty"`
}

// AutoFixFixingView is the auto fix Ticket fixing the failure of a
// Ticket's latest pushed commit: attempt Attempt of the Auto-fix limit,
// Limit.
type AutoFixFixingView struct {
	FixTicket string `json:"fix_ticket"`
	Attempt   int    `json:"attempt"`
	Limit     int    `json:"limit"`
}

// AutoFixCoveringView is the open auto fix Ticket covering a Ticket whose
// later commit carries the failure it fixes, and the commit it was opened
// for.
type AutoFixCoveringView struct {
	FixTicket string `json:"fix_ticket"`
	Commit    string `json:"commit"`
}

// AutoFixStoppedView is the chain Auto-fix stopped at the limit on a CI
// failed item, which comes back to the human: "Auto-fix stopped after N
// attempts", Attempts being the N auto fix Tickets, attempt 1 first.
type AutoFixStoppedView struct {
	Attempts []string `json:"attempts"`
}

// ciAutoFix is what Auto-fix does about t's CI failure in the Repository
// name, nil when it does nothing.
func (e *evaluation) ciAutoFix(t domain.Ticket, name string) *CIAutoFixView {
	rc := e.ci.ticket(t).in(name)
	switch {
	case rc == nil:
		return nil
	case rc.fixing != nil && rc.fixing.AutoFix != nil:
		return &CIAutoFixView{Fixing: &AutoFixFixingView{FixTicket: DisplayID(e.p.Key, rc.fixing.Number), Attempt: rc.fixing.AutoFix.Attempt, Limit: e.p.CI.AutoFix.AttemptLimit()}}
	case rc.covering != nil:
		return &CIAutoFixView{Covering: &AutoFixCoveringView{FixTicket: DisplayID(e.p.Key, rc.covering.Number), Commit: rc.covering.AutoFix.Commit}}
	}
	if it, ok := e.itemIn(t, name); ok {
		if stopped := e.autoFixStopped(it); stopped != nil {
			return &CIAutoFixView{Stopped: stopped}
		}
	}
	return nil
}

// autoFixStopped is it's "Auto-fix stopped" line while Auto-fix has
// stopped at the limit on it, nil otherwise: each attempt, found by
// following the chain back from the highest auto fix Ticket it lists.
func (e *evaluation) autoFixStopped(it ciItem) *AutoFixStoppedView {
	plan, status := e.planAutoFix(it)
	if status != autoFixStopped {
		return nil
	}
	v := &AutoFixStoppedView{Attempts: []string{}}
	// A chain is never longer than its highest attempt.
	for n := plan.mark.Previous; n != 0 && len(v.Attempts) < plan.mark.Attempt; {
		v.Attempts = append(v.Attempts, DisplayID(e.p.Key, n))
		fix, ok := e.r.Ticket(e.p.Key, n)
		if !ok || fix.AutoFix == nil {
			break
		}
		n = fix.AutoFix.Previous
	}
	slices.Reverse(v.Attempts)
	return v
}

// covering is the open auto fix Ticket covering the CI failed item of
// rc's latest pushed commit, if one does: the commit carries the failure
// it was opened for, and the item doesn't list the fix Ticket itself.
// The first in number order answers, worked out once per item.
func (s *ciState) covering(rc *repoCI) (domain.Ticket, bool) {
	commit := rc.latest.Commit
	k := itemKey{rc.name, commit}
	if fix, ok := s.covers[k]; ok {
		return fix, fix.Number != 0
	}
	var found domain.Ticket
	for _, fix := range s.openAutoFixes() {
		if s.carriesFailureOf(fix, rc.name, commit) && !s.lists(fix, rc.name, commit) {
			found = fix
			break
		}
	}
	s.covers[k] = found
	return found, found.Number != 0
}

// carriesFailureOf reports whether commit, in the Repository name,
// carries the failure the auto fix Ticket fix was opened for: a Ticket
// it covers pushed commit there after the commit it was opened for. That
// Ticket may since have ridden on to a still later commit: the Factory
// reports the Tickets riding onto a commit one at a time, and the commit
// they leave still carries their work.
func (s *ciState) carriesFailureOf(fix domain.Ticket, name, commit string) bool {
	m := fix.AutoFix
	if m == nil || m.Repository != recordName(s.p.Key, name) {
		return false
	}
	return slices.ContainsFunc(m.Covers, func(n int) bool {
		t, ok := s.r.Ticket(s.p.Key, n)
		if !ok {
			return false
		}
		pushed := pushedIn(s.p.Key, t, name)
		i := slices.IndexFunc(pushed, func(pc domain.PushedCommit) bool { return pc.Commit == m.Commit })
		return i >= 0 && slices.ContainsFunc(pushed[i+1:], func(pc domain.PushedCommit) bool { return pc.Commit == commit })
	})
}

// lists reports whether the CI failed item of commit, in the Repository
// name, lists t: it is Accepted and not landed there, and its CI result
// there is failed, from commit, its latest pushed commit there.
func (s *ciState) lists(t domain.Ticket, name, commit string) bool {
	pushed := pushedIn(s.p.Key, t, name)
	if t.State != domain.Accepted || landedIn(t, name) || len(pushed) == 0 || pushed[len(pushed)-1].Commit != commit {
		return false
	}
	return s.resultIn(t, name).result == CIFailed
}

// openAutoFixes are the Project's auto fix Tickets that are neither Done
// nor Cancelled, in number order, read on first use.
func (s *ciState) openAutoFixes() []domain.Ticket {
	if !s.autoFixesRead {
		s.autoFixesRead = true
		for _, t := range s.r.Tickets(s.p.Key) {
			if t.AutoFix != nil && !finished(t.State) {
				s.autoFixes = append(s.autoFixes, t)
			}
		}
	}
	return s.autoFixes
}
