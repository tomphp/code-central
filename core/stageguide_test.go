package core_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Stage Guides (ticket 73).

func TestEachStageHasAGuide(t *testing.T) {
	f := newFixture(t)
	for _, stage := range []string{"grilling", "spec", "tickets", "implementation"} {
		g, err := f.core.GetStageGuide(ctx, core.StageGuideInput{Stage: stage})
		if err != nil {
			t.Fatalf("get_stage_guide %s: %v", stage, err)
		}
		if g.Stage != stage || g.Guide == "" {
			t.Fatalf("get_stage_guide %s: %+v", stage, g)
		}
	}
}

func TestAnUnknownStageHasNoGuide(t *testing.T) {
	f := newFixture(t)
	for _, stage := range []string{"", "build", "stage:spec", "Spec"} {
		_, err := f.core.GetStageGuide(ctx, core.StageGuideInput{Stage: stage})
		wantCode(t, err, core.Invalid, "stage")
	}
}

// guide is the named Stage's guide, failing the test if it can't be read.
func guide(t *testing.T, f fixture, stage string) string {
	t.Helper()
	g, err := f.core.GetStageGuide(ctx, core.StageGuideInput{Stage: stage})
	if err != nil {
		t.Fatalf("get_stage_guide %s: %v", stage, err)
	}
	return g.Guide
}

// Any Factory or agent, on any door, must be able to follow a guide, so
// it names Code Central operations only (ADR 0007): no MCP tool names, no
// slash commands, and no command lines or their flags.
var notAnOperationName = map[string]*regexp.Regexp{
	"an MCP tool name":    regexp.MustCompile(`mcp__`),
	"a slash command":     regexp.MustCompile("(^|[\\s`(\"'])/[a-z][a-z0-9:_-]*"),
	"a claude command":    regexp.MustCompile(`\bclaude\b`),
	"a command-line flag": regexp.MustCompile("(^|[\\s`(\"'])--?[a-zA-Z]"),
}

func TestStageGuidesNameOnlyCodeCentralOperations(t *testing.T) {
	f := newFixture(t)
	for _, stage := range core.GuideStages {
		g := guide(t, f, stage)
		for what, re := range notAnOperationName {
			if m := re.FindString(g); m != "" {
				t.Errorf("the %s guide contains %s: %q", stage, what, m)
			}
		}
	}
}

func TestTheOperationNamesRuleCatchesToolNamesSlashCommandsAndClaudeFlags(t *testing.T) {
	for _, text := range []string{
		"Call mcp__code-central__finish.",
		"Run /implement first.",
		"Start with `/grilling`.",
		"Resume with claude --resume abc.",
		"Pass `--allowedTools`.",
	} {
		caught := false
		for _, re := range notAnOperationName {
			caught = caught || re.MatchString(text)
		}
		if !caught {
			t.Errorf("the rule lets %q through", text)
		}
	}
	for _, text := range []string{"Call `finish` with `branch` and `commit`.", "Merge the base into the branch, then commit it."} {
		for what, re := range notAnOperationName {
			if re.MatchString(text) {
				t.Errorf("the rule takes %q for %s", text, what)
			}
		}
	}
}

// wantAll fails the test for each phrase the guide doesn't contain.
func wantAll(t *testing.T, stage, g string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		if !strings.Contains(g, p) {
			t.Errorf("the %s guide doesn't say %q", stage, p)
		}
	}
}

func TestEveryGuideSaysWhatItProducesHowToFinishAndHowToRedo(t *testing.T) {
	f := newFixture(t)
	for _, stage := range core.GuideStages {
		wantAll(t, stage, guide(t, f, stage),
			"## What this Ticket produces",
			"## Finishing",
			"`finish`",
			"## A redo",
			// A redo reads why the last Claim ended first, builds on it and
			// answers each point.
			"rejection", "Failure Report", "give-up note",
			"build on what the earlier Claims left",
			"answer each point",
			// The In Review gate replaces checking with the user.
			"In Review",
			"instead of checking with the user",
			"`post_round`",
		)
	}
}

