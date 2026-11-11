package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Land per Spec (code-central-371): under Merge with the option on, an
// implementation Ticket of a Requirement is merged into its Spec's Spec
// Branch when accepted, and the Spec lands once all its Tickets are
// accepted. Core works out which Tickets this covers, in one place, and
// when a Spec is ready; the Factory does the git.

// specLanded is whether t is a Spec-landed Ticket in a Project with
// landing: the Project is under Merge with Land per Spec on, t is an
// implementation Ticket (not a Stage, Map or Prototype Ticket), and its
// Spec is a Requirement Spec, one with Stages. Long-lived Specs and plain
// Specs land Ticket by Ticket.
func specLanded(r store.Reader, landing store.Landing, key string, t store.Ticket) bool {
	if landing.Method != store.LandingMerge || !landing.LandPerSpec {
		return false
	}
	if StageOf(t.Labels) != "" || isMap(t.Labels) || slices.Contains(t.Labels, PrototypeLabel) {
		return false
	}
	return isRequirement(r, key, t.Spec)
}

// isRequirement is whether Spec n has a Stage Ticket.
func isRequirement(r store.Reader, key string, n int) bool {
	return slices.ContainsFunc(r.Tickets(key), func(t store.Ticket) bool {
		return t.Spec == n && StageOf(t.Labels) != ""
	})
}

// specMergeCounts is whether t has a Spec merge that counts: one after
// t's last move back to Ready, the way a pushed commit counts.
func specMergeCounts(t store.Ticket) bool {
	return t.SpecMerge != nil && t.SpecMerge.Merged.After(backToReady(t))
}

// waitingForSpec is whether t is Spec-landed and Accepted but not yet in
// the base: it waits for its Spec to land, and the Factory has pushed
// nothing for it. Once its Spec is merged into the base and pushed under
// CI Tracking, it has a pushed commit and waits on CI instead.
func waitingForSpec(r store.Reader, landing store.Landing, key string, t store.Ticket) bool {
	if t.State != store.Accepted || !specLanded(r, landing, key, t) {
		return false
	}
	return len(t.PushedCommits) == 0 || !latestPushed(t).Pushed.After(backToReady(t))
}

// SpecMergeView is the Spec Branch commit a Ticket's work was merged
// into and when Code Central heard of it; Counts is false for a merge no
// later than the Ticket's last move back to Ready.
type SpecMergeView struct {
	Commit string    `json:"commit"`
	Merged time.Time `json:"merged"`
	Counts bool      `json:"counts"`
}

// LandPerSpecView is a Requirement's Land per Spec status on a Spec read,
// present only while the Project is under Merge with Land per Spec on.
type LandPerSpecView struct {
	// Waiting are the Accepted Tickets waiting for the Spec to land.
	Waiting []string `json:"waiting"`
	// Ready is whether the Spec is ready to land: the Factory merges it
	// into the base.
	Ready bool `json:"ready"`
}

// landPerSpecView is the Land per Spec status of Requirement Spec n with
// these Tickets, or nil when the option doesn't apply.
func landPerSpecView(r store.Reader, key string, tickets []store.Ticket) *LandPerSpecView {
	p, _ := r.Project(key)
	if p.Landing.Method != store.LandingMerge || !p.Landing.LandPerSpec {
		return nil
	}
	v := &LandPerSpecView{Waiting: []string{}}
	ready, merged := true, false
	for _, t := range tickets {
		if StageOf(t.Labels) == "tickets" {
			ready = ready && finished(t.State)
			continue
		}
		if StageOf(t.Labels) != "" {
			continue
		}
		switch t.State {
		case store.Done, store.Cancelled:
		case store.Accepted:
			if waitingForSpec(r, p.Landing, key, t) {
				v.Waiting = append(v.Waiting, DisplayID(key, t.Number))
				merged = merged || specMergeCounts(t)
			}
		default:
			ready = false
		}
	}
	v.Ready = ready && merged
	return v
}

// ReportSpecMergeInput is the Factory's report_spec_merge(ticket, commit,
// full?).
type ReportSpecMergeInput struct {
	Ticket Ref    `json:"ticket"`
	Commit string `json:"commit"`
	// Full asks for the Ticket in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportSpecMerge is the Factory's report_spec_merge: the Spec Branch
// commit it merged an Accepted Spec-landed Ticket's work into, recorded
// with when. Reporting the same commit again changes nothing; a newer one
// replaces it.
func (c *Core) ReportSpecMerge(_ context.Context, key string, in ReportSpecMergeInput) (Written[TicketView], error) {
	commit := strings.TrimSpace(in.Commit)
	if commit == "" {
		return Written[TicketView]{}, invalid("commit", "report_spec_merge needs the Spec Branch commit the Factory merged into")
	}
	return c.updateTicketIn(key, "report_spec_merge", in.Ticket, in.Full, func(r store.Reader, t *store.Ticket) (bool, error) {
		p, _ := r.Project(key)
		if t.State != store.Accepted || !specLanded(r, p.Landing, key, *t) {
			return false, &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is %s: only an Accepted Spec-landed Ticket is merged into its Spec Branch", DisplayID(key, t.Number), t.State)}
		}
		if t.SpecMerge != nil && t.SpecMerge.Commit == commit && specMergeCounts(*t) {
			return false, nil
		}
		t.SpecMerge = &store.SpecMerge{Commit: commit, Merged: c.now()}
		return true, nil
	})
}
