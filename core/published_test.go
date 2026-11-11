package core_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// The published workflow (ticket 26): get_workflow, the MCP instructions
// and the next steps, and the context budget they must stay inside.

func TestTheWorkflowListsEveryStateWithItsMeaning(t *testing.T) {
	w := workflow(t)
	var names []store.State
	for _, s := range w.States {
		names = append(names, s.Name)
		if s.Meaning == "" {
			t.Errorf("State %s has no meaning", s.Name)
		}
	}
	if !slices.Equal(names, store.States) {
		t.Fatalf("States %v, want the ten in workflow order", names)
	}
	if w.States[3].Meaning != "The agent is stopped on an unanswered Question" {
		t.Fatalf("Needs Answer means %q", w.States[3].Meaning)
	}
}

func TestTheWorkflowsActionsAreTheTableCoreEnforces(t *testing.T) {
	w := workflow(t)
	if len(w.Actions) != len(core.Workflow) {
		t.Fatalf("%d actions, want one per row of core.Workflow (%d)", len(w.Actions), len(core.Workflow))
	}
	finish := action(t, w, "finish", "agent (holder)")
	if !slices.Equal(finish.From, []store.State{store.InProgress}) || finish.To != store.InReview {
		t.Fatalf("finish %+v", finish)
	}
	if finish.Requires == "" || !finish.ClaimID {
		t.Fatalf("finish should require a summary and the Claim ID: %+v", finish)
	}
	if finish.NextStep != "stop working this Ticket" {
		t.Fatalf("finish's next step %q", finish.NextStep)
	}
	if a := action(t, w, "approve", "human or agent"); a.ClaimID || a.Requires != "" {
		t.Fatalf("approve needs no content and no Claim ID: %+v", a)
	}
	if move := action(t, w, "move", "human"); move.To != "" || !slices.Equal(move.Except, []store.State{store.NeedsAnswer, store.Failed}) {
		t.Fatalf("move goes anywhere except Needs Answer and Failed (entered only by a Round or a Failure Report): %+v", move)
	}
	// The agent's cancel and the human's are two rows.
	action(t, w, "cancel", "agent")
	action(t, w, "cancel", "human")
	// land is the way from Accepted to Done, for either Actor (ADR 0006).
	if land := action(t, w, "land", "human or agent"); !slices.Equal(land.From, []store.State{store.Accepted, store.InReview}) || land.To != store.Done || land.Requires == "" {
		t.Fatalf("land %+v", land)
	}
	// send_back has the human's row from Failed and the agent's from
	// Accepted, whose note is required (ticket 72).
	if sb := action(t, w, "send_back", "human"); !slices.Equal(sb.From, []store.State{store.Failed}) || sb.To != store.Ready {
		t.Fatalf("human send_back %+v", sb)
	}
	if sb := action(t, w, "send_back", "agent"); !slices.Equal(sb.From, []store.State{store.Accepted}) || sb.To != store.Ready || !strings.Contains(sb.Requires, "required") {
		t.Fatalf("agent send_back %+v", sb)
	}
}

func TestTheWorkflowSaysWhatCountsAsNeedsMe(t *testing.T) {
	w := workflow(t)
	want := []core.NeedsMeReason{
		core.ReasonNeedsAnswer, core.ReasonAnswered, core.ReasonSilentClaim, core.ReasonLongClaim, core.ReasonFailed,
		core.ReasonInReview, core.ReasonCIFailed, core.ReasonCICancelled, core.ReasonToMerge, core.ReasonCIUnavailable, core.ReasonAcceptedNotLanded, core.ReasonBlockedByCancelled, core.ReasonNewIssue, core.ReasonClosedExternally,
	}
	if len(w.NeedsMe) != len(want) {
		t.Fatalf("Needs Me reasons %v, want the fourteen in group order", w.NeedsMe)
	}
	for i, r := range w.NeedsMe {
		if r.Reason != want[i] || r.Meaning == "" {
			t.Fatalf("needs_me[%d] = %+v, want %s with its meaning", i, r, want[i])
		}
	}
}

func TestTheProseCoversTheRulesTheTableCannotSay(t *testing.T) {
	prose := workflow(t).Prose
	for _, want := range []string{
		"When you can't carry on", "`post_round`", "`report_failure`", "`give_up`",
		"A send-back with no note means the cause in the last Failure Report still stands",
		"branch named after the Ticket ID", "commit often", "Don't push",
		"Holding several Claims", "Silence is tracked per session", "same resume command", "same files",
		"`claim` returns these", "`get_ticket` with `parts` returns what `left_out` counts", "`rounds` on a redo",
		"`parts: [\"body\"]`", "`sections`", "`outline`", "add `tickets` only when sibling Tickets matter",
		"returns an acknowledgement (`id`, `updated`, `changed`, …)", "`full: true` returns the record",
		"Under a Project's Branch Landing Method, an Accepted Ticket shows the human `to_merge` at once",
	} {
		if !strings.Contains(prose, want) {
			t.Errorf("get_workflow prose does not say %q", want)
		}
	}
}

func TestTheInstructionsPointAtTheStageGuideAndTheRoundRule(t *testing.T) {
	if strings.Contains(core.Instructions, "Call get_workflow before working a Ticket") {
		t.Error("MCP instructions still send every session to get_workflow")
	}
	for _, want := range []string{"Code Central holds", "`stage_guide`", "`get_workflow`", "triage", "landing", "post_round", "never printed in the terminal", "numbered question", "acknowledgement", "`full: true`"} {
		if !strings.Contains(core.Instructions, want) {
			t.Errorf("MCP instructions do not say %q", want)
		}
	}
}

func action(t *testing.T, w core.WorkflowView, name string, who core.Who) core.ActionView {
	t.Helper()
	for _, a := range w.Actions {
		if a.Name == name && a.Who == who {
			return a
		}
	}
	t.Fatalf("no action %s taken by %s in %+v", name, who, w.Actions)
	return core.ActionView{}
}

func workflow(t *testing.T) core.WorkflowView {
	t.Helper()
	w, err := newFixture(t).core.GetWorkflow(ctx)
	if err != nil {
		t.Fatalf("get_workflow: %v", err)
	}
	return w
}
