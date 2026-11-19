package core

import (
	"slices"

	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Auto-fix chains its attempts (Spec code-central-1022, ADR 0066): an
// open auto fix Ticket covers the Tickets its CI failed item listed, so
// a later commit carrying them waits on it out of Needs Me, until the
// fix Ticket's own push has a CI result and its item lists it, which is
// the next attempt.

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
		if stopped := e.autoFixStoppedLine(it); stopped != nil {
			return &CIAutoFixView{Stopped: stopped}
		}
	}
	return nil
}

// autoFixStoppedLine is it's "Auto-fix stopped" line while Auto-fix has
// stopped at the limit on it, nil otherwise: each attempt, found by
// following the chain back from the highest auto fix Ticket it lists.
func (e *evaluation) autoFixStoppedLine(it ciItem) *AutoFixStoppedView {
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

// covering is the open auto fix Ticket covering t's CI failed item of
// rc's latest pushed commit, if one does: the commit carries the failure
// it was opened for, and the item doesn't list the fix Ticket itself.
// Where chains meet on one commit, t's answer is the one whose mark
// covers t, else the first in number order.
func (s *ciState) covering(t domain.Ticket, rc *repoCI) (domain.Ticket, bool) {
	fixes := s.coveringFixes(itemKey{rc.name, rc.latest.Commit})
	if len(fixes) == 0 {
		return domain.Ticket{}, false
	}
	if i := slices.IndexFunc(fixes, func(fix domain.Ticket) bool { return slices.Contains(fix.AutoFix.Covers, t.Number) }); i >= 0 {
		return fixes[i], true
	}
	return fixes[0], true
}

// coveringFixes are the open auto fix Tickets covering k's CI failed
// item, in number order, worked out once per item.
func (s *ciState) coveringFixes(k itemKey) []domain.Ticket {
	if fixes, ok := s.coveredBy[k]; ok {
		return fixes
	}
	fixes := []domain.Ticket{}
	for _, fix := range s.openAutoFixes() {
		if s.carriesFailureOf(fix, k) && !s.listedOn(fix, k) {
			fixes = append(fixes, fix)
		}
	}
	s.coveredBy[k] = fixes
	return fixes
}

// carriesFailureOf reports whether k's commit carries the failure the
// auto fix Ticket fix was opened for: a Ticket it covers pushed that
// commit in k's Repository after the commit it was opened for. That
// Ticket may since have ridden on to a still later commit: the Factory
// reports the Tickets riding onto a commit one at a time, and the commit
// they leave still carries their work.
func (s *ciState) carriesFailureOf(fix domain.Ticket, k itemKey) bool {
	m := fix.AutoFix
	if m == nil || m.Repository != recordName(s.p.Key, k.name) {
		return false
	}
	return slices.ContainsFunc(m.Covers, func(n int) bool {
		t, ok := s.r.Ticket(s.p.Key, n)
		if !ok {
			return false
		}
		pushed := pushedIn(s.p.Key, t, k.name)
		i := slices.IndexFunc(pushed, func(pc domain.PushedCommit) bool { return pc.Commit == m.Commit })
		return i >= 0 && slices.ContainsFunc(pushed[i+1:], func(pc domain.PushedCommit) bool { return pc.Commit == k.commit })
	})
}

// listedOn reports whether t is listed on k's CI failed item: it is
// Accepted and not landed in k's Repository, and its CI result there is
// failed, from k's commit, its latest pushed commit there. Only its CI
// result is worked out, not whether the failure is handled, which would
// ask again whose fix Ticket covers what.
func (s *ciState) listedOn(t domain.Ticket, k itemKey) bool {
	pushed := pushedIn(s.p.Key, t, k.name)
	if t.State != domain.Accepted || landedIn(t, k.name) || len(pushed) == 0 || pushed[len(pushed)-1].Commit != k.commit {
		return false
	}
	return s.repoResult(t, k.name).result == CIFailed
}

// openAutoFixes are the Project's auto fix Tickets that are neither Done
// nor Cancelled, in number order, read on first use.
func (s *ciState) openAutoFixes() []domain.Ticket {
	if !s.autoFixesRead {
		s.autoFixesRead = true
		for _, t := range s.r.FindTickets(s.p.Key, store.TicketFilter{States: unfinished}) {
			if t.AutoFix != nil {
				s.autoFixes = append(s.autoFixes, t)
			}
		}
	}
	return s.autoFixes
}
