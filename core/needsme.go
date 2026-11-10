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
	// ReasonToMerge is a Ticket Accepted under the Branch Landing Method
	// (code-central-175): the human merges its branch, at once.
	ReasonToMerge NeedsMeReason = "to_merge"
	// ReasonAcceptedNotLanded is a Ticket Accepted for more than
	// NotLandedAfter (ticket 72): its work hasn't reached the base.
	ReasonAcceptedNotLanded  NeedsMeReason = "accepted_not_landed"
	ReasonBlockedByCancelled NeedsMeReason = "blocked_by_cancelled"
	// ReasonClosedExternally is a linked Ticket whose external item isn't
	// open, whose Link isn't detached and that isn't finished (ticket 42).
	ReasonClosedExternally NeedsMeReason = "closed_externally"
)

// needsMeReasons are the reasons in group order, each with what it means
// for the published workflow.
var needsMeReasons = []NeedsMeReasonView{
	{ReasonNeedsAnswer, "Needs Answer: a Round waits for the human's answers"},
	{ReasonAnswered, "Answered: the answers wait for the asking session to pick them up"},
	{ReasonSilentClaim, "Silent Claim: In Progress with no activity for 30 minutes"},
	{ReasonLongClaim, "Long Claim: In Progress and active, but no move (claim or pick_up_answers) for more than 60 minutes"},
	{ReasonFailed, "Failed: a Failure Report waits for the human"},
	{ReasonInReview, "In Review: finished work waits to be accepted or rejected"},
	{ReasonToMerge, "To merge: Accepted under the Branch Landing Method; merge its branch into the base"},
	{ReasonAcceptedNotLanded, "Accepted but not landed: Accepted for more than 10 minutes without landing in the base"},
	{ReasonBlockedByCancelled, "Blocked by a Cancelled Ticket with no Replacements"},
	{ReasonClosedExternally, "Closed externally: its linked item was closed, deleted or left its Integration's query; cancel it, mark it Done or unlink it"},
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
// the Failure Report, the finish, the accept (To merge, and Accepted but
// not landed),
// or the Blocker's cancel. Only the
// detail field for its reason is set.
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
	// CIResult is the Ticket's CI result, whatever the reason, for the
	// row's badge; left out for a Ticket never pushed while CI Tracking
	// was on.
	CIResult CIResult `json:"ci_result,omitempty"`
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
	}
	var all []waiting
	c.store.View(func(r store.Reader) error {
		for _, p := range r.Projects() {
			for _, t := range r.Tickets(p.Key) {
				if row, ok := c.needsMeRow(r, p.Key, t); ok {
					all = append(all, waiting{row, t.Number})
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

// needsMeRow is t's Needs Me row, if it waits on the human.
func (c *Core) needsMeRow(r store.Reader, key string, t store.Ticket) (NeedsMeRow, bool) {
	reason, since := c.reasonOf(r, key, t)
	if reason == "" {
		return NeedsMeRow{}, false
	}
	req := needsMeRequirement(r, key, t)
	row := NeedsMeRow{
		Project: key, ID: DisplayID(key, t.Number), Title: t.Title, State: t.State, Reason: reason, Since: since, Link: linkView(t.Link),
		Requirement: req, Spec: needsMeSpec(r, key, t, req), Prototype: slices.Contains(t.Labels, PrototypeLabel),
		CIResult: ticketCIResult(r, key, t, c.now()),
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
		row.Branch = t.Branch
	case ReasonBlockedByCancelled:
		b, _ := cancelledBlocker(r, key, t)
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
	switch t.State {
	case store.NeedsAnswer:
		return ReasonNeedsAnswer, entered(t)
	case store.Answered:
		return ReasonAnswered, entered(t)
	case store.InProgress, store.Accepted:
		p, _ := r.Project(key) // t is in it, so it exists
		if t.State == store.Accepted && p.Landing.Method == store.LandingBranch {
			return ReasonToMerge, entered(t)
		}
		if reason, since := c.timedReason(p, t); reason != "" {
			return reason, since
		}
	case store.Failed:
		return ReasonFailed, entered(t)
	case store.InReview:
		return ReasonInReview, entered(t)
	}
	if b, ok := cancelledBlocker(r, key, t); ok {
		return ReasonBlockedByCancelled, cancelledAt(r, key, b)
	}
	if since, ok := closedExternally(t); ok {
		return ReasonClosedExternally, since
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
func cancelledBlocker(r store.Reader, key string, t store.Ticket) (BlockerView, bool) {
	for _, b := range openBlockers(r, key, t) {
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
