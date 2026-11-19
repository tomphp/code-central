package core

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Needs Me (ticket 32): what is waiting on the human, computed on read,
// never stored. Each Ticket has at most one reason; when two apply it
// takes the earlier group, the cheaper one to unblock.

// isCIItem reports whether r is shown as a CI item: one Needs Me row
// per pushed commit, with no Ticket of its own (code-central-432).
func isCIItem(r api.NeedsMeReason) bool {
	return r == api.ReasonCIFailed || r == api.ReasonCICancelled || r == api.ReasonCIUnavailable
}

// isTimed reports whether time passing gives r: Silent Claim, Long Claim,
// CI unavailable or Accepted but not landed.
func isTimed(r api.NeedsMeReason) bool {
	return r == api.ReasonSilentClaim || r == api.ReasonLongClaim || r == api.ReasonCIUnavailable || r == api.ReasonAcceptedNotLanded
}

// groupIndex is a reason's place in the group order.
func groupIndex(r api.NeedsMeReason) int {
	return slices.IndexFunc(api.NeedsMeReasonViews, func(x api.NeedsMeReasonView) bool { return x.Reason == r })
}

// requirement is the Requirement t is on, or nil when its Spec isn't
// one.
func (e *evaluation) requirement(t domain.Ticket) *api.NeedsMeRequirement {
	if !e.evaluated().requirements[t.Spec] {
		return nil
	}
	key := e.p.Key
	sp, _ := e.r.Spec(key, t.Spec)
	return &api.NeedsMeRequirement{Spec: DisplayID(key, t.Spec), Title: sp.Title, Stage: StageOf(t.Labels)}
}

// needsMeSpec is the Spec t is on when t is on a Map, or is a Map Ticket
// or a Prototype, and its Spec isn't a Requirement; nil otherwise.
func needsMeSpec(r store.Reader, key string, t domain.Ticket, req *api.NeedsMeRequirement) *api.NeedsMeSpec {
	if req != nil {
		return nil
	}
	sp, _ := r.Spec(key, t.Spec)
	isMap := slices.Contains(sp.Labels, MapLabel)
	if !isMap && !slices.ContainsFunc(t.Labels, isWayfinderType) {
		return nil
	}
	return &api.NeedsMeSpec{Spec: DisplayID(key, t.Spec), Title: sp.Title, Map: isMap}
}

// NeedsMe is needs_me, the human's read across every Project: one
// evaluation of each.
func (c *Core) NeedsMe(context.Context) (api.NeedsMeView, error) {
	var all []needsMeItem
	counts := map[string]int{}
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			items := c.evaluate(r, p).items()
			if n := needsMeCount(items); n > 0 {
				counts[p.Key] = n
			}
			all = append(all, items...)
		}
		return nil
	})
	slices.SortFunc(all, func(a, b needsMeItem) int {
		return cmp.Or(
			cmp.Compare(groupIndex(a.row.Reason), groupIndex(b.row.Reason)),
			a.row.Since.Compare(b.row.Since),
			cmp.Compare(a.row.Project, b.row.Project),
			cmp.Compare(a.n, b.n),
			cmp.Compare(a.commit, b.commit),
		)
	})
	v := api.NeedsMeView{Counts: counts, Groups: []api.NeedsMeGroup{}}
	for _, n := range counts {
		v.Count += n
	}
	for _, it := range all {
		if len(v.Groups) == 0 || v.Groups[len(v.Groups)-1].Reason != it.row.Reason {
			v.Groups = append(v.Groups, api.NeedsMeGroup{Reason: it.row.Reason})
		}
		g := &v.Groups[len(v.Groups)-1]
		g.Tickets = append(g.Tickets, it.row)
	}
	return v, nil
}

// needsMeCount is the Needs Me count of one Project's items, as the human
// sees it: each item counts once, except that a Requirement's Stage
// Tickets count once in total, as their Requirement.
func needsMeCount(items []needsMeItem) int {
	requirements, n := map[string]bool{}, 0
	for _, it := range items {
		if rq := it.row.Requirement; rq != nil && rq.Stage != "" {
			requirements[rq.Spec] = true
		} else {
			n++
		}
	}
	return n + len(requirements)
}

