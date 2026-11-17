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
	Reason NeedsMeReason `json:"reason"`
	// Repository is the Repository Name the commit was pushed in, left
	// out while the Project has the Home Repository alone. Each option
	// acts on that Repository only.
	Repository string            `json:"repository,omitempty"`
	Commit     string            `json:"commit"`
	Tickets    []CIItemTicket    `json:"tickets"`
	Options    []CIFailureOption `json:"options"`
}

// CIItemTicket is one Ticket a CI item lists, with its Spec.
type CIItemTicket struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Spec  string `json:"spec"`
}

// ciItem is one CI item as worked out: its reason, the Repository Name
// and commit, the Tickets it lists, since when it waits (the earliest of
// theirs), the detail of its reason and its CI result.
type ciItem struct {
	reason      NeedsMeReason
	repository  string
	commit      string
	tickets     []store.Ticket
	since       time.Time
	failure     *CIFailureSummary
	unavailable *CIUnavailableView
	result      CIResult
}

// label is it's pushed commit as its title names it: Name@sha while p has
// more than one Repository, the short commit otherwise.
func (it ciItem) label(p store.Project) string {
	if severalRepositories(p) {
		return it.repository + "@" + shortCommit(it.commit)
	}
	return shortCommit(it.commit)
}

// ciItemView is it as the views carry it.
func ciItemView(r store.Reader, p store.Project, it ciItem) *CIItemView {
	v := &CIItemView{Reason: it.reason, Commit: it.commit, Tickets: []CIItemTicket{}, Options: ciItemOptions(r, p, it)}
	if severalRepositories(p) {
		v.Repository = it.repository
	}
	for _, t := range it.tickets {
		v.Tickets = append(v.Tickets, CIItemTicket{ID: DisplayID(p.Key, t.Number), Title: t.Title, Spec: DisplayID(p.Key, t.Spec)})
	}
	return v
}

// ciItemOptions are the failure options the human may choose on it,
// empty while CI unavailable: Re-run failed jobs under every Landing
// Method; Land anyway under the Merge + Push of its Repository's Landing
// Policy only, since under Branch and Pull Request the human's merge
// decides, and a local-only Repository lands once merged; Send back to
// fix only when it lists exactly one Ticket and that Ticket isn't
// Spec-landed, whose work is in the base with its Spec's; and for a
// failure New fix Ticket, since a cancelled run has nothing to fix
// forward.
func ciItemOptions(r store.Reader, p store.Project, it ciItem) []CIFailureOption {
	options := []CIFailureOption{}
	if it.reason == ReasonCIUnavailable {
		return options
	}
	options = append(options, OptionRerunCI)
	if landingPolicyIn(p, it.repository).OffersLandAnyway() {
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
// item in the Repository name while t is CI failed or CI cancelled there,
// nil otherwise; see ciItemOptions.
func (e *evaluation) failureOptions(t store.Ticket, name string) []CIFailureOption {
	it, ok := e.itemIn(t, name)
	if !ok || it.reason == ReasonCIUnavailable {
		return nil
	}
	return ciItemOptions(e.r, e.p, it)
}

// ciItemRow is it's Needs Me row: one per Repository and commit, with no
// Ticket.
func (e *evaluation) ciItemRow(it ciItem) NeedsMeRow {
	return NeedsMeRow{
		Project: e.p.Key, Title: needsMeTitle(it.reason) + " at " + it.label(e.p), Reason: it.reason, Since: it.since,
		CIFailure: it.failure, CIUnavailable: it.unavailable, CIItem: ciItemView(e.r, e.p, it),
		CIResult: it.result,
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

// responseOf is p's response to commit in the Repository record names,
// zero when it has none.
func responseOf(p store.Project, record, commit string) store.CIResponse {
	if i := slices.IndexFunc(p.CIResponses, func(r store.CIResponse) bool { return r.Repository == record && r.Commit == commit }); i >= 0 {
		return p.CIResponses[i]
	}
	return store.CIResponse{Repository: record, Commit: commit}
}

// respond applies change to p's response to commit in the Repository
// record names, adding one when p has none, on a copy of the list.
func respond(p *store.Project, record, commit string, change func(*store.CIResponse)) {
	rs := slices.Clone(p.CIResponses)
	i := slices.IndexFunc(rs, func(r store.CIResponse) bool { return r.Repository == record && r.Commit == commit })
	if i < 0 {
		rs = append(rs, store.CIResponse{Repository: record, Commit: commit})
		i = len(rs) - 1
	}
	change(&rs[i])
	p.CIResponses = rs
}
