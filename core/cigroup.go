package core

import (
	"cmp"
	"slices"

	"github.com/tomoram/code-central/store"
)

// Per-Spec CI (code-central-379): the Accepted Tickets of one Spec that
// the Factory pushed together share a pushed commit, so they share its CI
// result. Their CI failed, CI cancelled and CI unavailable reasons go to
// one of them, the lead Ticket, and its options act on all of them.

// CIGroupView is a Spec-landed Ticket's group of waiting Tickets that
// share its pushed commit.
type CIGroupView struct {
	Spec string `json:"spec"`
	// Lead is the Ticket that carries the group's CI reasons.
	Lead    string `json:"lead"`
	Waiting int    `json:"waiting"`
	// Tickets are the waiting Tickets, the lead first.
	Tickets []string `json:"tickets"`
}

// ciGroup is the Tickets waiting with t on one pushed commit, the lead
// (accepted earliest) first: the Accepted Spec-landed Tickets of t's Spec
// whose latest pushed commit is t's. It is nil when t isn't one of them.
func ciGroup(r store.Reader, key string, t store.Ticket) []store.Ticket {
	p, _ := r.Project(key)
	if t.State != store.Accepted || len(t.PushedCommits) == 0 || !specLanded(r, p.Landing, key, t) {
		return nil
	}
	commit := latestPushed(t).Commit
	var group []store.Ticket
	for _, o := range r.Tickets(key) {
		if o.Spec == t.Spec && o.State == store.Accepted && len(o.PushedCommits) > 0 && latestPushed(o).Commit == commit && specLanded(r, p.Landing, key, o) {
			group = append(group, o)
		}
	}
	slices.SortFunc(group, func(a, b store.Ticket) int {
		if c := entered(a).Compare(entered(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.Number, b.Number)
	})
	return group
}

// isCILead is whether t carries its CI reasons: it is outside any group
// or the group's lead.
func isCILead(r store.Reader, key string, t store.Ticket) bool {
	g := ciGroup(r, key, t)
	return len(g) == 0 || g[0].Number == t.Number
}

// ciGroupView is t's group, or nil when t is in none.
func ciGroupView(r store.Reader, key string, t store.Ticket) *CIGroupView {
	g := ciGroup(r, key, t)
	if len(g) == 0 {
		return nil
	}
	v := &CIGroupView{Spec: DisplayID(key, t.Spec), Lead: DisplayID(key, g[0].Number), Waiting: len(g), Tickets: []string{}}
	for _, o := range g {
		v.Tickets = append(v.Tickets, DisplayID(key, o.Number))
	}
	return v
}

// groupOrSelf is t's group, or t alone when it is in none.
func groupOrSelf(r store.Reader, key string, t store.Ticket) []store.Ticket {
	if g := ciGroup(r, key, t); len(g) > 0 {
		return g
	}
	return []store.Ticket{t}
}
