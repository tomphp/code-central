package factory_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// line is the Factory's stdout line starting with prefix, failing the
// test when there is none.
func line(t *testing.T, r *factorytest.Running, prefix string) string {
	t.Helper()
	for _, l := range strings.Split(r.Stdout(), "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("no line starting %q in:\n%s", prefix, r.Stdout())
	return ""
}

func TestTheClaimedLineNamesTheTicketStageTitleSpecWorktreeAndResumeCommand(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	impl := h.ReadyTicket("Paint the shed")
	grill := h.StageTicket("grilling", "Shed colours")
	r := h.Start()
	h.WaitForState(impl, store.InReview)
	h.WaitForState(grill, store.InReview)
	r.WaitForOutput(impl + " turn ended")
	r.WaitForOutput(grill + " turn ended")

	for _, c := range []struct{ id, stage, title string }{{impl, "implementation", "Paint the shed"}, {grill, "grilling", "Shed colours"}} {
		call := callsFor(h, c.id)[0]
		want := fmt.Sprintf(`%s claimed (%s): "%s" in Spec %s; worktree %s; resume: claude --resume %s`,
			c.id, c.stage, c.title, h.Ticket(c.id).Spec.ID, h.Worktree(c.id), call.Session)
		if got := line(t, r, c.id+" claimed"); got != want {
			t.Errorf("claimed line\n got %s\nwant %s", got, want)
		}
		out := r.Stdout()
		if strings.Index(out, c.id+" claimed") > strings.Index(out, c.id+" turn ended") {
			t.Errorf("%s's claimed line came after its turn ended", c.id)
		}
	}
}

func TestTheTurnEndedLineGivesTheStageTitleExitStateAndDuration(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	asks := h.ReadyTicket("Ask first")
	done := h.StageTicket("spec", "Write it up")
	h.Script(asks, factorytest.PostRound("Which colour?"), factorytest.Result("success"))
	r := h.Start()
	h.WaitForState(asks, store.NeedsAnswer)
	h.WaitForState(done, store.InReview)
	r.WaitForOutput(asks + " turn ended")
	r.WaitForOutput(done + " turn ended")

	for _, c := range []struct{ id, want string }{
		{asks, asks + ` turn ended (implementation): "Ask first": exit 0, success; now Needs Answer; ran `},
		{done, done + ` turn ended (spec): "Write it up": exit 0, success; now In Review; ran `},
	} {
		got := line(t, r, c.id+" turn ended")
		ran, ok := strings.CutPrefix(got, c.want)
		if !ok || !regexp.MustCompile(`^(\d+m)?\d+s$`).MatchString(ran) {
			t.Errorf("turn ended line\n got %s\nwant %s<duration>", got, c.want)
		}
	}
}

func TestTheTurnEndedLineNamesTheStateTheFactoryLeftTheTicketIn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Stops early")
	h.Script(id, factorytest.Say("I give up."), factorytest.Result("success"))
	r := h.Start()
	h.WaitForState(id, store.Failed)
	r.WaitForOutput(id + " turn ended")
	if got := line(t, r, id+" turn ended"); !strings.Contains(got, ": exit 0, success; now Failed; ran ") {
		t.Errorf("turn ended line %q: want it to say the Factory's Failure Report left it Failed", got)
	}
}

func TestTheResumedLineNamesTheRoundBeingAnswered(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _, r := answeredTicket(t, h)
	r.Poll()
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + " resumed")
	want := id + ` resumed (implementation): "Ask first" with the answers to Round 1`
	if got := line(t, r, id+" resumed"); got != want {
		t.Errorf("resumed line\n got %s\nwant %s", got, want)
	}
}

func TestTheResumedLineForACutOffClaimSaysSoAfterARestart(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("tickets", "Cut it up")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WaitFor(id+"'s turn", func() bool { return len(h.Calls()) == 1 })
	r.Signal(os.Interrupt)
	r.Signal(os.Interrupt)
	r.Exited()

	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	r = h.Start()
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + " turn ended")
	want := id + ` resumed (tickets): "Cut it up" after being cut off`
	if got := line(t, r, id+" resumed"); got != want {
		t.Errorf("resumed line\n got %s\nwant %s", got, want)
	}
	if got := line(t, r, id+" turn ended"); !strings.HasPrefix(got, id+` turn ended (tickets): "Cut it up": exit 0, success; now In Review; ran `) {
		t.Errorf("turn ended line %q: want the Stage and title kept across the restart", got)
	}
}

