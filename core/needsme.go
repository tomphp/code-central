package core

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Needs Me (ticket 32): what is waiting on the human, computed on read,
// never stored. Each Ticket has at most one reason; when two apply it
// takes the earlier group, the cheaper one to unblock.

// NeedsMeReason is why a Ticket waits on the human. "" means it doesn't.
type NeedsMeReason string

// The reasons, in group order: cheapest to unblock first.
const (
	ReasonNeedsAnswer NeedsMeReason = "needs_answer"
	ReasonAnswered    NeedsMeReason = "answered"
	ReasonSilentClaim NeedsMeReason = "silent_claim"
	ReasonLongClaim   NeedsMeReason = "long_claim"
	ReasonFailed      NeedsMeReason = "failed"
	ReasonInReview    NeedsMeReason = "in_review"
	// ReasonCIFailed is a pushed commit whose CI result is failed, with
	// the Accepted Tickets waiting on it, which the human hasn't dealt
	// with (code-central-261). It is one item per commit, with no Ticket
	// (code-central-432); each listed Ticket reads it as its reason.
	ReasonCIFailed NeedsMeReason = "ci_failed"
	// ReasonCICancelled is a pushed commit whose CI result is cancelled,
	// with the Accepted Tickets waiting on it, which the human hasn't
	// dealt with. One item per commit, like CI failed.
	ReasonCICancelled NeedsMeReason = "ci_cancelled"
	// ReasonBaseDiverged is a Project under Pull Request or Merge + Push
	// whose Factory reports its local base diverged from the remote's
	// (code-central-394, code-central-425). It is one item per Project,
	// with no Ticket.
	ReasonBaseDiverged NeedsMeReason = "base_diverged"
	// ReasonBaseMissing is a Project whose Factory reports its base
	// missing, under any Landing Method. It is one item per Project, with
	// no Ticket.
	ReasonBaseMissing NeedsMeReason = "base_missing"
	// ReasonPullRequestClosed is an Accepted Ticket under Pull Request
	// whose pull request is closed or abandoned without merging
	// (code-central-391); it clears if the pull request is reopened.
	ReasonPullRequestClosed NeedsMeReason = "pull_request_closed"
	// ReasonToMerge is a Ticket Accepted under the Branch or Pull Request
	// Landing Method (code-central-175): the human merges its branch or
	// pull request, at once.
	ReasonToMerge NeedsMeReason = "to_merge"
	// ReasonCIUnavailable is a pushed commit the CI gate has held Tickets
	// on for CIUnavailableAfter since their push with no word of its CI
	// (code-central-262). One item per commit, like CI failed.
	ReasonCIUnavailable NeedsMeReason = "ci_unavailable"
	// ReasonAcceptedNotLanded is a Ticket Accepted for more than
	// NotLandedAfter (ticket 72): its work hasn't reached the base.
	ReasonAcceptedNotLanded  NeedsMeReason = "accepted_not_landed"
	ReasonBlockedByCancelled NeedsMeReason = "blocked_by_cancelled"
	// ReasonNewIssue is an Incoming Issue awaiting triage that isn't
	// finished and isn't closed externally (code-central-287).
	ReasonNewIssue NeedsMeReason = "new_issue"
	// ReasonClosedExternally is a linked Ticket whose external item isn't
	// open, whose Link isn't detached and that isn't finished (ticket 42).
	ReasonClosedExternally NeedsMeReason = "closed_externally"
)

// isCIItem reports whether r is shown as a CI item: one Needs Me row
// per pushed commit, with no Ticket of its own (code-central-432).
func (r NeedsMeReason) isCIItem() bool {
	return r == ReasonCIFailed || r == ReasonCICancelled || r == ReasonCIUnavailable
}

// needsMeReasons are the reasons in group order, each with what it means
// for the published workflow.
var needsMeReasons = []NeedsMeReasonView{
	{ReasonNeedsAnswer, "Needs Answer: a Round waits for the human's answers"},
	{ReasonAnswered, "Answered: answers wait for the asking session to pick them up"},
	{ReasonSilentClaim, "Silent Claim: In Progress, no activity for 30 minutes"},
	{ReasonLongClaim, "Long Claim: In Progress and active, with no claim or pick_up_answers for over 60 minutes"},
	{ReasonFailed, "Failed: a Failure Report waits for the human"},
	{ReasonInReview, "In Review: finished work waits to be accepted or rejected"},
	{ReasonCIFailed, "CI failed: one item per pushed commit, listing the Tickets waiting on it; a failure option acts on all of them"},
	{ReasonCICancelled, "CI cancelled: one item per pushed commit whose CI was cancelled"},
	{ReasonBaseDiverged, "Base diverged: a Project's local base has diverged from the remote's; reconcile it by hand"},
	{ReasonBaseMissing, "Base missing: a Project's base branch exists nowhere; create it or change it in Options"},
	{ReasonPullRequestClosed, "Pull request closed: Accepted with its pull request closed without merging"},
	{ReasonToMerge, "To merge: Accepted under Branch or Pull Request; merge it to the base"},
	{ReasonCIUnavailable, "CI unavailable: one item per pushed commit with no word of its CI 10 minutes after its push"},
	{ReasonAcceptedNotLanded, "Accepted but not landed: Accepted over 10 minutes and not in the base, unless its CI is running"},
	{ReasonBlockedByCancelled, "Blocked by a Cancelled Ticket with no Replacements"},
	{ReasonNewIssue, "New issue: an Incoming Issue awaiting triage"},
	{ReasonClosedExternally, "Closed externally: its linked item was closed, deleted or left its Integration's query; cancel, mark Done or unlink it"},
}

