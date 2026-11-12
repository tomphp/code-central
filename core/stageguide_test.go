package core_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/store"
)

// Stage Guides (ticket 73).

func TestEachStageHasAGuide(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, stage := range []string{"grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "implementation"} {
		g, err := f.core.GetStageGuide(ctx, "acme", core.StageGuideInput{Stage: stage})
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
	f.project(t, "acme")
	for _, stage := range []string{"", "build", "stage:spec", "Spec", "map", "chart", "wayfinder:chart", "map-map", "research"} {
		_, err := f.core.GetStageGuide(ctx, "acme", core.StageGuideInput{Stage: stage})
		wantCode(t, err, core.Invalid, "stage")
	}
}

// guide is the named Stage's guide, failing the test if it can't be read.
func guide(t *testing.T, f fixture, stage string) string {
	t.Helper()
	f.core.CreateProject(ctx, core.CreateProjectInput{Key: "acme"}) // a second create is refused
	g, err := f.core.GetStageGuide(ctx, "acme", core.StageGuideInput{Stage: stage})
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
	for _, stage := range core.GuideNames {
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
	for _, stage := range core.GuideNames {
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
		// A redo reads the earlier Rounds, which claim's Ticket leaves out.
		"`parts: [\"rounds\"]`",
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
		// The finish summary ends with a recommendation, prose only.
		"**Implement** or **Split into Tickets**", "one-line reason", "size", "prose only",
		// A Requirement made from a Map has no grilling Stage: it works from
		// the Map instead.
		"no grilling Stage", "`map` Tag", "Decisions so far", "resolution comments they link",
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

// troubleSections are each Stage Guide's section on what to do when the
// agent can't carry on, by Stage: from its heading to the next section or
// the end.
func troubleSections(t *testing.T) map[string]string {
	t.Helper()
	const heading = "## When you can't carry on"
	f := newFixture(t)
	out := map[string]string{}
	for _, stage := range core.GuideNames {
		g := guide(t, f, stage)
		i := strings.Index(g, heading)
		if i < 0 {
			t.Fatalf("the %s guide has no %q section", stage, heading)
		}
		s := g[i:]
		if j := strings.Index(s[len(heading):], "\n## "); j >= 0 {
			s = s[:len(heading)+j+1]
		}
		out[stage] = s
	}
	return out
}

func TestEveryGuideSaysWhatToDoWhenTheAgentCantCarryOn(t *testing.T) {
	for stage, s := range troubleSections(t) {
		wantAll(t, stage, s,
			// The three exits, with report_failure's four fields and
			// give_up's note on where the work stands.
			"`post_round`",
			"`report_failure`", "`what_broke`", "`why`", "`what_to_fix`", "`where_work_stands`",
			"`give_up`", "where the work stands",
			"Never leave a claimed Ticket without one of these or `finish`",
			"`stale_claim`", "stop at once",
			"`claim` the Ticket again with your `session_id`", "tell the human",
			"A send-back with no note means the cause in the last Failure Report still stands",
		)
	}
}

func TestGetWorkflowsTroubleRulesAreTheStageGuidesText(t *testing.T) {
	prose := workflow(t).Prose
	for stage, s := range troubleSections(t) {
		if !strings.Contains(prose, s) {
			t.Errorf("get_workflow's prose doesn't carry the %s guide's trouble rules:\n%s", stage, s)
		}
	}
}

// Subagent Exploration (code-central-441, ADR 0034).

const subagentFragment = "ask an Explore subagent and keep its answer, not the files"

func (f fixture) setSubagentExploration(t *testing.T, on bool) core.ProjectView {
	t.Helper()
	w, err := f.core.SetSubagentExploration(human, "acme", core.SetSubagentExplorationInput{On: on, Full: true})
	if err != nil {
		t.Fatalf("set_subagent_exploration: %v", err)
	}
	return *w.Record
}

func TestAProjectStoredWithoutTheSettingHasSubagentExplorationOn(t *testing.T) {
	f := newFixture(t)
	if p := f.project(t, "acme"); !p.StageGuides.SubagentExploration {
		t.Fatalf("stage_guides = %+v, want Subagent Exploration on", p.StageGuides)
	}
	if g := guide(t, f, "implementation"); !strings.Contains(g, subagentFragment) {
		t.Fatalf("the implementation guide has no subagent line:\n%s", g)
	}
}

func TestSubagentExplorationOffLeavesNoLineAndNoGap(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if p := f.setSubagentExploration(t, false); p.StageGuides.SubagentExploration {
		t.Fatalf("stage_guides = %+v, want Subagent Exploration off", p.StageGuides)
	}
	g := guide(t, f, "implementation")
	if strings.Contains(g, subagentFragment) || strings.Contains(g, "subagent-exploration") {
		t.Fatalf("the guide still has the line or its placeholder:\n%s", g)
	}
	if !strings.Contains(g, "- Work test-first at the seams the Spec's Testing Decisions name.\n- Run the full suite") {
		t.Fatalf("the bullets around the line are not adjacent:\n%s", g)
	}
	f.setSubagentExploration(t, true)
	if g := guide(t, f, "implementation"); !strings.Contains(g, "name.\n- "+"When understanding existing code") {
		t.Fatalf("on again, the line is not the same bullet:\n%s", g)
	}
}

func TestNoServedGuideHasAnUnreplacedPlaceholder(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, on := range []bool{true, false} {
		f.setSubagentExploration(t, on)
		for _, stage := range core.GuideNames {
			if g := guide(t, f, stage); strings.Contains(g, "<!--") {
				t.Errorf("the %s guide has a placeholder with Subagent Exploration %v", stage, on)
			}
		}
	}
}

func TestClaimBuildsTheGuideFromTheTicketsProject(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Search")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Index", State: store.Ready})
	f.setSubagentExploration(t, false)
	res, err := f.core.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}
	if res.StageGuide == nil || strings.Contains(res.StageGuide.Guide, subagentFragment) || res.StageGuide.Guide != guide(t, f, "implementation") {
		t.Fatalf("claim carried %+v, want the guide with Subagent Exploration off", res.StageGuide)
	}
}

func TestSetSubagentExplorationSendsTheProjectsNoticeOnlyOnAChange(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	w, err := f.core.SetSubagentExploration(human, "acme", core.SetSubagentExplorationInput{On: false})
	if err != nil {
		t.Fatal(err)
	}
	if w.Record != nil || w.ID != "acme" || !slices.Equal(w.Changed, []string{"stage_guides"}) {
		t.Errorf("acknowledgement = %+v, want stage_guides changed on acme", w)
	}
	if got := <-notices; got != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
		t.Fatalf("got %+v, want the Project's notice", got)
	}
	noNotice(t, notices)
	if _, err := f.core.SetSubagentExploration(human, "acme", core.SetSubagentExplorationInput{On: false}); err != nil {
		t.Fatal(err)
	}
	noNotice(t, notices)
}

