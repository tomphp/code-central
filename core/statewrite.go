package core

import (
	"slices"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// The work item State due (Spec code-central-1215): as with comments and
// tags, Core works it out from the work's state, never from events. It
// is the mapped State of the furthest phase the linked work reached after
// that mapping was turned on, until its key is reported written; once a
// phase's key is written, it and every nearer phase are never due again.
// Unlike tags, it is due on a work item out of scope, and under Hide Code
// Central, as a State names nothing.

// phase is one of the phases linked work reaches, with its Written key
// and its Work Item States mapping.
type phase struct {
	key     string
	mapping func(domain.WorkItemStates) domain.WorkItemState
}

// The phases' places in phases, nearest first.
const (
	inProgressPhase = iota
	acceptedPhase
	donePhase
)

var phases = [...]phase{
	inProgressPhase: {domain.StateKeyInProgress, func(s domain.WorkItemStates) domain.WorkItemState { return s.InProgress }},
	acceptedPhase:   {domain.StateKeyAccepted, func(s domain.WorkItemStates) domain.WorkItemState { return s.Accepted }},
	donePhase:       {domain.StateKeyDone, func(s domain.WorkItemStates) domain.WorkItemState { return s.Done }},
}

// reached is when linked work reached each of phases, the zero time for
// one it hasn't.
type reached [len(phases)]time.Time

// stateWrite is the State due on a work item whose Link has the written
// keys, its work having reached each phase at, as states maps them; nil
// when none is.
func stateWrite(states domain.WorkItemStates, written []string, at reached) *api.WorkItemStateWrite {
	for i := len(phases) - 1; i >= 0; i-- {
		p := phases[i]
		if slices.Contains(written, p.key) {
			return nil
		}
		m := p.mapping(states)
		if m.Name == "" || !at[i].After(m.Since) {
			continue
		}
		w := &api.WorkItemStateWrite{Name: m.Name, Key: p.key, Nearer: []string{}}
		for _, nearer := range phases[:i] {
			if !slices.Contains(written, nearer.key) {
				w.Nearer = append(w.Nearer, nearer.key)
			}
		}
		return w
	}
	return nil
}

// stateWrites is the State due on the work item of each Spec in key with
// an outbound Azure Boards Link, by number, as states maps them.
func stateWrites(r store.Reader, key string, states domain.WorkItemStates) map[int]api.WorkItemWrites {
	out := map[int]api.WorkItemWrites{}
	if states == (domain.WorkItemStates{}) || inboxOf(r, key, domain.IntegrationAzureBoards).spec == 0 {
		return out
	}
	for _, s := range r.FindSpecs(key, store.SpecFilter{Link: &store.LinkFilter{Integration: domain.IntegrationAzureBoards}}) {
		if !s.Link.Attached() || !s.Link.Outbound || s.Link.ExternalState == domain.ExternalDeleted {
			continue
		}
		if due := stateWrite(states, s.Link.Written, specReached(specTickets(r, key, s.Number, store.Moves))); due != nil {
			out[s.Number] = api.WorkItemWrites{Target: domain.DisplayID(key, s.Number), Link: *linkView(s.Link), Comments: []api.IssueComment{}, State: due}
		}
	}
	return out
}

// specReached is when the work on a Spec with tickets reached each
// phase. In Progress is the first Move of any of them to In Progress.
// Accepted is while every Stage Ticket is finished and every
// implementation Ticket is Accepted or finished, with one Accepted at
// least, and Done is once the Spec is Finished with something landed:
// each at the latest Move into its Tickets' States. A Spec Finished with
// nothing landed reaches none, as nothing is due after it.
func specReached(tickets []domain.Ticket) reached {
	var at reached
	settled, accepted, landed := len(tickets) > 0, false, false
	var latest time.Time
	for _, t := range tickets {
		for _, m := range t.Moves {
			if first := &at[inProgressPhase]; m.To == domain.InProgress && (first.IsZero() || m.Time.Before(*first)) {
				*first = m.Time
			}
		}
		implementation := domain.StageOf(t.Labels) == ""
		accepted = accepted || implementation && t.State == domain.Accepted
		landed = landed || implementation && t.State == domain.Done
		settled = settled && (finished(t.State) || implementation && t.State == domain.Accepted)
		if in := movedIn(t); in.After(latest) {
			latest = in
		}
	}
	switch {
	case !settled:
	case accepted:
		at[acceptedPhase] = latest
	case landed:
		at[donePhase] = latest
	default:
		return reached{}
	}
	return at
}

// movedIn is when t last moved into its State: when it was last updated
// if no Move did, as for one imported in it.
func movedIn(t domain.Ticket) time.Time {
	for _, m := range slices.Backward(t.Moves) {
		if m.To == t.State {
			return m.Time
		}
	}
	return t.Updated
}
