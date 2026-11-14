package core

import (
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// CI items (code-central-432): CI failed, CI cancelled and CI unavailable
// are each one Needs Me item per pushed commit, listing the Tickets
// whose latest pushed commit it is and that wait on it, across Specs.
// The item has no Ticket of its own, like Base diverged, and is worked
// out on read by the Project's evaluation, from the Tickets whose reason
// is a CI item's, so each listed Ticket has the item's reason. A
// response, Land anyway or Re-run failed jobs, is stored once per commit
// on the Project (store.CIResponse) and acts on every listed Ticket.

// CIItemView is a CI item as a Needs Me row and each listed Ticket's
// view carry it: its reason, the pushed commit, the Tickets waiting on
// it, accepted earliest first, and the failure options the human may
// choose, each an operation's name, which act on every one of them;
// none while CI unavailable.
type CIItemView struct {
	Reason  NeedsMeReason  `json:"reason"`
	Commit  string         `json:"commit"`
	Tickets []CIItemTicket `json:"tickets"`
	Options []string       `json:"options"`
}

// CIItemTicket is one Ticket a CI item lists, with its Spec.
type CIItemTicket struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Spec  string `json:"spec"`
}

// ciItem is one CI item as worked out: its reason and commit, the
// Tickets it lists, since when it waits (the earliest of theirs), and the
// detail of its reason.
type ciItem struct {
	reason      NeedsMeReason
	commit      string
	tickets     []store.Ticket
	since       time.Time
	failure     *CIFailureSummary
	unavailable *CIUnavailableView
}

// ciItemView is it as the views carry it.
func ciItemView(r store.Reader, p store.Project, it ciItem) *CIItemView {
	v := &CIItemView{Reason: it.reason, Commit: it.commit, Tickets: []CIItemTicket{}, Options: ciItemOptions(r, p, it)}
	for _, t := range it.tickets {
		v.Tickets = append(v.Tickets, CIItemTicket{ID: DisplayID(p.Key, t.Number), Title: t.Title, Spec: DisplayID(p.Key, t.Spec)})
	}
	return v
}

// ciItemOptions are the failure options the human may choose on it,
// empty while CI unavailable: Re-run failed jobs under every Landing
// Method; Land anyway under Merge only, since under Branch and Pull
// Request the human's merge decides; Send back to fix only when it lists
// exactly one Ticket and that Ticket isn't Spec-landed, whose work is in
// the base with its Spec's; and for a failure New fix Ticket, since a
// cancelled run has nothing to fix forward.
func ciItemOptions(r store.Reader, p store.Project, it ciItem) []string {
	options := []string{}
	if it.reason == ReasonCIUnavailable {
		return options
	}
	options = append(options, OptionRerunCI)
	if p.Landing.Method == store.LandingMerge {
		options = append(options, OptionLandAnyway)
	}
	if len(it.tickets) == 1 && !specLanded(r, p, it.tickets[0]) {
		options = append(options, OptionSendBack)
	}
	if it.reason == ReasonCIFailed {
		options = append(options, OptionNewFixTicket)
	}
	return options
}

// failureOptions are the failure options the human may choose on t's CI
// item while t is CI failed or CI cancelled, nil otherwise; see
// ciItemOptions.
func (e *evaluation) failureOptions(t store.Ticket) []string {
	it, ok := e.itemOf(t)
	if !ok || it.reason == ReasonCIUnavailable {
		return nil
	}
	return ciItemOptions(e.r, e.p, it)
}

// ciItemRow is it's Needs Me row: one per commit, with no Ticket.
func (e *evaluation) ciItemRow(it ciItem) NeedsMeRow {
	return NeedsMeRow{
		Project: e.p.Key, Title: needsMeTitle(it.reason) + " at " + shortCommit(it.commit), Reason: it.reason, Since: it.since,
		CIFailure: it.failure, CIUnavailable: it.unavailable, CIItem: ciItemView(e.r, e.p, it),
		CIResult: e.ci.ticket(it.tickets[0]).result,
	}
}

// needsMeTitle is a CI item's reason as its title starts.
func needsMeTitle(reason NeedsMeReason) string {
	switch reason {
	case ReasonCICancelled:
		return "CI cancelled"
	case ReasonCIUnavailable:
		return "CI unavailable"
	}
	return "CI failed"
}

// shortCommit is commit's first seven characters, as git shows it.
func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// responseOf is p's response to commit, zero when it has none.
func responseOf(p store.Project, commit string) store.CIResponse {
	if i := slices.IndexFunc(p.CIResponses, func(r store.CIResponse) bool { return r.Commit == commit }); i >= 0 {
		return p.CIResponses[i]
	}
	return store.CIResponse{Commit: commit}
}

// respond applies change to p's response to commit, adding one when p
// has none, on a copy of the list.
func respond(p *store.Project, commit string, change func(*store.CIResponse)) {
	rs := slices.Clone(p.CIResponses)
	i := slices.IndexFunc(rs, func(r store.CIResponse) bool { return r.Commit == commit })
	if i < 0 {
		rs = append(rs, store.CIResponse{Commit: commit})
		i = len(rs) - 1
	}
	change(&rs[i])
	p.CIResponses = rs
}