// toolCalls is a turn calling the common tools, and a tool the summary
// knows nothing of, then finishing.
func toolCalls(wt string) []factorytest.Step {
	long := strings.Repeat("x", 200)
	return []factorytest.Step{
		factorytest.ToolUse("Bash", `{"command":"go test ./factory/...\necho done","description":"Run the tests"}`),
		factorytest.ToolUse("Read", `{"file_path":`+inWorktree(wt, "factory/run.go")+`}`),
		factorytest.ToolUse("Edit", `{"file_path":"/etc/hosts","old_string":"a","new_string":"b"}`),
		factorytest.ToolUse("Write", `{"file_path":`+inWorktree(wt, "notes.md")+`,"content":"hi"}`),
		factorytest.ToolUse("NotebookEdit", `{"notebook_path":`+inWorktree(wt, "a.ipynb")+`,"new_source":"x"}`),
		factorytest.ToolUse("Grep", `{"pattern":"func main","path":`+inWorktree(wt, "cmd")+`}`),
		factorytest.ToolUse("Grep", `{"pattern":"TODO"}`),
		factorytest.ToolUse("Glob", `{"pattern":"**/*.go"}`),
		factorytest.ToolUse("WebFetch", `{"url":"https://example.com/a","prompt":"Read it"}`),
		factorytest.ToolUse("WebSearch", `{"query":"go flag package"}`),
		factorytest.ToolUse("Agent", `{"description":"Find the callers","prompt":"Look for them"}`),
		factorytest.ToolUse("Task", `{"description":"Old name","prompt":"p"}`),
		factorytest.ToolUse("Skill", `{"skill":"tdd","args":"x"}`),
		factorytest.ToolUse("mcp__code-central__finish", `{"ticket":"{{ticket}}","claim_id":"{{claim}}","summary":"s"}`),
		factorytest.ToolUse("mcp__code-central__get_workflow", `{}`),
		factorytest.ToolUse("Frobnicate", `{"count":3,"target":"the\n  widget","other":"no"}`),
		factorytest.ToolUse("TodoWrite", `{"todos":[]}`),
		factorytest.ToolUse("Bash", `{"command":"echo `+long+`"}`),
		factorytest.Finish("Done."),
		factorytest.Result("success"),
	}
}

// inWorktree is the JSON string of the path rel (slash-separated) in the
// worktree wt, as a session on this OS would give it.
func inWorktree(wt, rel string) string {
	b, _ := json.Marshal(filepath.Join(wt, filepath.FromSlash(rel)))
	return string(b)
}

func TestVerbosePrintsOneLinePerToolCallWithItsSummary(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Busy")
	wt := h.Worktree(id)
	h.Script(id, toolCalls(wt)...)
	r := h.Start("--verbose")
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + " turn ended")

	var got []string
	for _, l := range strings.Split(r.Stdout(), "\n") {
		if rest, ok := strings.CutPrefix(l, id+": "); ok {
			got = append(got, rest)
		}
	}
	want := []string{
		"Bash: go test ./factory/...",
		"Read: " + filepath.FromSlash("factory/run.go"),
		"Edit: /etc/hosts",
		"Write: notes.md",
		"NotebookEdit: a.ipynb",
		"Grep: func main in cmd",
		"Grep: TODO",
		"Glob: **/*.go",
		"WebFetch: https://example.com/a",
		"WebSearch: go flag package",
		"Agent: Find the callers",
		"Task: Old name",
		"Skill: tdd",
		"mcp__code-central__finish: code-central finish " + id,
		"mcp__code-central__get_workflow: code-central get_workflow",
		"Frobnicate: the widget",
		"TodoWrite",
		"Bash: echo " + strings.Repeat("x", 120-len("echo ")-1) + "…",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("tool-call lines:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := line(t, r, id+" claimed"); !strings.HasPrefix(got, id+` claimed (implementation): "Busy" in Spec `) {
		t.Errorf("claimed line under --verbose: %s", got)
	}
	if got := line(t, r, id+" turn ended"); !strings.HasPrefix(got, id+` turn ended (implementation): "Busy": exit 0, success; now In Review; ran `) {
		t.Errorf("turn ended line under --verbose: %s", got)
	}
}

func TestWithoutVerboseNoToolCallLinesArePrinted(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Busy")
	h.Script(id, toolCalls(h.Worktree(id))...)
	r := h.Start()
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + " turn ended")
	if out := r.Stdout(); strings.Contains(out, id+": Bash") || strings.Contains(out, "go test") {
		t.Errorf("tool-call lines printed without --verbose:\n%s", out)
	}
}
