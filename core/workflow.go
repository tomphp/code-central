package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/domain"
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
	// FactoryMove moves are the Factory's own, made through a FactoryOnly
	// operation: no Actor takes them as an action, so none is offered
	// them.
	FactoryMove Who = "factory"
)

// Action is one row of the fixed workflow: from which States, to which,
// and who may take it. Every move core makes is checked against
// Workflow, and the published workflow is generated from it.
type Action struct {
	Name string
	From []domain.State
	// To is the State the action moves to; "" means the Actor chooses
	// (move), among every State outside Except.
	To     domain.State
	Except []domain.State
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
var unfinished = []domain.State{domain.Draft, domain.Ready, domain.InProgress, domain.NeedsAnswer, domain.Answered, domain.Failed, domain.InReview, domain.Accepted}

// Workflow is the one action table, in the spec's order.
var Workflow = []Action{
	{Name: "approve", From: []domain.State{domain.Draft}, To: domain.Ready, Who: AnyActor},
	{Name: "defer", From: []domain.State{domain.Ready}, To: domain.Draft, Who: AnyActor,
		Requires: "reason (required), stored as a comment"},
	{Name: "claim", From: []domain.State{domain.Ready}, To: domain.InProgress, Who: AgentActor,
		Requires: "spec or ticket to narrow it (optional), session_id, working_dir and resume_command (optional); returns the Claim ID"},
	{Name: "give_up", From: []domain.State{domain.InProgress}, To: domain.Ready, Who: Holder,
		Requires: "note on where the work stands (required)",
		NextStep: stopWorking},
	{Name: "release", From: []domain.State{domain.InProgress, domain.Answered}, To: domain.Ready, Who: HumanActor,
		Requires: "note (optional)"},
	{Name: "post_round", From: []domain.State{domain.InProgress}, To: domain.NeedsAnswer, Who: Holder,
		Requires: "Questions, each with a title, a body and an optional recommended answer",
		NextStep: postRoundNextStep},
	{Name: "answered", From: []domain.State{domain.NeedsAnswer}, To: domain.Answered, Who: Automatic,
		Requires: "made when the last open Question is answered or dismissed"},
	{Name: "pick_up_answers", From: []domain.State{domain.Answered}, To: domain.InProgress, Who: Holder,
		Requires: "nothing; returns the latest Round with every answer and dismissal, and comments since"},
	{Name: "report_failure", From: []domain.State{domain.InProgress}, To: domain.Failed, Who: Holder,
		Requires: "what_broke, why, what_to_fix and where_work_stands (all required)",
		NextStep: stopWorking},
	{Name: "send_back", From: []domain.State{domain.Failed}, To: domain.Ready, Who: HumanActor,
		Requires: "note (optional)"},
	// An agent that can't land Accepted work (a merge conflict) sends it
	// back for another agent to resolve (ticket 72).
	{Name: "send_back", From: []domain.State{domain.Accepted}, To: domain.Ready, Who: AgentActor,
		Requires: "note (required): why it can't land, a merge conflict's base commit and files"},
	// The human sends Accepted work whose CI failed back to fix it on its
	// branch (code-central-264).
	{Name: "send_back", From: []domain.State{domain.Accepted}, To: domain.Ready, Who: HumanActor,
		Requires: "note (required): what to fix, such as a failed CI run"},
	{Name: "to_draft", From: []domain.State{domain.Failed}, To: domain.Draft, Who: HumanActor},
	{Name: "finish", From: []domain.State{domain.InProgress}, To: domain.InReview, Who: Holder,
		Requires: "summary (required): what was done, where (branch or commit), what to check",
		NextStep: stopWorking},
	// accept has two rows: work whose finish named a branch, in any of
	// its Changed Repositories, is Accepted and waits to land (ADR 0006);
	// any other Ticket goes straight to Done.
	{Name: "accept", From: []domain.State{domain.InReview}, To: domain.Accepted, Who: HumanActor,
		Requires: "nothing; taken when the last finish named a branch"},
	{Name: "accept", From: []domain.State{domain.InReview}, To: domain.Done, Who: HumanActor,
		Requires: "nothing; taken when no finish named a branch"},
	// The Factory fails a finish its finish check finds leaving out a
	// Repository with work, or naming a commit that isn't its branch's
	// head (Spec code-central-848, ADR 0048). The finishing Claim has
	// ended, so it is the Factory's move, not the holder's.
	{Name: failFinish, From: []domain.State{domain.InReview, domain.Accepted}, To: domain.Failed, Who: FactoryMove,
		Requires: "a Failure Report's four parts; from Accepted only while landed nowhere"},
	// implement is the other way to accept a Requirement's spec Stage
	// (ADR 0057): valid only on that Stage, which core checks.
	{Name: "implement", From: []domain.State{domain.InReview}, To: domain.Done, Who: HumanActor,
		Requires: "nothing; only on a spec Stage In Review: accepts it, creates one Ready implementation Ticket for the whole Spec and Cancels the tickets Stage with that Ticket as its Replacement",
		NextStep: "The spec Stage is Done and the tickets Stage is Cancelled, and <id> now builds the whole Spec."},
	{Name: "reject", From: []domain.State{domain.InReview}, To: domain.Ready, Who: HumanActor,
		Requires: "comment (required)"},
	{Name: "land", From: []domain.State{domain.Accepted, domain.InReview}, To: domain.Done, Who: AnyActor,
		Requires: "commit (required) that put the work into the base, recorded on the Move with via merge"},
	{Name: "cancel", From: []domain.State{domain.Draft, domain.Ready}, To: domain.Cancelled, Who: AgentActor,
		Requires: "replacements (required), reason (optional)"},
	{Name: "cancel", From: unfinished, To: domain.Cancelled, Who: HumanActor,
		Requires: "replacements (optional)"},
	// Needs Answer and Failed are entered only by posting a Round or a
	// Failure Report, so move can't reach either.
	{Name: "move", From: domain.States, Except: []domain.State{domain.NeedsAnswer, domain.Failed}, Who: HumanActor,
		Requires: "to_state: any State but the current one and those in except"},
}

// claimStates are the States a Claim spans. Any move out of them ends it.
var claimStates = []domain.State{domain.InProgress, domain.NeedsAnswer, domain.Answered}

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
func allowedActions(state domain.State, actor Actor) []string {
	out := []string{}
	for _, a := range Workflow {
		if slices.Contains(a.From, state) && a.Who.may(actor) && !slices.Contains(out, a.Name) {
			out = append(out, a.Name)
		}
	}
	return out
}

// check finds the row letting actor take action name on a Ticket in
// state, or returns action_not_allowed naming the allowed actions. When
// to is set, a row with a fixed To elsewhere doesn't match: an action
// with several rows from one State (accept) picks its row by target.
func check(name string, state, to domain.State, actor Actor, ticketID string) (Action, error) {
	humanOnly := false
	for _, a := range Workflow {
		if a.Name != name || !slices.Contains(a.From, state) || (to != "" && a.To != "" && a.To != to) {
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
func liveClaim(t domain.Ticket) (*domain.Claim, bool) {
	for i := range t.Claims {
		if t.Claims[i].Live() {
			return &t.Claims[i], true
		}
	}
	return nil, false
}

// holder checks that claimID is the Ticket's live Claim.
func holder(t *domain.Ticket, key, claimID string) (*domain.Claim, error) {
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
func transition(ctx context.Context, t *domain.Ticket, key, name, claimID string, now time.Time) error {
	return transitionTo(ctx, t, key, name, claimID, "", now)
}

// transitionTo is transition for an action whose row leaves the target
// to the Actor (move): to is used when the row's To is "". Move has
// already checked to against the row's Except. For an action with a row
// per target (accept), to picks the row.
func transitionTo(ctx context.Context, t *domain.Ticket, key, name, claimID string, to domain.State, now time.Time) error {
	actor := ActorOf(ctx)
	id := DisplayID(key, t.Number)
	if isHolderAction(name) {
		if _, err := holder(t, key, claimID); err != nil {
			return err
		}
	}
	var a Action
	var err error
	switch name {
	case automaticAnswered:
		a, err = automatic(name, t.State)
	case failFinish:
		a, err = factoryMove(name, t.State, id)
	default:
		a, err = check(name, t.State, to, actor, id)
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
	if from == domain.NeedsAnswer && name != automaticAnswered {
		dismissOpen(t, now)
	}
	m := domain.Move{From: from, To: to, Actor: string(actor), Time: now}
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
func automatic(name string, state domain.State) (Action, error) {
	for _, a := range Workflow {
		if a.Name == name && a.Who == Automatic && slices.Contains(a.From, state) {
			return a, nil
		}
	}
	return Action{}, fmt.Errorf("core: no automatic move %s from %s", name, state)
}

// factoryMove finds the Factory's row name from state, or refuses with
// the States it is made from. Whoever calls the Factory's operation makes
// it: the operation's audience keeps it to the Factory.
func factoryMove(name string, state domain.State, ticketID string) (Action, error) {
	var from []string
	for _, a := range Workflow {
		if a.Name != name || a.Who != FactoryMove {
			continue
		}
		if slices.Contains(a.From, state) {
			return a, nil
		}
		for _, s := range a.From {
			from = append(from, string(s))
		}
	}
	return Action{}, &Error{Code: ActionNotAllowed, Message: fmt.Sprintf("%s is %s, so %s is not allowed: it is made only from %s", ticketID, state, name, strings.Join(from, " or "))}
}

func isHolderAction(name string) bool {
	for _, a := range Workflow {
		if a.Name == name && a.Who == Holder {
			return true
		}
	}
	return false
}
