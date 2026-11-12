package core_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/mcp"
	"github.com/tomoram/code-central/store"
)

// The context budget for what agents load. These are ceilings, not
// targets: each is set just above the size after Spec 187's cuts, to hold
// them, and text an agent pays for in every session must stay small. To
// check a change against the baseline, run the Context Report
// (docs/context-report.md).

// The ceilings in bytes, which the Context Report measures in.
const (
	// workflowBudget is the whole get_workflow response.
	workflowBudget = 12550
	// ticketBudget is the default get_ticket of redone's acme-2.
	ticketBudget = 1100
	// acknowledgementBudget is each write's default result.
	acknowledgementBudget = 300
)

// guideBudgets are each Stage Guide's ceiling in bytes, trouble rules
// included, by Stage.
var guideBudgets = map[string]int{
	"grilling":       4200,
	"spec":           3450,
	"tickets":        3500,
	"implementation": 3300,
	"map-chart":      4000,
	"map-grilling":   4000,
	"map-research":   3300,
	"map-task":       3000,
	"prototype":      3300,
}

func TestTheMCPInstructionsStayWithinAbout150Words(t *testing.T) {
	if n := len(strings.Fields(core.Instructions)); n > 150 {
		t.Fatalf("MCP instructions are %d words, over the 150 budget", n)
	}
}

func TestEveryToolDescriptionIsOneLine(t *testing.T) {
	for name, d := range mcp.Descriptions() {
		if strings.TrimSpace(d) == "" || strings.ContainsAny(d, "\r\n") {
			t.Errorf("tool %s: description %q is not one line", name, d)
		}
	}
}

func TestTheWorkflowProseStaysWithinAbout1500Tokens(t *testing.T) {
	if n := tokens(workflow(t).Prose); n > 1500 {
		t.Fatalf("get_workflow prose is about %d tokens, over the 1,500 budget", n)
	}
}

func TestEveryNextStepIsOneSentence(t *testing.T) {
	steps := core.NextSteps()
	if len(steps) == 0 {
		t.Fatal("no action has a next step")
	}
	for name, s := range steps {
		if !oneSentence(s) {
			t.Errorf("%s: next step %q is not one sentence", name, s)
		}
	}
}

func TestTheBudgetChecksCatchOverlongText(t *testing.T) {
	if oneSentence("End your turn now. Then run code-central wait.") {
		t.Error("two sentences passed as one")
	}
	if !oneSentence("End your turn now, after running `code-central wait acme-2` in the background if you can.") {
		t.Error("one sentence with a command in it was rejected")
	}
	if tokens(strings.Repeat("word ", 1200)) <= 1500 {
		t.Error("1,200 words should be over 1,500 tokens")
	}
}

// tokens estimates a text's token count the usual rough ways (four
// characters a token, or three words to four tokens) and takes the larger.
func tokens(s string) int {
	return max(len(s)/4, len(strings.Fields(s))*4/3)
}

// sentenceBreak is the end of one sentence followed by the start of
// another.
var sentenceBreak = regexp.MustCompile(`[.!?]\s+\S`)

func oneSentence(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && !strings.ContainsAny(s, "\r\n") && !sentenceBreak.MatchString(s)
}

func TestTheStageGuidesTroubleRulesStayWithin800Bytes(t *testing.T) {
	for stage, s := range troubleSections(t) {
		if n := len(s); n > 800 {
			t.Errorf("the %s guide's trouble rules are %d bytes, over the 800 budget", stage, n)
		}
	}
}

func TestEachStageGuideStaysWithinItsBudget(t *testing.T) {
	f := newFixture(t)
	for _, stage := range core.GuideNames {
		limit, ok := guideBudgets[stage]
		if !ok {
			t.Errorf("the %s guide has no budget", stage)
			continue
		}
		if n := len(guide(t, f, stage)); n > limit {
			t.Errorf("the %s guide is %d bytes, over the %d budget", stage, n, limit)
		}
	}
}

func TestTheWorkflowResponseStaysWithinItsBudget(t *testing.T) {
	if n := len(marshal(t, workflow(t))); n > workflowBudget {
		t.Fatalf("the get_workflow response is %d bytes, over the %d budget", n, workflowBudget)
	}
}

func TestTheDefaultTicketReadStaysWithinItsBudget(t *testing.T) {
	f := newFixture(t)
	f.redone(t)
	v, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "acme-2"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(marshal(t, v)); n > ticketBudget {
		t.Fatalf("the default get_ticket of a Ticket with Rounds, Claims and comments is %d bytes, over the %d budget", n, ticketBudget)
	}
}

func TestEveryWritesAcknowledgementStaysWithinItsBudget(t *testing.T) {
	for name, b := range acknowledgements(t) {
		if n := len(b); n > acknowledgementBudget {
			t.Errorf("%s's acknowledgement is %d bytes, over the %d budget", name, n, acknowledgementBudget)
		}
	}
}

// acknowledgements are the default results of the writes an agent makes,
// as JSON, by operation.
func acknowledgements(t *testing.T) map[string][]byte {
	t.Helper()
	f := newFixture(t)
	f.work(t, store.Ready, store.Ready, store.Ready, store.Ready) // acme-2 to acme-5
	out := map[string][]byte{}
	write := func(name string, v any, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = marshal(t, v)
	}
	title, desc := "Login form", "The form, with its fields."
	s, err := f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Search", Body: "Find things.\n"}) // acme-6
	write("create_spec", s, err)
	s, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: &title, Body: ptr("New.\n")})
	write("edit_spec", s, err)
	v, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Logout", Description: desc}) // acme-7
	write("create_ticket", v, err)
	v, err = f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: &title, Description: &desc})
	write("edit_ticket", v, err)
	v, err = f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Still on it"})
	write("comment", v, err)
	v, err = f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "5", Blocker: "4"})
	write("add_blocker", v, err)
	v, err = f.core.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: "5", Blocker: "4"})
	write("remove_blocker", v, err)
	v, err = f.core.Cancel(human, "acme", core.CancelInput{Ticket: "7", Reason: "Not needed"})
	write("cancel", v, err)

	f.claim(t, core.ClaimInput{}) // claim-1 on acme-2
	v, err = f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "s1"})
	write("set_session", v, err)
	r, err := f.core.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []core.QuestionInput{qDB}})
	write("post_round", r, err)
	f.claim(t, core.ClaimInput{}) // claim-2 on acme-3
	fin, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "3", ClaimID: "claim-2", Summary: "Done on acme-3.", Branch: "acme-3", Commit: "abc123"})
	write("finish", fin, err)
	f.claim(t, core.ClaimInput{}) // claim-3 on acme-4
	stop, err := f.core.ReportFailure(ctx, "acme", report("4", "claim-3"))
	write("report_failure", stop, err)
	f.claim(t, core.ClaimInput{}) // claim-4 on acme-5
	stop, err = f.core.GiveUp(ctx, "acme", core.GiveUpInput{Ticket: "5", ClaimID: "claim-4", Note: "Branch acme-5 has the form"})
	write("give_up", stop, err)
	return out
}

// marshal is v's JSON, as a client receives it.
func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
