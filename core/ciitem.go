package core

import (
	"slices"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// CI items (code-central-432): CI failed, CI cancelled and CI unavailable
// are each one Needs Me item per pushed commit, listing the Tickets
// whose latest pushed commit it is and that wait on it, across Specs.
// The item has no Ticket of its own, like Base diverged, and is worked
// out on read by the Project's evaluation, from the Tickets whose reason
// is a CI item's, so each listed Ticket has the item's reason. A
// response, Land anyway or Re-run failed jobs, is stored once per commit
// on the Project (domain.CIResponse) and acts on every listed Ticket.

// ciItem is one CI item as worked out: its reason, the Repository Name
// and commit, the Tickets it lists, since when it waits (the earliest of
// theirs), the detail of its reason and its CI result.
type ciItem struct {
	reason      api.NeedsMeReason
	repository  string
	commit      string
	tickets     []domain.Ticket
	since       time.Time
	failure     *api.CIFailureSummary
	unavailable *api.CIUnavailableView
	result      api.CIResult
}

// label is it's pushed commit as its title names it: Name@sha while p has
// more than one Repository, the short commit otherwise.
func (it ciItem) label(p domain.Project) string {
	if severalRepositories(p) {
		return it.repository + "@" + shortCommit(it.commit)
	}
	return shortCommit(it.commit)
}

// ciItemView is it as the views carry it, with the two fix notes while
// their options are offered.
func (e *evaluation) ciItemView(it ciItem) *api.CIItemView {
	p := e.p
	v := &api.CIItemView{Reason: it.reason, Commit: it.commit, Tickets: []api.CIItemTicket{}, Options: ciItemOptions(e.r, p, it)}
	if severalRepositories(p) {
		v.Repository = it.repository
	}
	for _, t := range it.tickets {
		v.Tickets = append(v.Tickets, api.CIItemTicket{ID: DisplayID(p.Key, t.Number), Title: t.Title, Spec: DisplayID(p.Key, t.Spec)})
	}
	if slices.Contains(v.Options, api.OptionSendBack) {
		v.SendBackNote = e.runSummary(it)
	}
	if slices.Contains(v.Options, api.OptionNewFixTicket) {
		v.NewFixNote = e.newFixNote(it)
	}
	v.AutoFixStopped = e.autoFixStoppedLine(it)
	return v
}

// ciItemOptions are the failure options the human may choose on it,
// empty while CI unavailable: Re-run failed jobs under every Landing
// Method; Land anyway under the Merge + Push of its Repository's Landing
// Policy only, since under Branch and Pull Request the human's merge
// decides, and a local-only Repository lands once merged; Send back to
// fix only when it lists exactly one Ticket and that Ticket's work in the
// item's Repository doesn't land with its Spec's (landsWithSpecIn); and for a
// failure New fix Ticket, since a cancelled run has nothing to fix
// forward.
func ciItemOptions(r store.Reader, p domain.Project, it ciItem) []api.CIFailureOption {
	options := []api.CIFailureOption{}
	if it.reason == api.ReasonCIUnavailable {
		return options
	}
	options = append(options, api.OptionRerunCI)
	if landingPolicyIn(p, it.repository).OffersLandAnyway() {
		options = append(options, api.OptionLandAnyway)
	}
	if len(it.tickets) == 1 && !landsWithSpecIn(r, p, it.tickets[0], it.repository) {
		options = append(options, api.OptionSendBack)
	}
	if it.reason == api.ReasonCIFailed {
		options = append(options, api.OptionNewFixTicket)
	}
	return options
}

// failureOptions are the failure options the human may choose on t's CI
// item in the Repository name while t is CI failed or CI cancelled there,
// nil otherwise; see ciItemOptions.
func (e *evaluation) failureOptions(t domain.Ticket, name string) []api.CIFailureOption {
	it, ok := e.itemIn(t, name)
	if !ok || it.reason == api.ReasonCIUnavailable {
		return nil
	}
	return ciItemOptions(e.r, e.p, it)
}

// ciItemRow is it's Needs Me row: one per Repository and commit, with no
// Ticket.
func (e *evaluation) ciItemRow(it ciItem) api.NeedsMeRow {
	return api.NeedsMeRow{
		Project: e.p.Key, Title: needsMeTitle(it.reason) + " at " + it.label(e.p), Reason: it.reason, Since: it.since,
		CIFailure: it.failure, CIUnavailable: it.unavailable, CIItem: e.ciItemView(it),
		CIResult: it.result,
	}
}

// needsMeTitle is a CI item's reason as its title starts.
func needsMeTitle(reason api.NeedsMeReason) string {
	switch reason {
	case api.ReasonCICancelled:
		return "CI cancelled"
	case api.ReasonCIUnavailable:
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
func responseOf(p domain.Project, record, commit string) domain.CIResponse {
	if i := slices.IndexFunc(p.CIResponses, func(r domain.CIResponse) bool { return r.Repository == record && r.Commit == commit }); i >= 0 {
		return p.CIResponses[i]
	}
	return domain.CIResponse{Repository: record, Commit: commit}
}

// respond applies change to p's response to commit in the Repository
// record names, adding one when p has none, on a copy of the list.
func respond(p *domain.Project, record, commit string, change func(*domain.CIResponse)) {
	rs := slices.Clone(p.CIResponses)
	i := slices.IndexFunc(rs, func(r domain.CIResponse) bool { return r.Repository == record && r.Commit == commit })
	if i < 0 {
		rs = append(rs, domain.CIResponse{Repository: record, Commit: commit})
		i = len(rs) - 1
	}
	change(&rs[i])
	p.CIResponses = rs
}