// row is the Needs Me row of its own of t, which waits for tr and no CI
// item lists.
func (e *evaluation) row(t domain.Ticket, tr ticketReason) api.NeedsMeRow {
	key := e.p.Key
	req := e.requirement(t)
	row := api.NeedsMeRow{
		Project: key, ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Reason: tr.reason, Since: tr.since, Link: linkView(t.Link),
		Requirement: req, Spec: needsMeSpec(e.r, key, t, req), Prototype: slices.Contains(t.Labels, PrototypeLabel),
		Retro:    slices.Contains(t.Labels, RetroLabel),
		CIResult: e.ci.ticket(t).result, PullRequest: pullRequestView(homePullRequest(key, t)),
	}
	switch tr.reason {
	case api.ReasonNeedsAnswer:
		if rd := t.Head.LatestRound; rd != nil {
			s := api.RoundSummary{Number: rd.Number, Questions: len(rd.Questions)}
			for _, q := range rd.Questions {
				if q.Status == domain.QuestionOpen {
					s.Open++
				}
			}
			row.Round = &s
		}
	case api.ReasonFailed:
		if fr := t.Head.LatestFailureReport; fr != nil {
			row.Failure = &api.FailureSummary{Headline: firstLine(fr.WhatBroke), Reports: fr.Number}
		}
	case api.ReasonInReview:
		row.Summary = firstLine(finishSummary(e.withComments(t)))
	case api.ReasonToMerge:
		row.Branch = branchOf(t)
	case api.ReasonAcceptedNotLanded:
		row.NotLanded = e.c.notLandedView(e.p, e.withComments(t), e.ci)
	case api.ReasonPullRequestClosed:
		row.ClosedPullRequests = closedPullRequests(e.p, t)
		if row.PullRequest == nil && len(row.ClosedPullRequests) > 0 {
			// The row's link is a closed pull request when the Home
			// Repository has none.
			row.PullRequest = &row.ClosedPullRequests[0]
		}
	case api.ReasonBlockedByCancelled:
		b, _ := cancelledBlocker(e.rule, t)
		row.Blocker = &b
	}
	return row
}

// withComments is t, which the evaluation read as a head, with its
// Comments: a row reads them only for the reasons that show one.
func (e *evaluation) withComments(t domain.Ticket) domain.Ticket {
	if full, ok := e.r.Ticket(e.p.Key, t.Number, store.Comments); ok {
		return full
	}
	return t
}

// finishSummary is the comment stored with t's last move into In Review,
// or "" when that move had none (a human move).
func finishSummary(t domain.Ticket) string {
	at := entered(t)
	for i := len(t.Comments) - 1; i >= 0; i-- {
		if cm := t.Comments[i]; cm.Time.Equal(at) && cm.Actor != string(api.Human) {
			return cm.Body
		}
	}
	return ""
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// entered is when t moved into its current State (its last write, if no
// move records it), from its head.
func entered(t domain.Ticket) time.Time {
	if t.Head.Entered.IsZero() {
		return t.Updated
	}
	return t.Head.Entered
}

// cancelledBlocker is the first open Blocker of t that is Cancelled with
// no Replacements left to follow.
func cancelledBlocker(rule blockRule, t domain.Ticket) (api.BlockerView, bool) {
	for _, b := range rule.openBlockers(t) {
		if b.State == domain.Cancelled {
			return b, true
		}
	}
	return api.BlockerView{}, false
}

// cancelledAt is when the Cancelled Blocker b was cancelled.
func cancelledAt(r store.Reader, key string, b api.BlockerView) time.Time {
	n, _ := refNumber(api.Ref(b.ID), key, "blocker")
	bt, _ := r.Ticket(key, n)
	return entered(bt)
}
