package api

import "fmt"

// OpenSpecMergeInput is the Factory's open_spec_merge(spec, conflicts,
// base_tip, spec_branch_tip, repositories?): the files the Spec Branch
// conflicts with the base in, and both branches' tips. Those are the
// Home Repository's; repositories gives each conflicting Repository's
// the same way, the Home Repository's too when it isn't given above
// (Spec code-central-848).
type OpenSpecMergeInput struct {
	Spec          Ref                 `json:"spec" path:"n"`
	Conflicts     []string            `json:"conflicts"`
	BaseTip       string              `json:"base_tip"`
	SpecBranchTip string              `json:"spec_branch_tip"`
	Repositories  []SpecMergeConflict `json:"repositories,omitempty"`
}

// SpecMergeConflict is how the Spec Branch conflicts with the base in the
// Repository Name: the files, and both branches' tips there.
type SpecMergeConflict struct {
	Name          string   `json:"name"`
	Conflicts     []string `json:"conflicts"`
	BaseTip       string   `json:"base_tip"`
	SpecBranchTip string   `json:"spec_branch_tip"`
}

// SpecMergeOutcome says what open_spec_merge did.
type SpecMergeOutcome string

// The open_spec_merge outcomes.
const (
	// SpecMergeOpened is a new Ready Spec merge Ticket.
	SpecMergeOpened SpecMergeOutcome = "opened"
	// SpecMergeCommented is the conflict commented on the open one.
	SpecMergeCommented SpecMergeOutcome = "commented"
	// SpecMergeUnchanged is nothing written: the open one already says it.
	SpecMergeUnchanged SpecMergeOutcome = "unchanged"
)

// SpecMergeOutcomes are the open_spec_merge outcomes.
func SpecMergeOutcomes() []SpecMergeOutcome {
	return []SpecMergeOutcome{SpecMergeOpened, SpecMergeCommented, SpecMergeUnchanged}
}

// OpenSpecMergeResult is open_spec_merge's result: the Spec merge Ticket
// and what was done to it.
type OpenSpecMergeResult struct {
	Ticket  string           `json:"ticket"`
	Outcome SpecMergeOutcome `json:"outcome"`
}

// IsSpecMergeTicket reports whether v is its Spec's Spec merge Ticket:
// marked, or unmarked (opened before the mark existed) with either of
// the Spec merge titles.
func (v TicketView) IsSpecMergeTicket() bool {
	return v.SpecMergeTicket || IsSpecMergeTitle(v.Spec.ID, v.Title)
}

// SpecMergeTitle is the title of the Ticket that merges the base into the
// Spec Branch of spec, a display ID: the Spec merge Ticket's title with
// Rebase off.
func SpecMergeTitle(spec string) string {
	return fmt.Sprintf("Merge the base into the Spec Branch of %s", spec)
}

// SpecMergeRebaseTitle is the title of the Ticket that rebases the Spec
// Branch of spec, a display ID, onto the base: the Spec merge Ticket's
// title with Rebase in force in any conflicting Repository (ADR 0067).
func SpecMergeRebaseTitle(spec string) string {
	return fmt.Sprintf("Rebase the Spec Branch of %s onto the base", spec)
}

// IsSpecMergeTitle reports whether title is either of the Spec merge
// Ticket's titles for spec, a display ID, which is how an unmarked Spec
// merge Ticket (one opened before the mark existed) is recognised.
func IsSpecMergeTitle(spec, title string) bool {
	return title == SpecMergeTitle(spec) || title == SpecMergeRebaseTitle(spec)
}
