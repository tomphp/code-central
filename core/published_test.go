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
}

func TestTheWorkflowSaysWhatCountsAsNeedsMe(t *testing.T) {
	w := workflow(t)
	want := []core.NeedsMeReason{
		core.ReasonNeedsAnswer, core.ReasonAnswered, core.ReasonSilentClaim, core.ReasonFailed,
		core.ReasonInReview, core.ReasonBlockedByCancelled, core.ReasonClosedExternally,
	}
	if len(w.NeedsMe) != len(want) {
		t.Fatalf("Needs Me reasons %v, want the spec's seven in group order", w.NeedsMe)
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
		"Three exits on trouble", "`post_round`", "`report_failure`", "`give_up`",
		"A send-back with no note means nothing has changed",
		"branch named after the Ticket ID", "commit and push often",
		"Holding several Claims", "Silence is tracked per session", "same resume command", "same files",
	} {
		if !strings.Contains(prose, want) {
			t.Errorf("get_workflow prose does not say %q", want)
		}
	}
}

func TestTheInstructionsPointAtGetWorkflowAndTheRoundRule(t *testing.T) {
	for _, want := range []string{"Code Central holds", "get_workflow", "post_round", "never printed in the terminal", "numbered question"} {
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