// groupIndex is a reason's place in the group order.
func groupIndex(r NeedsMeReason) int {
	return slices.IndexFunc(needsMeReasons, func(x NeedsMeReasonView) bool { return x.Reason == r })
}

// NeedsMeView is needs_me: everything waiting on the human across every
// Project, in groups in group order (empty groups left out), each group
// longest-waiting first. Every view renders this order as it comes.
type NeedsMeView struct {
	Count  int            `json:"count"`
	Groups []NeedsMeGroup `json:"groups"`
}

// NeedsMeGroup is the Tickets waiting for one reason.
type NeedsMeGroup struct {
	Reason  NeedsMeReason `json:"reason"`
	Tickets []NeedsMeRow  `json:"tickets"`
}

// NeedsMeRow is one waiting Ticket with what its one-line detail needs.
// Since is when it started waiting: the Round posted, the last answer,
// the last activity (Silent Claim, so the quiet time is now minus it),
// the last move (Long Claim),
// the Failure Report, the finish, the first failed CI Run finishing (CI
// failed), the accept (To merge, and Accepted but not landed), the push
// (CI unavailable),
// the Blocker's cancel, or the Ticket's creation (New issue). Only the
// detail field for its reason is set. A CI failed, CI cancelled or CI
// unavailable row is one per pushed commit, with no Ticket: its ID is
// empty, and CIItem lists the Tickets waiting on the commit.
type NeedsMeRow struct {
	Project string        `json:"project"`
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	State   store.State   `json:"state"`
	Reason  NeedsMeReason `json:"reason"`
	Since   time.Time     `json:"since"`
	// Round is the latest Round (Needs Answer).
	Round *RoundSummary `json:"round,omitempty"`
	// Failure is the latest Failure Report (Failed).
	Failure *FailureSummary `json:"failure,omitempty"`
	// Summary is the first line of the finish summary (In Review).
	Summary string `json:"summary,omitempty"`
	// Branch is the Ticket's branch, for the human to merge (To merge).
	Branch string `json:"branch,omitempty"`
	// CIFailure is what failed on the pushed commit (CI failed).
	CIFailure *CIFailureSummary `json:"ci_failure,omitempty"`
	// CIUnavailable says which: the CI Watcher not Watching, or no CI
	// Runs for the pushed commit (CI unavailable).
	CIUnavailable *CIUnavailableView `json:"ci_unavailable,omitempty"`
	// CIItem is the pushed commit's item: the Tickets waiting on it and
	// the failure options (CI failed, CI cancelled and CI unavailable).
	CIItem *CIItemView `json:"ci_item,omitempty"`
	// Blocker is the Cancelled Blocker (blocked by a Cancelled Ticket).
	Blocker *BlockerView `json:"blocker,omitempty"`
	// Link is the Ticket's Link, whatever the reason: the row's badge, and
	// the external item when closed externally.
	Link *LinkView `json:"link,omitempty"`
	// Requirement is set when the Ticket's Spec is a Requirement.
	Requirement *NeedsMeRequirement `json:"requirement,omitempty"`
	// Spec is the Spec a Map Ticket or Prototype is on, as its context,
	// set when its Spec isn't a Requirement (Spec 213).
	Spec *NeedsMeSpec `json:"spec,omitempty"`
	// Prototype is set on a Ticket labelled prototype, for its badge.
	Prototype bool `json:"prototype,omitempty"`
	// CIResult is the Ticket's CI result, whatever the reason, or a CI
	// item's commit's, for the row's badge; left out for a Ticket never
	// pushed while CI Tracking was on.
	CIResult CIResult `json:"ci_result,omitempty"`
	// PullRequest is the Ticket's pull request, for its item's link; left
	// out until the Pull Request Watcher reports one.
	PullRequest *PullRequestView `json:"pull_request,omitempty"`
	// BaseDiverged is the local and remote base commits (Base diverged,
	// whose row has no Ticket: its ID is empty).
	BaseDiverged *BaseDivergedView `json:"base_diverged,omitempty"`
	// BaseMissing is the missing base's branch and the fix (Base missing,
	// whose row has no Ticket either).
	BaseMissing *BaseMissingView `json:"base_missing,omitempty"`
}

