package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// Who says which Actor may take an Action.
type Who string

// The Who values of the action table.
const (
	// AnyActor is "Human or agent".
	AnyActor Who = "human or agent"
	// AgentActor is "Agent". The human can do everything agents can, so
	// any Actor may take these.
	AgentActor Who = "agent"
	// Holder is "Agent (holder)": the agent must send the live Claim ID.
	Holder Who = "agent (holder)"
	// HumanActor is "Human": only the human Actor.
	HumanActor Who = "human"
	// Automatic moves are made by core itself, never asked for.
	Automatic Who = "automatic"
)

// Action is one row of the fixed workflow: from which States, to which,
// and who may take it. Every move core makes is checked against
// Workflow, and the published workflow is generated from it.
type Action struct {
	Name string
	From []store.State
	// To is the State the action moves to; "" means the Actor chooses
	// (move), among every State outside Except.
	To     store.State
	Except []store.State
	Who    Who
	// Requires names the content the action takes, for the published
	// workflow; "" means none beyond the Ticket (and the Claim ID for
	// Holder actions).
	Requires string
	// NextStep is the one sentence a write returns to tell the agent
	// what to do now; "" means the action returns none.
	NextStep string
}

// unfinished is every State except Done and Cancelled.
var unfinished = []store.State{store.Draft, store.Ready, store.InProgress, store.NeedsAnswer, store.Answered, store.Failed, store.InReview}

// Workflow is the one action table, in the spec's order.
var Workflow = []Action{
	{Name: "approve", From: []store.State{store.Draft}, To: store.Ready, Who: AnyActor},
	{Name: "defer", From: []store.State{store.Ready}, To: store.Draft, Who: AnyActor,
		Requires: "reason (required), stored as a comment"},
	{Name: "claim", From: []store.State{store.Ready}, To: store.InProgress, Who: AgentActor,
		Requires: "spec or ticket to narrow it (optional), session_id, working_dir and resume_command (optional); returns the Claim ID"},
	{Name: "give_up", From: []store.State{store.InProgress}, To: store.Ready, Who: Holder,
		Requires: "note on where the work stands (required)",
		NextStep: stopWorking},
	{Name: "release", From: []store.State{store.InProgress, store.Answered}, To: store.Ready, Who: HumanActor,
		Requires: "note (optional)"},
	{Name: "post_round", From: []store.State{store.InProgress}, To: store.NeedsAnswer, Who: Holder,
		Requires: "Questions, each with a title, a body and an optional recommended answer",
		NextStep: postRoundNextStep},
	{Name: "answered", From: []store.State{store.NeedsAnswer}, To: store.Answered, Who: Automatic,
		Requires: "made when the last open Question is answered or dismissed"},
	{Name: "pick_up_answers", From: []store.State{store.Answered}, To: store.InProgress, Who: Holder,
		Requires: "nothing; returns the latest Round with every answer and dismissal, and comments since"},
	{Name: "report_failure", From: []store.State{store.InProgress}, To: store.Failed, Who: Holder,
		Requires: "what_broke, why, what_to_fix and where_work_stands (all required)",
		NextStep: stopWorking},
	{Name: "send_back", From: []store.State{store.Failed}, To: store.Ready, Who: HumanActor,
		Requires: "note (optional)"},
	{Name: "to_draft", From: []store.State{store.Failed}, To: store.Draft, Who: HumanActor},
	{Name: "finish", From: []store.State{store.InProgress}, To: store.InReview, Who: Holder,
		Requires: "summary (required): what was done, where (branch or commit), what to check",
		NextStep: stopWorking},
	{Name: "accept", From: []store.State{store.InReview}, To: store.Done, Who: HumanActor},
	{Name: "reject", From: []store.State{store.InReview}, To: store.Ready, Who: HumanActor,
		Requires: "comment (required)"},
	{Name: "cancel", From: []store.State{store.Draft, store.Ready}, To: store.Cancelled, Who: AgentActor,
		Requires: "replacements (required), reason (optional)"},
	{Name: "cancel", From: unfinished, To: store.Cancelled, Who: HumanActor,
		Requires: "replacements (optional)"},
	// Needs Answer and Failed are entered only by posting a Round or a
	// Failure Report, so move can't reach either.
	{Name: "move", From: store.States, Except: []store.State{store.NeedsAnswer, store.Failed}, Who: HumanActor,
		Requires: "to_state: any State but the current one and those in except"},
}

// claimStates are the States a Claim spans. Any move out of them ends it.
var claimStates = []store.State{store.InProgress, store.NeedsAnswer, store.Answered}

