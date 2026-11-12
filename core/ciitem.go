package core

import (
	"cmp"
	"slices"
	"time"

	"github.com/tomoram/code-central/store"
)

// CI items (code-central-432): CI failed, CI cancelled and CI unavailable
// are each one Needs Me item per pushed commit, listing the Tickets
// whose latest pushed commit it is and that wait on it, across Specs.
// The item has no Ticket of its own, like Base diverged, and is worked
// out on read. A response, Land anyway or Re-run failed jobs, is stored
// once per commit on the Project (store.CIResponse) and acts on every
// listed Ticket; a response stored on a Ticket's pushed commit before
// this is still read.

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

// ciItems are p's CI items, in order of their first listed Ticket, each
// listing its Tickets accepted earliest first.
func (c *Core) ciItems(r store.Reader, p store.Project) []ciItem {
	tickets := r.Tickets(p.Key)
	slices.SortFunc(tickets, func(a, b store.Ticket) int {
		return cmp.Or(entered(a).Compare(entered(b)), cmp.Compare(a.Number, b.Number))
	})
	var items []ciItem
	for _, t := range tickets {
		reason, since, failure, unavailable := c.ciReasonOf(r, p, t)
		if reason == "" {
			continue
		}
		commit := latestPushed(t).Commit
		i := slices.IndexFunc(items, func(it ciItem) bool { return it.commit == commit && it.reason == reason })
		if i < 0 {
			items = append(items, ciItem{reason: reason, commit: commit, since: since, failure: failure, unavailable: unavailable})
			i = len(items) - 1
		}
		items[i].tickets = append(items[i].tickets, t)
		if since.Before(items[i].since) {
			items[i].since = since
		}
	}
	return items
}

// ciReasonOf is t's CI reason, since when, and its detail, while a CI
// item lists t: an Accepted Ticket whose latest pushed commit's CI
// result is failed or cancelled and that the human hasn't dealt with,
// or that is CI unavailable.
func (c *Core) ciReasonOf(r store.Reader, p store.Project, t store.Ticket) (reason NeedsMeReason, since time.Time, failure *CIFailureSummary, unavailable *CIUnavailableView) {
	if f, since, ok := ciFailure(r, p.Key, t, c.now()); ok {
		return ReasonCIFailed, since, &f, nil
	}
	if since, ok := ciCancelled(r, p.Key, t, c.now()); ok {
		return ReasonCICancelled, since, nil, nil
	}
	if _, u := c.ciWaitingOn(r, p, t); u != nil {
		return ReasonCIUnavailable, u.Pushed, nil, u
	}
	return "", time.Time{}, nil, nil
}

// ciItemOf is the CI item listing t, if one does.
func (c *Core) ciItemOf(r store.Reader, p store.Project, t store.Ticket) (ciItem, bool) {
	reason, _, _, _ := c.ciReasonOf(r, p, t)
	if reason == "" {
		return ciItem{}, false
	}
	for _, it := range c.ciItems(r, p) {
		if it.reason == reason && it.commit == latestPushed(t).Commit {
			return it, true
		}
	}
	return ciItem{}, false
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
	if len(it.tickets) == 1 && !specLanded(r, p.Landing, p.Key, it.tickets[0]) {
		options = append(options, OptionSendBack)
	}
	if it.reason == ReasonCIFailed {
		options = append(options, OptionNewFixTicket)
	}
	return options
}

// ciItemRow is it's Needs Me row: one per commit, with no Ticket.
func (c *Core) ciItemRow(r store.Reader, p store.Project, it ciItem) NeedsMeRow {
	return NeedsMeRow{
		Project: p.Key, Title: needsMeTitle(it.reason) + " at " + shortCommit(it.commit), Reason: it.reason, Since: it.since,
		CIFailure: it.failure, CIUnavailable: it.unavailable, CIItem: ciItemView(r, p, it),
		CIResult: ticketCIResult(r, p.Key, it.tickets[0], c.now()),
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

// responded is pc with p's response to its commit overlaid: Land anyway
// from either, and the later of the two Re-run requests with its
// attempts, so a response stored on the pushed commit before
// code-central-432 still counts.
func responded(p store.Project, pc store.PushedCommit) store.PushedCommit {
	res := responseOf(p, pc.Commit)
	if pc.LandAnyway.IsZero() {
		pc.LandAnyway = res.LandAnyway
	}
	if res.Rerun.After(pc.Rerun) {
		pc.Rerun, pc.Reruns = res.Rerun, res.Reruns
	}
	return pc
}

// respondedCommits are t's pushed commits, each with p's response to it
// overlaid.
func respondedCommits(p store.Project, t store.Ticket) []store.PushedCommit {
	out := make([]store.PushedCommit, 0, len(t.PushedCommits))
	for _, pc := range t.PushedCommits {
		out = append(out, responded(p, pc))
	}
	return out
}

// respondedLatest is t's latest pushed commit with p's response to it
// overlaid; t must have one.
func respondedLatest(p store.Project, t store.Ticket) store.PushedCommit {
	return responded(p, latestPushed(t))
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
