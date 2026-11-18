package factory_test

import (
	"github.com/tomoram/code-central/domain"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// event is a Session Event without its time and seq, for comparing.
type event struct {
	Kind                  core.SessionEventKind
	Text, Tool, ToolUseID string
	IsError, Truncated    bool
}

func events(tr core.TranscriptView) []event {
	var out []event
	for _, e := range tr.Events {
		out = append(out, event{e.Kind, e.Text, e.Tool, e.ToolUseID, e.IsError, e.Truncated})
	}
	return out
}

// waitForTranscript polls until the Transcript of ticket's first Claim
// holds an event whose text contains s, and returns it. The Ticket has
// been claimed: a test settles the Factory first.
func waitForTranscript(h *factorytest.Harness, r *factorytest.Running, ticket, s string) core.TranscriptView {
	var tr core.TranscriptView
	r.PollUntil("an event with "+s+" in "+ticket+"'s Transcript", func() bool {
		tr = h.Transcript(ticket, 1)
		return slices.ContainsFunc(tr.Events, func(e core.HeldSessionEvent) bool { return strings.Contains(e.Text, s) })
	})
	return tr
}

func TestATurnsStreamJSONIsPostedAsSessionEvents(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Paint the shed")
	wt := h.Worktree(id)
	h.Script(id,
		factorytest.Think("Which brush first?"),
		factorytest.Step{Stdout: `{"type":"user","message":{"role":"user","content":"Ticket: a prompt from the Factory"}}`},
		factorytest.Step{Stdout: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Another prompt"}]}}`},
		factorytest.Say("I'll paint the shed.\n\nFirst, the tests."),
		factorytest.Say("  \n"),
		factorytest.ToolUse("Bash", `{"command":"go test ./...\necho done","description":"Run the tests"}`),
		factorytest.ToolResult("toolu_Bash", "ok", false),
		factorytest.ToolUse("Read", `{"file_path":`+inWorktree(wt, "shed/paint.go")+`}`),
		factorytest.ToolResult("toolu_Read", []map[string]string{{"type": "text", "text": "package shed"}, {"type": "text", "text": "func Paint() {}"}}, false),
		factorytest.ToolUse("Grep", `{"pattern":"TODO","path":`+inWorktree(wt, "shed")+`}`),
		factorytest.ToolResult("toolu_Grep", strings.Repeat("é", 3000), true),
		factorytest.ToolUse("mcp__code-central__get_ticket", `{"ticket":"`+id+`"}`),
		factorytest.ToolUse("Frobnicate", `{"level":3,"target":"the shed"}`),
		factorytest.Finish("Done."),
		factorytest.Result("success"),
	)
	r := h.Start("--verbose")
	h.WantState(id, domain.InReview)
	r.WantOutput(id + " turn ended")
	// The turn has ended, so everything it printed waits for a batch.
	r.Poll()
	tr := h.Transcript(id, 1)

	cut := strings.Repeat("é", 2048) // 4 KB of two-byte runes
	want := []event{
		{Kind: "factory", Text: line(t, r, id+" claimed")},
		{Kind: "text", Text: "I'll paint the shed.\n\nFirst, the tests."},
		{Kind: "tool_call", Text: "Bash: go test ./...", Tool: "Bash", ToolUseID: "toolu_Bash"},
		{Kind: "tool_result", Text: "ok", Tool: "Bash", ToolUseID: "toolu_Bash"},
		{Kind: "tool_call", Text: "Read: " + filepath.FromSlash("shed/paint.go"), Tool: "Read", ToolUseID: "toolu_Read"},
		{Kind: "tool_result", Text: "package shed\nfunc Paint() {}", Tool: "Read", ToolUseID: "toolu_Read"},
		{Kind: "tool_call", Text: "Grep: TODO in shed", Tool: "Grep", ToolUseID: "toolu_Grep"},
		{Kind: "tool_result", Text: cut, Tool: "Grep", ToolUseID: "toolu_Grep", IsError: true, Truncated: true},
		{Kind: "tool_call", Text: "mcp__code-central__get_ticket: code-central get_ticket " + id, Tool: "mcp__code-central__get_ticket", ToolUseID: "toolu_mcp__code-central__get_ticket"},
		{Kind: "tool_call", Text: "Frobnicate: the shed", Tool: "Frobnicate", ToolUseID: "toolu_Frobnicate"},
		{Kind: "factory", Text: line(t, r, id+" turn ended")},
	}
	if got := events(tr); !slices.Equal(got, want) {
		t.Errorf("Transcript\n got %+v\nwant %+v", got, want)
	}
	if tr.Tail {
		t.Error("the Transcript is a tail; want the whole session")
	}
	for i, e := range tr.Events {
		if e.Time.IsZero() || i > 0 && e.Time.Before(tr.Events[i-1].Time) {
			t.Errorf("event %d's time is %v after %v; want the time the Factory saw it, in order", i, e.Time, tr.Events[max(i-1, 0)].Time)
		}
	}
	// The --verbose line is printed, not posted again as a Factory line.
	if got := line(t, r, id+": Bash"); got != id+": Bash: go test ./..." {
		t.Errorf("verbose line: %s", got)
	}
}

func TestTheFactorysLinesAboutAClaimArePostedToItsTranscript(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Ask first")
	h.Script(id, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	r := h.Start()
	h.WantState(id, domain.NeedsAnswer)
	r.WantOutput(id + " turn ended")
	h.Answer(id, "Green")
	r.Poll()
	if n := strings.Count(r.Stdout(), id+" turn ended"); n != 2 {
		t.Fatalf("%d turn ended lines after the resume, want 2; stdout:\n%s", n, r.Stdout())
	}
	// The second turn ended line was printed after the poll's batches.
	r.Poll()
	tr := h.Transcript(id, 1)
	if len(tr.Events) != 4 {
		t.Fatalf("Transcript holds %+v, want the claimed, turn ended, resumed and turn ended lines", events(tr))
	}

	var want []event
	for _, l := range strings.Split(r.Stdout(), "\n") {
		if strings.HasPrefix(l, id+" ") {
			want = append(want, event{Kind: "factory", Text: l})
		}
	}
	if len(want) != 4 || !strings.Contains(want[2].Text, " resumed ") {
		t.Fatalf("want claimed, turn ended, resumed and turn ended lines; stdout:\n%s", r.Stdout())
	}
	if got := events(tr); !slices.Equal(got, want) {
		t.Errorf("Transcript\n got %+v\nwant %+v", got, want)
	}
	// A line with no Claim, such as base:, is only printed.
	line(t, r, "base: ")
}

func TestATurnThatKeepsPostingEventsNeverMakesItsClaimSilent(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id,
		factorytest.Wait(), factorytest.Say("Still painting."),
		factorytest.Wait(), factorytest.Say("Nearly there."),
		factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	waitForTranscript(h, r, id, "claimed")

	for _, said := range []string{"Still painting.", "Nearly there."} {
		h.Server.Clock.Advance(core.SilentAfter + time.Minute)
		if h.Ticket(id).SilentSince == nil {
			t.Fatalf("%s isn't Silent after %v quiet; the test proves nothing", id, core.SilentAfter+time.Minute)
		}
		h.Continue(id)
		waitForTranscript(h, r, id, said)
		if s := h.Ticket(id).SilentSince; s != nil {
			t.Errorf("%s is Silent since %v after posting %q", id, *s, said)
		}
	}
	h.Continue(id)
	r.Settle()
	h.WantState(id, domain.InReview)
}

func TestWithCodeCentralDownMidTurnTheTurnStillFinishesAndDroppedEventsHoldNothingUp(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Paint through an outage")
	h.Script(id,
		factorytest.Say("Before the outage."), factorytest.Wait(),
		factorytest.Say("During the outage."), factorytest.Wait(),
		factorytest.Say("After the outage."), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	waitForTranscript(h, r, id, "Before the outage.")

	h.SetDown(true)
	h.Continue(id)
	r.PollUntil("a post of Session Events to be refused", func() bool { return h.Refused("/api/session-events") > 0 })
	h.SetDown(false)
	h.Continue(id)
	r.Settle()
	h.WantState(id, domain.InReview)
	r.WantOutput(id + " turn ended")
	r.Poll()
	tr := h.Transcript(id, 1)

	var said []string
	for _, e := range tr.Events {
		if e.Kind == "text" {
			said = append(said, e.Text)
		}
	}
	// The batch posted while down is dropped, never retried.
	if want := []string{"Before the outage.", "After the outage."}; !slices.Equal(said, want) {
		t.Errorf("agent text in the Transcript is %q; want %q", said, want)
	}
	if strings.Contains(r.Stdout()+r.Stderr(), "session-events") {
		t.Errorf("a failed post was reported; want it dropped quietly:\n%s%s", r.Stdout(), r.Stderr())
	}
}
