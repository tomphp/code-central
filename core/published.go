package core

import (
	"context"
	_ "embed"
	"maps"

	"github.com/tomoram/code-central/store"
)

// The published workflow: what an agent session is taught by the server
// rather than by copies in a repository. The MCP instructions reach every
// session; get_workflow holds the rest; each write's next step arrives
// when it matters. A core test keeps all of it within its context budget.

// Instructions are the MCP server instructions every session receives.
const Instructions = "Code Central holds this Project's work: Specs, and the Tickets that deliver them, " +
	"each in one State of a fixed workflow, and it shows the human what needs them. " +
	"Call get_workflow before working a Ticket: it has every State, action and rule. " +
	"The Round rule: while you hold a Claim, every question for the human is a Round posted with post_round, " +
	"never printed in the terminal, even when the human is at the terminal. " +
	"When a skill grills, each numbered question becomes one Question, with its recommendation as the recommended answer. " +
	"Every write tells you its next step; follow it. " +
	"To import a `.scratch/` folder, call `get_import_guide`."

//go:embed workflow.md
var workflowProse string

// stateMeanings are the nine States' meanings, in workflow order.
var stateMeanings = map[store.State]string{
	store.Draft:       "Still being defined, so no agent should pick it up",
	store.Ready:       "An agent can claim it",
	store.InProgress:  "An agent has claimed it and is working",
	store.NeedsAnswer: "The agent is stopped on an unanswered Question",
	store.Answered:    "Every Question is answered and the asking session has not yet picked the work up",
	store.Failed:      "The agent stopped because something broke",
	store.InReview:    "The work is finished and waiting for the human to accept it",
	store.Done:        "Finished",
	store.Cancelled:   "Abandoned",
}

// WorkflowView is get_workflow's result: the table core enforces, as
// data, plus the prose that explains how to work within it.
type WorkflowView struct {
	States  []StateView         `json:"states"`
	Actions []ActionView        `json:"actions"`
	NeedsMe []NeedsMeReasonView `json:"needs_me"`
	Prose   string              `json:"prose"`
}

// NeedsMeReasonView is one Needs Me reason code, in group order, and
// what it means.
type NeedsMeReasonView struct {
	Reason  NeedsMeReason `json:"reason"`
	Meaning string        `json:"meaning"`
}

// StateView is one State and what it means.
type StateView struct {
	Name    store.State `json:"name"`
	Meaning string      `json:"meaning"`
}

// ActionView is one row of the action table. To is empty when the Actor
// chooses it (move), among every State outside Except. ClaimID says the
// action must send the live Claim ID.
type ActionView struct {
	Name     string        `json:"name"`
	From     []store.State `json:"from"`
	To       store.State   `json:"to,omitempty"`
	Except   []store.State `json:"except,omitempty"`
	Who      Who           `json:"who"`
	ClaimID  bool          `json:"claim_id"`
	Requires string        `json:"requires,omitempty"`
	NextStep string        `json:"next_step,omitempty"`
}

// GetWorkflow is get_workflow: the whole workflow, generated from the
// same table every move is checked against, so the two can't drift.
func (c *Core) GetWorkflow(context.Context) (WorkflowView, error) {
	w := WorkflowView{NeedsMe: needsMeReasons, Prose: workflowProse}
	for _, s := range store.States {
		w.States = append(w.States, StateView{Name: s, Meaning: stateMeanings[s]})
	}
	for _, a := range Workflow {
		w.Actions = append(w.Actions, ActionView{
			Name: a.Name, From: a.From, To: a.To, Except: a.Except, Who: a.Who,
			ClaimID: a.Who == Holder, Requires: a.Requires, NextStep: a.NextStep,
		})
	}
	return w, nil
}

// NextSteps lists every action's next step, by action name, for the
// context budget test. Import's two are included.
func NextSteps() map[string]string {
	out := maps.Clone(importNextSteps)
	for _, a := range Workflow {
		if a.NextStep != "" {
			out[a.Name] = a.NextStep
		}
	}
	return out
}

// nextStep is the next step the named action's write returns.
func nextStep(name string) string { return NextSteps()[name] }