func TestAStageNeverChangesRepositoryFiles(t *testing.T) {
	f := newFixture(t)
	for _, stage := range core.Stages {
		wantAll(t, stage, guide(t, f, stage),
			"never changes repository files",
			"pushes no branch",
		)
	}
}

func TestTheGrillingGuideRetitlesTheSpecAndPostsAResolution(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "grilling", guide(t, f, "grilling"),
		"`edit_spec`", "title",
		"resolution comment", "`comment`",
		"every decision", "new term", "open point", "ADR",
		"Leave the Spec's body as the Requirement's text",
		"already clear", "with no Round",
		"points at the resolution comment",
	)
}

func TestTheSpecGuideReplacesTheBodyInTheSpecTemplate(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "spec", guide(t, f, "spec"),
		"`edit_spec`",
		"Problem Statement", "Solution", "User Stories", "Implementation Decisions",
		"Testing Decisions", "Out of Scope", "Further Notes",
		"Create no Spec", "add no Label",
		"grilling Ticket's Rounds and resolution",
		// It reads the resolution comment, and a Round only when that points
		// to one, not the whole grilling Ticket.
		"resolution comment", "`get_ticket` and `parts`", "only when the comment points to one",
		"test seams", "key decisions", "out of scope",
	)
}

func TestTheTicketsGuideCreatesReadyTicketsBlockedByItself(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "tickets", guide(t, f, "tickets"),
		"`create_ticket`", "`Ready`", "blocked by this Ticket", "`add_blocker`",
		"numbered breakdown", "blocking edges",
		"`edit_ticket`", "`cancel`", "Replacements", "never adds a second batch",
		"writing them", // new terms and ADRs go to an implementation Ticket
		// It reads the sections it slices from, not the whole Spec.
		"`get_spec` and `sections`", "read the rest only when",
	)
}

func TestTheImplementationGuideCommitsOnTheTicketBranchAndFinishesWithIt(t *testing.T) {
	f := newFixture(t)
	wantAll(t, "implementation", guide(t, f, "implementation"),
		"test-first", "full suite", "review",
		"branch named after the Ticket's ID", "Don't push", "pull request",
		"`branch`", "`commit`",
		"acceptance criteria", "test result", "what to check",
		"merge conflict", "merge the base into the branch first",
	)
}

func TestClaimCarriesTheGuideForTheTicketsStage(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if _, err := f.core.SubmitRequirement(ctx, "acme", core.SubmitRequirementInput{Text: darkMode}); err != nil {
		t.Fatal(err)
	}
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{}) // the grilling Stage, acme-2
	if err != nil {
		t.Fatal(err)
	}
	if res.Ticket.ID != "acme-2" || res.StageGuide == nil || res.StageGuide.Stage != "grilling" ||
		res.StageGuide.Guide != guide(t, f, "grilling") {
		t.Fatalf("the grilling Stage's claim carried %+v", res.StageGuide)
	}
	// The tickets Stage (acme-4), unblocked by hand, gets its own guide.
	if _, err := f.core.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: "acme-4", Blocker: "acme-3"}); err != nil {
		t.Fatal(err)
	}
	res, err = f.core.Claim(ctx, "acme", core.ClaimInput{Ticket: "acme-4"})
	if err != nil {
		t.Fatal(err)
	}
	if res.StageGuide == nil || res.StageGuide.Stage != "tickets" || res.StageGuide.Guide != guide(t, f, "tickets") {
		t.Fatalf("the tickets Stage's claim carried %+v", res.StageGuide)
	}
}

func TestClaimCarriesTheImplementationGuideForATicketWithNoStage(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Search")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Index", State: store.Ready, Labels: []string{"backend"}})
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res.StageGuide == nil || res.StageGuide.Stage != "implementation" || res.StageGuide.Guide != guide(t, f, "implementation") {
		t.Fatalf("claim carried %+v", res.StageGuide)
	}
}

func TestAClaimOfNothingCarriesNoGuide(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil || res.Ticket != nil || res.StageGuide != nil {
		t.Fatalf("claim of nothing: %+v, %v", res, err)
	}
}