// NeedsMeRequirement is the Requirement a Needs Me row's Ticket is on:
// its Spec ID and title, and the Ticket's Stage, set only on a Stage
// Ticket.
type NeedsMeRequirement struct {
	Spec  string `json:"spec"`
	Title string `json:"title"`
	Stage string `json:"stage,omitempty"`
}

// needsMeRequirement is the Requirement t is on, or nil when its Spec
// isn't one.
func needsMeRequirement(r store.Reader, key string, t store.Ticket) *NeedsMeRequirement {
	if !slices.ContainsFunc(specTickets(r, key, t.Spec), func(st store.Ticket) bool { return StageOf(st.Labels) != "" }) {
		return nil
	}
	sp, _ := r.Spec(key, t.Spec)
	return &NeedsMeRequirement{Spec: DisplayID(key, t.Spec), Title: sp.Title, Stage: StageOf(t.Labels)}
}

// NeedsMeSpec is the Spec a Needs Me row's Map Ticket or Prototype is on:
// its Spec ID and title, and whether it is a Map.
type NeedsMeSpec struct {
	Spec  string `json:"spec"`
	Title string `json:"title"`
	Map   bool   `json:"map,omitempty"`
}

// needsMeSpec is the Spec t is on when t is on a Map, or is a Map Ticket
// or a Prototype, and its Spec isn't a Requirement; nil otherwise.
func needsMeSpec(r store.Reader, key string, t store.Ticket, req *NeedsMeRequirement) *NeedsMeSpec {
	if req != nil {
		return nil
	}
	sp, _ := r.Spec(key, t.Spec)
	isMap := slices.Contains(sp.Labels, MapLabel)
	if !isMap && !slices.ContainsFunc(t.Labels, isWayfinderType) {
		return nil
	}
	return &NeedsMeSpec{Spec: DisplayID(key, t.Spec), Title: sp.Title, Map: isMap}
}

// RoundSummary is a Round's number and how many of its Questions are open.
type RoundSummary struct {
	Number    int `json:"number"`
	Open      int `json:"open"`
	Questions int `json:"questions"`
}

// FailureSummary is the latest Failure Report's headline (the first line
// of what_broke) and how many reports the Ticket has.
type FailureSummary struct {
	Headline string `json:"headline"`
	Reports  int    `json:"reports"`
}

// NeedsMe is needs_me, the human's read across every Project.
func (c *Core) NeedsMe(context.Context) (NeedsMeView, error) {
	type waiting struct {
		row NeedsMeRow
		n   int
		// commit orders a Project's CI items, which have no Ticket.
		commit string
	}
	var all []waiting
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			if row, ok := c.baseDivergedRow(p); ok {
				all = append(all, waiting{row: row})
			}
			if row, ok := c.baseMissingRow(p); ok {
				all = append(all, waiting{row: row})
			}
			ci := c.ciStateOf(r, p)
			for _, it := range ci.items() {
				all = append(all, waiting{row: ciItemRow(ci, it), commit: it.commit})
			}
			for _, t := range r.Tickets(p.Key) {
				if row, ok := c.needsMeRow(r, ci, t); ok {
					all = append(all, waiting{row: row, n: t.Number})
				}
			}
		}
		return nil
	})
	slices.SortFunc(all, func(a, b waiting) int {
		return cmp.Or(
			cmp.Compare(groupIndex(a.row.Reason), groupIndex(b.row.Reason)),
			a.row.Since.Compare(b.row.Since),
			cmp.Compare(a.row.Project, b.row.Project),
			cmp.Compare(a.n, b.n),
			cmp.Compare(a.commit, b.commit),
		)
	})
	v := NeedsMeView{Count: len(all), Groups: []NeedsMeGroup{}}
	for _, w := range all {
		if len(v.Groups) == 0 || v.Groups[len(v.Groups)-1].Reason != w.row.Reason {
			v.Groups = append(v.Groups, NeedsMeGroup{Reason: w.row.Reason})
		}
		g := &v.Groups[len(v.Groups)-1]
		g.Tickets = append(g.Tickets, w.row)
	}
	return v, nil
}