func TestAnAgentCannotSetSubagentExploration(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.SetSubagentExploration(ctx, "acme", core.SetSubagentExplorationInput{On: false})
	wantCode(t, err, core.ActionNotAllowed, "")
}

func TestGetStageGuideOnAnUnknownProjectIsRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.core.GetStageGuide(ctx, "nope", core.StageGuideInput{Stage: "tickets"})
	wantCode(t, err, core.UnknownProject, "")
}

// Model Assignment (code-central-471, ADR 0034).
func TestAProjectStoredWithoutTheSettingHasModelAssignmentOff(t *testing.T) {
	f := newFixture(t)
	if p := f.project(t, "acme"); p.StageGuides.ModelAssignment {
		t.Fatalf("stage_guides = %+v, want Model Assignment off", p.StageGuides)
	}
}

func TestSetModelAssignmentTurnsItOnAndSendsTheNoticeOnlyOnAChange(t *testing.T) {
	f := newFixture(t)
	if p := f.project(t, "acme"); p.StageGuides.ModelAssignment {
		t.Fatalf("stage_guides = %+v, want Model Assignment off", p.StageGuides)
	}
	notices, stop := f.core.Events().Subscribe()
	defer stop()

	w, err := f.core.SetModelAssignment(human, "acme", core.SetModelAssignmentInput{On: true})
	if err != nil {
		t.Fatal(err)
	}
	if w.Record != nil || w.ID != "acme" || !slices.Equal(w.Changed, []string{"stage_guides"}) {
		t.Errorf("acknowledgement = %+v, want stage_guides changed on acme", w)
	}
	if got := <-notices; got != (events.Event{Type: events.ProjectChanged, Project: "acme"}) {
		t.Fatalf("got %+v, want the Project's notice", got)
	}
	again, err := f.core.SetModelAssignment(human, "acme", core.SetModelAssignmentInput{On: true, Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if sg := again.Record.StageGuides; !sg.ModelAssignment || !sg.SubagentExploration {
		t.Fatalf("stage_guides = %+v, want Model Assignment on and Subagent Exploration untouched", sg)
	}
	noNotice(t, notices)
}

func TestAnAgentCannotSetModelAssignment(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	_, err := f.core.SetModelAssignment(ctx, "acme", core.SetModelAssignmentInput{On: true})
	wantCode(t, err, core.ActionNotAllowed, "")
}

// The Model Assignment fragment (code-central-472, ADR 0034).
func (f fixture) setModelAssignment(t *testing.T, on bool) {
	t.Helper()
	if _, err := f.core.SetModelAssignment(human, "acme", core.SetModelAssignmentInput{On: on}); err != nil {
		t.Fatal(err)
	}
}

func TestModelAssignmentOffLeavesNoModelTextNoGapAndNoPlaceholder(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	g := guide(t, f, "tickets")
	for _, s := range []string{"model-assignment", "effort", "set_tags", "<!--", "- \n", "\n\n\n"} {
		if strings.Contains(g, s) {
			t.Errorf("the tickets guide has %q with Model Assignment off:\n%s", s, g)
		}
	}
	if !strings.Contains(g, "Give no Stage Label.\n\nIf the Spec records") {
		t.Errorf("the bullets are not followed by the paragraph:\n%s", g)
	}
}

func TestModelAssignmentOnListsTheReportedAliasesAndLevels(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.reportModels(t, core.ReportModelsInput{Aliases: []string{"opus", "sonnet"}, EffortLevels: []string{"low", "high"}})
	f.setModelAssignment(t, true)
	g := guide(t, f, "tickets")
	for _, s := range []string{"`opus`, `sonnet`", "`low`, `high`", "`create_ticket`", "`tags`", "reason", "`set_tags`", "Give no Stage Label.\n- "} {
		if !strings.Contains(g, s) {
			t.Errorf("the tickets guide lacks %q:\n%s", s, g)
		}
	}
	if strings.Contains(g, "medium") || strings.Contains(g, "haiku") {
		t.Errorf("the guide lists more than the reported values:\n%s", g)
	}
}

func TestModelAssignmentOnWithNoAliasesAsksForEffortOnlyWithSeededLevels(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.setModelAssignment(t, true)
	g := guide(t, f, "tickets")
	if !strings.Contains(g, "`low`, `medium`, `high`, `xhigh`, `max`") || !strings.Contains(g, "`set_tags`") {
		t.Errorf("the guide lacks the seeded levels or set_tags:\n%s", g)
	}
	if strings.Contains(g, "`model`") || strings.Contains(g, "alias") {
		t.Errorf("the guide mentions the model with no aliases:\n%s", g)
	}
}

func TestOnlyTheTicketsGuideChangesWithModelAssignment(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	before := map[string]string{}
	for _, stage := range core.GuideNames {
		before[stage] = guide(t, f, stage)
	}
	f.setModelAssignment(t, true)
	for _, stage := range core.GuideNames {
		g := guide(t, f, stage)
		if (g != before[stage]) != (stage == "tickets") {
			t.Errorf("the %s guide changed = %v with Model Assignment on", stage, g != before[stage])
		}
		if strings.Contains(g, "<!--") || strings.Contains(g, "{{") {
			t.Errorf("the %s guide has unrendered text", stage)
		}
	}
}
