package core

import (
	"cmp"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// The Needs Me evaluation (Spec code-central-568): a Project's Needs Me
// worked out once per read, from the store, the Project and the clock,
// so every reader gets the same answer. Needs Me, Attention, the Ticket
// view, the timed-reason watch, list_projects, the CI failure options
// and the Spec reads' Requirement and Map blocks all read it; none works
// out a reason for itself. It holds the one precedence chain, and splits
// the Tickets waiting on the human between rows of their own and CI
// items. Core keeps no cache of it, since timed reasons depend on the
// clock: every read evaluates again. Like the CI state it is built on,
// it is an implementation name, kept out of the glossary.

// evaluation is a Project's Needs Me as one read has it at now: each
// Ticket's reason and since, the CI items listing the Tickets whose
// reason is a CI item's, the Base diverged and Base missing rows, and
// whether each item is a Focus Item. It is worked out for every Ticket
// at once on the first ask, so a read of one Ticket costs as much as a
// read of the Project, and it is never kept past the store read it is
// built in.
type evaluation struct {
	c    *Core
	r    store.Reader
	p    store.Project
	rule blockRule
	// ci is the Project's CI state, which the chain reads each Ticket's
	// CI from.
	ci *ciState
	// given is read in place of the stored Ticket of its number: a
	// write's view of a Ticket it hasn't stored yet. Its number is 0 when
	// there is none.
	given store.Ticket

	done bool
	// list is the Project's Tickets in number order, given in place.
	list []store.Ticket
	// reasons are the reasons of the Tickets waiting on the human, by
	// number.
	reasons map[int]ticketReason
	ciItems []ciItem
	// listedBy is the index in ciItems of the item listing each Ticket a
	// CI item lists, by number.
	listedBy map[int]int
	// requirements are the Specs with a Stage Ticket, the Requirements,
	// by number.
	requirements map[int]bool
	// specWork is each Spec's pull-request work under Pull Request (see
	// specWork), by Spec number, read once.
	specWork map[int][]store.Ticket
}

// ticketReason is why a Ticket waits on the human, and since when.
type ticketReason struct {
	reason NeedsMeReason
	since  time.Time
}

// evaluate is p's evaluation as r has it now.
func (c *Core) evaluate(r store.Reader, p store.Project) *evaluation {
	return &evaluation{c: c, r: r, p: p, rule: c.blockRule(r, p.Key), ci: c.ciStateOf(r, p)}
}

// evaluateWith is p's evaluation with t read in place of the stored
// Ticket of its number, as a write sees t before storing it.
func (c *Core) evaluateWith(r store.Reader, p store.Project, t store.Ticket) *evaluation {
	e := c.evaluate(r, p)
	e.given = t
	return e
}

// evaluated works the evaluation out on the first ask.
func (e *evaluation) evaluated() *evaluation {
	if e.done {
		return e
	}
	e.done = true
	e.list = e.r.Tickets(e.p.Key)
	if n := e.given.Number; n != 0 {
		i, found := slices.BinarySearchFunc(e.list, n, func(t store.Ticket, n int) int { return cmp.Compare(t.Number, n) })
		if found {
			e.list[i] = e.given
		} else {
			e.list = slices.Insert(e.list, i, e.given)
		}
	}
	e.reasons, e.listedBy, e.requirements, e.specWork = map[int]ticketReason{}, map[int]int{}, map[int]bool{}, map[int][]store.Ticket{}
	var listed []store.Ticket
	for _, t := range e.list {
		if StageOf(t.Labels) != "" {
			e.requirements[t.Spec] = true
		}
		reason, since := e.chain(t)
		if reason == "" {
			continue
		}
		e.reasons[t.Number] = ticketReason{reason, since}
		if reason.isCIItem() {
			listed = append(listed, t)
		}
	}
	e.group(listed)
	return e
}

// chain is t's reason and since, or "" when it doesn't wait on the
// human: the one precedence chain, where the first reason that applies
// wins. A Ticket Needs Answer, Answered, Failed or In Review waits for
// that. One In Progress or Accepted waits for CI failed, else CI
// cancelled; then for nothing if it is one of a Spec's pull-request
// Tickets but not its lead; then for Pull request closed, To merge, and
// the timed reasons time passing gives: Silent Claim, else Long Claim,
// while In Progress, and CI unavailable, else Accepted but not landed,
// while Accepted. Any other Ticket, or one of those with none of its
// reasons, waits for Blocked by a Cancelled Ticket, else Closed
// externally, else New issue.
func (e *evaluation) chain(t store.Ticket) (NeedsMeReason, time.Time) {
	c, p, key := e.c, e.p, e.p.Key
	switch t.State {
	case store.NeedsAnswer:
		return ReasonNeedsAnswer, entered(t)
	case store.Answered:
		return ReasonAnswered, entered(t)
	case store.InProgress, store.Accepted:
		a := e.ci.ticket(t)
		if a.failure != nil {
			return ReasonCIFailed, a.since
		} else if a.cancelled {
			return ReasonCICancelled, a.since
		}
		if lead, waiting := e.specPullRequestLead(t); waiting && !lead {
			// A Spec's pull request is one item, on its lead Ticket.
			return "", time.Time{}
		}
		if t.State == store.Accepted && p.Landing.Method == store.LandingPullRequest && t.PullRequest != nil && t.PullRequest.State == store.PRClosed {
			return ReasonPullRequestClosed, entered(t)
		}
		if t.State == store.Accepted && p.LandingPolicy().LandsOnHumanMerge() {
			return ReasonToMerge, entered(t)
		}
		if s := c.silentSince(t); s != nil {
			return ReasonSilentClaim, *s
		}
		if s := c.longSince(t); s != nil {
			return ReasonLongClaim, *s
		}
		if a.unavailable != nil {
			return ReasonCIUnavailable, a.unavailable.Pushed
		}
		// One the CI gate holds on a running CI result, or waiting for its
		// Spec to land, is never Accepted but not landed: landing waits on
		// them.
		if s := c.notLandedSince(p, t); s != nil && !a.waiting && !waitingForSpec(e.r, p.Landing, key, t) {
			return ReasonAcceptedNotLanded, *s
		}
	case store.Failed:
		return ReasonFailed, entered(t)
	case store.InReview:
		return ReasonInReview, entered(t)
	}
	if b, ok := cancelledBlocker(e.rule, t); ok {
		return ReasonBlockedByCancelled, cancelledAt(e.r, key, b)
	}
	if since, ok := closedExternally(t); ok {
		return ReasonClosedExternally, since
	}
	if since, ok := newIssue(e.r, key, t); ok {
		return ReasonNewIssue, since
	}
	return "", time.Time{}
}

// group makes the CI items listing tickets, whose reasons are CI items',
// in order of their first listed Ticket: one per pushed commit and
// reason, listing its Tickets accepted earliest first, waiting since the
// earliest of theirs.
func (e *evaluation) group(tickets []store.Ticket) {
	slices.SortFunc(tickets, func(a, b store.Ticket) int {
		return cmp.Or(entered(a).Compare(entered(b)), cmp.Compare(a.Number, b.Number))
	})
	for _, t := range tickets {
		tr, a := e.reasons[t.Number], e.ci.ticket(t)
		i := slices.IndexFunc(e.ciItems, func(it ciItem) bool { return it.commit == a.latest.Commit && it.reason == tr.reason })
		if i < 0 {
			// Only the detail of its reason is set: a running commit has
			// no failure, and a failed or cancelled one isn't unavailable.
			e.ciItems = append(e.ciItems, ciItem{reason: tr.reason, commit: a.latest.Commit, since: tr.since, failure: a.failure, unavailable: a.unavailable})
			i = len(e.ciItems) - 1
		}
		it := &e.ciItems[i]
		it.tickets = append(it.tickets, t)
		if tr.since.Before(it.since) {
			it.since = tr.since
		}
		e.listedBy[t.Number] = i
	}
}

// specPullRequestLead is specPullRequestLead(t) reading the work of t's
// Spec from the evaluated Tickets, once per Spec.
func (e *evaluation) specPullRequestLead(t store.Ticket) (lead, waiting bool) {
	if !inSpecWork(e.r, e.p, t) {
		return false, false
	}
	g, ok := e.specWork[t.Spec]
	if !ok {
		g = specWorkOf(e.r, e.p, e.list, t.Spec)
		e.specWork[t.Spec] = g
	}
	return leadOf(g, t)
}

// tickets are the Project's Tickets as evaluated, in number order.
func (e *evaluation) tickets() []store.Ticket { return e.evaluated().list }

// reason is t's reason and since, or "" when it doesn't wait on the
// human. A Ticket a CI item lists has the item's reason.
func (e *evaluation) reason(t store.Ticket) (NeedsMeReason, time.Time) {
	tr := e.evaluated().reasons[t.Number]
	return tr.reason, tr.since
}

// itemOf is the CI item listing t, if one does.
func (e *evaluation) itemOf(t store.Ticket) (ciItem, bool) {
	i, ok := e.evaluated().listedBy[t.Number]
	if !ok {
		return ciItem{}, false
	}
	return e.ciItems[i], true
}

// baseRows are the Project's Base diverged and Base missing rows, each
// while its Factory reports it.
func (e *evaluation) baseRows() []NeedsMeRow {
	var out []NeedsMeRow
	if row, ok := e.c.baseDivergedRow(e.p); ok {
		out = append(out, row)
	}
	if row, ok := e.c.baseMissingRow(e.p); ok {
		out = append(out, row)
	}
	return out
}

// needsMeItem is one Needs Me item as the evaluation has it: its row,
// what orders it after its reason and since (its Ticket's number, or a
// CI item's pushed commit), and whether it is a Focus Item.
type needsMeItem struct {
	row    NeedsMeRow
	n      int
	commit string
	// focus says the Project's Focus Kinds leave its reason on. Focus
	// Kinds mark items and filter nothing.
	focus bool
}

// items are the Project's Needs Me items: its Base rows, its CI items,
// and a row of its own for each Ticket waiting on the human that no CI
// item lists.
func (e *evaluation) items() []needsMeItem {
	e.evaluated()
	var out []needsMeItem
	add := func(row NeedsMeRow, n int, commit string) {
		out = append(out, needsMeItem{row: row, n: n, commit: commit, focus: focusItem(e.p, row.Reason)})
	}
	for _, row := range e.baseRows() {
		add(row, 0, "")
	}
	for _, it := range e.ciItems {
		add(e.ciItemRow(it), 0, it.commit)
	}
	for _, t := range e.list {
		if tr, ok := e.reasons[t.Number]; ok && !tr.reason.isCIItem() {
			add(e.row(t, tr), t.Number, "")
		}
	}
	return out
}