// needsMeRow is t's Needs Me row, if it waits on the human with a row
// of its own: a Ticket a CI item lists has none.
func (c *Core) needsMeRow(r store.Reader, ci *ciState, t store.Ticket) (NeedsMeRow, bool) {
	key := ci.p.Key
	reason, since := c.reasonIn(r, ci, t)
	if reason == "" || reason.isCIItem() {
		return NeedsMeRow{}, false
	}
	req := needsMeRequirement(r, key, t)
	row := NeedsMeRow{
		Project: key, ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Reason: reason, Since: since, Link: linkView(t.Link),
		Requirement: req, Spec: needsMeSpec(r, key, t, req), Prototype: slices.Contains(t.Labels, PrototypeLabel),
		CIResult: ci.of(t).result, PullRequest: pullRequestView(t.PullRequest),
	}
	switch reason {
	case ReasonNeedsAnswer:
		if n := len(t.Rounds); n > 0 {
			rd := t.Rounds[n-1]
			s := RoundSummary{Number: rd.Number, Questions: len(rd.Questions)}
			for _, q := range rd.Questions {
				if q.Status == store.QuestionOpen {
					s.Open++
				}
			}
			row.Round = &s
		}
	case ReasonFailed:
		if n := len(t.FailureReports); n > 0 {
			row.Failure = &FailureSummary{Headline: firstLine(t.FailureReports[n-1].WhatBroke), Reports: n}
		}
	case ReasonInReview:
		row.Summary = firstLine(finishSummary(t))
	case ReasonToMerge:
		row.Branch = branchOf(t)
	case ReasonBlockedByCancelled:
		b, _ := cancelledBlocker(c.blockRule(r, key), t)
		row.Blocker = &b
	}
	return row, true
}

// finishSummary is the comment stored with t's last move into In Review,
// or "" when that move had none (a human move).
func finishSummary(t store.Ticket) string {
	at := entered(t)
	for i := len(t.Comments) - 1; i >= 0; i-- {
		if cm := t.Comments[i]; cm.Time.Equal(at) && cm.Actor != string(Human) {
			return cm.Body
		}
	}
	return ""
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// reasonOf is the Ticket's reason and since when it has waited.
func (c *Core) reasonOf(r store.Reader, key string, t store.Ticket) (NeedsMeReason, time.Time) {
	p, _ := r.Project(key) // t is in it, so it exists
	return c.reasonIn(r, c.ciStateOf(r, p), t)
}

// reasonIn is reasonOf reading t's CI from its Project's CI state ci.
func (c *Core) reasonIn(r store.Reader, ci *ciState, t store.Ticket) (NeedsMeReason, time.Time) {
	key := ci.p.Key
	switch t.State {
	case store.NeedsAnswer:
		return ReasonNeedsAnswer, entered(t)
	case store.Answered:
		return ReasonAnswered, entered(t)
	case store.InProgress, store.Accepted:
		p := ci.p
		if a := ci.of(t); a.failure != nil {
			return ReasonCIFailed, a.since
		} else if a.cancelled {
			return ReasonCICancelled, a.since
		}
		if lead, waiting := specPullRequestLead(r, key, t); waiting && !lead {
			// A Spec's pull request is one item, on its lead Ticket.
			return "", time.Time{}
		}
		if t.State == store.Accepted && p.Landing.Method == store.LandingPullRequest && t.PullRequest != nil && t.PullRequest.State == store.PRClosed {
			return ReasonPullRequestClosed, entered(t)
		}
		if t.State == store.Accepted && (p.LandingPolicy().LandsOnHumanMerge()) {
			return ReasonToMerge, entered(t)
		}
		if reason, since := c.timedReason(r, ci, t); reason != "" {
			return reason, since
		}
	case store.Failed:
		return ReasonFailed, entered(t)
	case store.InReview:
		return ReasonInReview, entered(t)
	}
	if b, ok := cancelledBlocker(c.blockRule(r, key), t); ok {
		return ReasonBlockedByCancelled, cancelledAt(r, key, b)
	}
	if since, ok := closedExternally(t); ok {
		return ReasonClosedExternally, since
	}
	if since, ok := newIssue(r, key, t); ok {
		return ReasonNewIssue, since
	}
	return "", time.Time{}
}

// entered is when t moved into its current State (its last write, if no
// move records it).
func entered(t store.Ticket) time.Time {
	for i := len(t.Moves) - 1; i >= 0; i-- {
		if t.Moves[i].To == t.State {
			return t.Moves[i].Time
		}
	}
	return t.Updated
}

// cancelledBlocker is the first open Blocker of t that is Cancelled with
// no Replacements left to follow.
func cancelledBlocker(rule blockRule, t store.Ticket) (BlockerView, bool) {
	for _, b := range rule.openBlockers(t) {
		if b.State == store.Cancelled {
			return b, true
		}
	}
	return BlockerView{}, false
}

// cancelledAt is when the Cancelled Blocker b was cancelled.
func cancelledAt(r store.Reader, key string, b BlockerView) time.Time {
	n, _ := Ref(b.ID).number(key, "blocker")
	bt, _ := r.Ticket(key, n)
	return entered(bt)
}
