package factory_test

import (
	"context"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory"
	"github.com/tomoram/code-central/factory/harness"
)

// reportingHarness declares caps and reports Plan Limits and a review on
// every Turn, whatever it declares.
type reportingHarness struct{ caps harness.Capabilities }

func (h reportingHarness) Capabilities() harness.Capabilities { return h.caps }

func (reportingHarness) Name() string { return "reporting" }

func (reportingHarness) Discover(string, func(string, ...any)) harness.Accepts {
	return harness.Accepts{}
}

func (reportingHarness) VerifyArgs(string, func(string, ...any)) error { return nil }

func (reportingHarness) SkillPrompt(skill, args string) string { return skill + " " + args }

func (reportingHarness) ToolLoadingNote() string { return "" }

func (reportingHarness) MCPConfig() string { return "" }

func (reportingHarness) ResumeCommand(string, []string) string { return "" }

func (reportingHarness) Denials() harness.DenialRules { return nil }

func (reportingHarness) Run(_ context.Context, t harness.Turn) harness.Outcome {
	t.OnPlanLimits(api.PlanLimits{})
	return harness.Outcome{Kind: harness.Done, ReviewCalled: true}
}

func TestATurnReportsOnlyThePlanLimitsAndReviewItsHarnessDeclares(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		caps                     harness.Capabilities
		wantLimits, wantReviewed bool
	}{
		"declared":     {harness.Capabilities{PlanLimits: true, ReviewDetection: true}, true, true},
		"not declared": {harness.Capabilities{}, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := factory.NewTurnRunner(reportingHarness{tc.caps}, factory.Env{}, false)
			var gotLimits bool
			in := factory.TurnInput{Turn: harness.Turn{OnPlanLimits: func(api.PlanLimits) { gotLimits = true }}}

			out := runner.Run(context.Background(), in)

			if gotLimits != tc.wantLimits {
				t.Errorf("Plan Limits reached the Factory: %t, want %t", gotLimits, tc.wantLimits)
			}
			if out.ReviewCalled != tc.wantReviewed {
				t.Errorf("the outcome says the review was called: %t, want %t", out.ReviewCalled, tc.wantReviewed)
			}
		})
	}
}
