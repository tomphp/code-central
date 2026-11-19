package api

import "github.com/tomoram/code-central/domain"

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
	Name    domain.State `json:"name"`
	Meaning string       `json:"meaning"`
}

// ActionView is one row of the action table. To is empty when the Actor
// chooses it (move), among every State outside Except. ClaimID says the
// action must send the live Claim ID.
type ActionView struct {
	Name     string         `json:"name"`
	From     []domain.State `json:"from"`
	To       domain.State   `json:"to,omitempty"`
	Except   []domain.State `json:"except,omitempty"`
	Who      Who            `json:"who"`
	ClaimID  bool           `json:"claim_id"`
	Requires string         `json:"requires,omitempty"`
	NextStep string         `json:"next_step,omitempty"`
}