// may reports whether actor may take an action with this Who. Holder
// actions also need the live Claim ID, which liveClaim checks.
func (w Who) may(actor Actor) bool {
	if actor == Integration {
		// An Integration never moves State (ADR 0004).
		return false
	}
	switch w {
	case AnyActor, AgentActor, Holder:
		return true
	case HumanActor:
		return actor == Human
	}
	return false
}

// allowedActions names the actions actor may take from state, in table
// order, each once.
func allowedActions(state store.State, actor Actor) []string {
	out := []string{}
	for _, a := range Workflow {
		if slices.Contains(a.From, state) && a.Who.may(actor) && !slices.Contains(out, a.Name) {
			out = append(out, a.Name)
		}
	}
	return out
}

// check finds the row letting actor take action name on a Ticket in
// state, or returns action_not_allowed naming the allowed actions.
func check(name string, state store.State, actor Actor, ticketID string) (Action, error) {
	humanOnly := false
	for _, a := range Workflow {
		if a.Name != name || !slices.Contains(a.From, state) {
			continue
		}
		if a.Who.may(actor) {
			return a, nil
		}
		humanOnly = humanOnly || a.Who == HumanActor
	}
	allowed := allowedActions(state, actor)
	msg := fmt.Sprintf("%s is %s, so %s is not allowed", ticketID, state, name)
	if humanOnly {
		msg = fmt.Sprintf("%s is a human action; this request is made as %s", name, actor)
	}
	if len(allowed) > 0 {
		msg += "; allowed from " + string(state) + ": " + strings.Join(allowed, ", ")
	}
	return Action{}, &Error{Code: ActionNotAllowed, Message: msg, AllowedActions: allowed}
}

// liveClaim returns the Ticket's live Claim, if any.
func liveClaim(t store.Ticket) (*store.Claim, bool) {
	for i := range t.Claims {
		if t.Claims[i].Live() {
			return &t.Claims[i], true
		}
	}
	return nil, false
}

// holder checks that claimID is the Ticket's live Claim.
func holder(t *store.Ticket, key, claimID string) (*store.Claim, error) {
	if c, ok := liveClaim(*t); ok && claimID != "" && c.ID == claimID {
		return c, nil
	}
	return nil, &Error{Code: StaleClaim, Message: fmt.Sprintf(
		"that Claim ID is not the live Claim on %s: stop working this Ticket", DisplayID(key, t.Number))}
}

// transition is one checked move of t by the action name, to the row's
// State (or to, for move). It records the move with the Actor, the time
// and the live Claim's number, and ends the Claim when the Ticket leaves
// the States a Claim spans. Holder actions must pass the live Claim ID.
func transition(ctx context.Context, t *store.Ticket, key, name, claimID string, now time.Time) error {
	return transitionTo(ctx, t, key, name, claimID, "", now)
}

// transitionTo is transition for an action whose row leaves the target
// to the Actor (move): to is used when the row's To is "". Move has
// already checked to against the row's Except.
func transitionTo(ctx context.Context, t *store.Ticket, key, name, claimID string, to store.State, now time.Time) error {
	actor := ActorOf(ctx)
	id := DisplayID(key, t.Number)
	if isHolderAction(name) {
		if _, err := holder(t, key, claimID); err != nil {
			return err
		}
	}
	var a Action
	var err error
	if name == automaticAnswered {
		a, err = automatic(name, t.State)
	} else {
		a, err = check(name, t.State, actor, id)
	}
	if err != nil {
		return err
	}
	from := t.State
	if a.To != "" {
		to = a.To
	}
	// Leaving Needs Answer other than by the last answer is a move by
	// hand: its open Questions become dismissed. The Claim ends only when
	// the Ticket leaves the Claim States.
	if from == store.NeedsAnswer && name != automaticAnswered {
		dismissOpen(t, now)
	}
	m := store.Move{From: from, To: to, Actor: string(actor), Time: now}
	if c, ok := liveClaim(*t); ok {
		m.Claim = c.Number
		if slices.Contains(claimStates, from) && !slices.Contains(claimStates, to) {
			c.Ended, c.EndedBy = now, name
		}
	}
	t.State, t.Updated = to, now
	t.Moves = append(t.Moves, m)
	return nil
}

// automaticAnswered is the automatic move made when a Round's last open
// Question is answered or dismissed. Only core makes it.
const automaticAnswered = "answered"

// automatic finds the Automatic row name from state.
func automatic(name string, state store.State) (Action, error) {
	for _, a := range Workflow {
		if a.Name == name && a.Who == Automatic && slices.Contains(a.From, state) {
			return a, nil
		}
	}
	return Action{}, fmt.Errorf("core: no automatic move %s from %s", name, state)
}

func isHolderAction(name string) bool {
	for _, a := range Workflow {
		if a.Name == name && a.Who == Holder {
			return true
		}
	}
	return false
}
