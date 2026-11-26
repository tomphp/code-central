package factory_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

func TestATurnThatExitsWithItsClaimInProgressBecomesAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Too big")
	h.Script(id,
		factorytest.WriteFile("half.txt", "half\n"),
		factorytest.Git("add", "half.txt"),
		factorytest.Git("commit", "-q", "-m", "Half a feature"),
		factorytest.Say("I ran out of turns halfway through."),
		factorytest.Step{Stdout: `{"type":"result","subtype":"error_max_turns","is_error":true,"terminal_reason":"max_turns"}`},
		factorytest.Exit(3))
	r := h.Start()
	r.WantOutput(id + ` turn ended (implementation): "Too big": exit 3, error_max_turns (max_turns); now Failed; ran `)

	tk := h.WantState(id, domain.Failed)
	if len(tk.FailureReports) != 1 {
		t.Fatalf("%d Failure Reports, want 1", len(tk.FailureReports))
	}
	fr := tk.FailureReports[0]
	all := strings.Join([]string{fr.WhatBroke, fr.Why, fr.WhatToFix, fr.WhereWorkStands}, "\n")
	commit := h.HeadOf(h.Worktree(id))[:7]
	for _, want := range []string{"exit 3", "error_max_turns", "max_turns", "I ran out of turns halfway through.", "branch " + id, h.Worktree(id), commit, "Half a feature"} {
		if !strings.Contains(all, want) {
			t.Errorf("the Failure Report doesn't give %q:\n%s", want, all)
		}
	}
	if fr.SessionID != h.Calls()[0].Session {
		t.Errorf("the report is under session %q, want the turn's %s", fr.SessionID, h.Calls()[0].Session)
	}
}

func TestATurnThatEndsWithDenialsAndNoExitGetsARoundOfOneQuestionPerDistinctCall(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Needs npm")
	h.Script(id, factorytest.ResultDenied(
		factorytest.Denied("npm install"),
		factorytest.Denied("rm -rf build"),
		factorytest.Denied("npm install")))
	r := h.Start()

	tk := h.WantState(id, domain.NeedsAnswer)
	r.WantOutput(id + " turn ended")
	if len(tk.Rounds) != 1 {
		t.Fatalf("%d Rounds, want 1", len(tk.Rounds))
	}
	qs := tk.Rounds[0].Questions
	if len(qs) != 2 {
		t.Fatalf("%d Questions, want one per distinct denied call: %+v", len(qs), qs)
	}
	for i, cmd := range []string{"npm install", "rm -rf build"} {
		q := qs[i]
		if !strings.Contains(q.Title, cmd) || !strings.Contains(q.Body, "Bash("+cmd+")") {
			t.Errorf("Question %d = %q / %q; want it to name the call %q", i+1, q.Title, q.Body, cmd)
		}
		for _, answer := range []string{"allow", "allow <pattern>", "deny <why>"} {
			if !strings.Contains(q.Body, answer) {
				t.Errorf("Question %d's body doesn't offer %q: %q", i+1, answer, q.Body)
			}
		}
	}
	if len(tk.FailureReports) != 0 {
		t.Errorf("a turn stopped by denials isn't a failure: %+v", tk.FailureReports)
	}
}

func TestWhatTheHumanAllowsIsPassedAsAllowedToolsOnThatClaimsLaterRunsOnly(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Needs npm")
	h.Script(id, factorytest.ResultDenied(
		factorytest.Denied("npm install"),
		factorytest.Denied("rm -rf ~/.cache"),
		factorytest.Denied("curl https://example.com")))
	r := h.Start()
	tk := h.WantState(id, domain.NeedsAnswer)
	r.WantOutput(id + " turn ended")

	// The resume waits so the state file is read while its Claim is held:
	// the turn's ending wakes a poll that drops it.
	h.Script(id, factorytest.Wait(), factorytest.Finish("Installed."), factorytest.Result("success"))
	for i, answer := range []string{"allow", "deny it wipes my cache", "Allow Bash(curl:*)"} {
		q := tk.Rounds[0].Questions[i]
		if _, err := h.Human().Answer(context.Background(), h.Project, api.AnswerInput{Ticket: api.Ref(id), Question: q.Number, Text: answer}); err != nil {
			t.Fatal(err)
		}
	}
	r.Poll()

	resume := h.WantCalls(2)[1]
	if got, _ := resume.Flag("--allowedTools"); got != "Bash(npm install),Bash(curl:*)" {
		t.Errorf("--allowedTools %q, want the allowed call and the allowed pattern", got)
	}
	if !strings.Contains(resume.Prompt, "it wipes my cache") {
		t.Errorf("prompt %q: want the reason for the denial", resume.Prompt)
	}
	if got := stateAllowedTools(t, h, resume.Claim); !slices.Equal(got, []string{"Bash(npm install)", "Bash(curl:*)"}) {
		t.Errorf("the state file allows %q for the Claim", got)
	}
	h.Continue(id)
	r.Settle()
	h.WantState(id, domain.InReview)

	other := h.ReadyTicket("Other work")
	r.Poll()
	h.WantState(other, domain.InReview)
	if c := callsFor(h, other)[0]; c.Has("--allowedTools") {
		t.Errorf("another Claim's run got --allowedTools: %q", c.Args)
	}
}

func TestDenialsOnATurnThatFinishedAreListedInAComment(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Mostly done")
	h.Script(id, factorytest.Finish("Done, but I couldn't run the linter."), factorytest.ResultDenied(
		factorytest.Denied("golangci-lint run"),
		factorytest.Denied("golangci-lint run")))
	r := h.Start()
	tk := h.WantState(id, domain.InReview)
	r.WantOutput(id + " turn ended")

	var body string
	for _, c := range tk.Comments {
		if strings.Contains(c.Body, "denied") {
			body = c.Body
			break
		}
	}
	if body == "" {
		t.Fatalf("no comment lists the denials: %+v", tk.Comments)
	}
	if strings.Count(body, "Bash(golangci-lint run)") != 1 {
		t.Errorf("comment %q: want the denied call listed once", body)
	}
	if len(tk.Rounds) != 0 {
		t.Errorf("a finished turn's denials got a Round: %+v", tk.Rounds)
	}
}

// hookRefused scripts a call a PreToolUse hook refused, and the result
// listing it among the permission_denials, as claude does.
func hookRefused(command string) []factorytest.Step {
	d := factorytest.Denied(command)
	return []factorytest.Step{
		factorytest.ToolResult(d.ID, "PreToolUse:Bash hook error: Edit the file with the Edit tool", true),
	}
}

func TestAHookRefusalOnATurnThatStoppedIsStuckNotAnAllowRound(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Hook refused")
	h.Script(id, append(hookRefused("python3 replace.py"),
		factorytest.ResultDenied(factorytest.Denied("python3 replace.py")))...)
	r := h.Start()

	tk := h.WantState(id, domain.Failed)
	r.WantOutput(id + " turn ended")
	if len(tk.Rounds) != 0 {
		t.Errorf("a hook refusal got a Round: %+v", tk.Rounds)
	}
	if len(tk.FailureReports) != 1 {
		t.Errorf("%d Failure Reports, want the stuck path's 1", len(tk.FailureReports))
	}
}

func TestAHookRefusalOnATurnThatFinishedIsNotListedAsADenial(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Hook refused, then done")
	steps := hookRefused("python3 replace.py")
	steps = append(steps, factorytest.Finish("Done."), factorytest.ResultDenied(
		factorytest.Denied("python3 replace.py"),
		factorytest.Denied("golangci-lint run")))
	h.Script(id, steps...)
	r := h.Start()

	tk := h.WantState(id, domain.InReview)
	r.WantOutput(id + " turn ended")
	var body string
	for _, c := range tk.Comments {
		if strings.Contains(c.Body, "denied") {
			body = c.Body
		}
	}
	if !strings.Contains(body, "Bash(golangci-lint run)") {
		t.Fatalf("comments %+v: want one listing the real denial", tk.Comments)
	}
	if strings.Contains(body, "python3") {
		t.Errorf("comment %q lists the hook's refusal as a denial", body)
	}
}

func TestAnOutagePausesClaimingAndResumingAndSaysWhatItIsWaitingOn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	// Longer than a poll, which moves the clock by --poll.
	h.Backoff = time.Minute
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WantOutput(cut + " turn ended")

	next := h.ReadyTicket("Next")
	r.Poll()
	if s := h.Ticket(next).State; s != domain.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s after %d runs; want nothing claimed or resumed during an outage", next, s, len(h.Calls()))
	}
	tk := h.Ticket(cut)
	if tk.State != domain.InProgress || len(tk.FailureReports) != 0 {
		t.Errorf("%s is %s with %d Failure Reports; a cut-off turn is never failed", cut, tk.State, len(tk.FailureReports))
	}
	r.WantOutput("waiting on rate_limit")
}

func TestAPauseWithNothingLeftCutOffEndsAndClaimingCarriesOn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WantOutput("waiting on rate_limit")

	// Its Claim ends while cut off, so no resume will test the service.
	if _, err := h.Human().Cancel(context.Background(), h.Project, api.CancelInput{Ticket: api.Ref(cut), Reason: "Not needed."}); err != nil {
		t.Fatal(err)
	}
	next := h.ReadyTicket("Next")
	r.Poll()
	h.WantState(next, domain.InReview)
	r.WantOutput("nothing is cut off any more: carrying on")
}

func TestACutOffTurnIsResumedWithBackoffUntilTheOutageIsOver(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	// Two polls, each of which moves the clock by --poll, 30s.
	h.Backoff = time.Minute
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("overloaded_error"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	h.WantCalls(1)
	r.WantOutput(cut + " turn ended")
	r.WantOutput("in 1m0s")
	next := h.ReadyTicket("Next")

	// Not resumed until the backoff has passed.
	r.Poll()
	if n := len(h.Calls()); n != 1 {
		t.Errorf("%d runs before the backoff passed; want the cut-off turn left alone", n)
	}

	// Still overloaded: the backoff doubles.
	r.Poll()
	h.WantCalls(2)
	r.WantOutput("in 2m0s")

	h.Script(cut, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	for range 3 {
		r.Poll()
	}
	if n := len(h.Calls()); n != 2 {
		t.Errorf("%d runs before the doubled backoff passed; want 2", n)
	}
	r.Poll()
	calls := h.WantCalls(3)
	first := calls[0]
	for _, c := range calls[1:] {
		if s, _ := c.Flag("--resume"); s != first.Session || c.Claim != first.Claim {
			t.Errorf("args %q: want the session %s resumed under its Claim", c.Args, first.Session)
		}
		if !strings.Contains(c.Prompt, "Your last turn was cut off; carry on.") {
			t.Errorf("prompt %q: want it told its last turn was cut off", c.Prompt)
		}
	}
	if s := h.Ticket(next).State; s != domain.Ready {
		t.Errorf("%s is %s; nothing is claimed until a turn gets through", next, s)
	}

	h.Continue(cut)
	r.Settle()
	tk := h.WantState(cut, domain.InReview)
	r.WantOutput("the outage is over")
	r.Poll()
	h.WantState(next, domain.InReview)
	if len(tk.FailureReports) != 0 {
		t.Errorf("a cut-off turn is never failed: %+v", tk.FailureReports)
	}
}

func TestAnExpiredLoginStopsTheFactory(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	busy := h.ReadyTicket("Busy")
	h.Script(busy, factorytest.Wait())
	r := h.Start("--parallel", "2")
	h.WantCalls(1)
	expired := h.ReadyTicket("Expired")
	h.Script(expired, factorytest.APIError("authentication_failed"), factorytest.Result("success"), factorytest.Exit(1))
	r.Poll()

	if code := r.Exited(); code == 0 {
		t.Errorf("exit code %d; want the Factory to stop with an error", code)
	}
	if !strings.Contains(r.Stderr(), "authentication_failed") || !strings.Contains(r.Stderr(), "log in") {
		t.Errorf("stderr %q: want it to say the login expired", r.Stderr())
	}
	wantStopped(t, h, busy)
	for _, id := range []string{busy, expired} {
		if tk := h.Ticket(id); tk.State != domain.InProgress || len(tk.FailureReports) != 0 {
			t.Errorf("%s is %s with %d Failure Reports; want its Claim kept", id, tk.State, len(tk.FailureReports))
		}
	}
	if got := stateTickets(t, h); len(got) != 2 {
		t.Errorf("the state file holds %q; want both Claims kept", got)
	}
}

// stateAllowedTools is the allowed tools the Factory's state file holds
// for claimID.
func stateAllowedTools(t *testing.T, h *factorytest.Rig, claimID string) []string {
	t.Helper()
	b, err := h.ReadStateFile()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Claims []struct {
			ClaimID      string   `json:"claim_id"`
			AllowedTools []string `json:"allowed_tools"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	for _, c := range doc.Claims {
		if c.ClaimID == claimID {
			return c.AllowedTools
		}
	}
	t.Fatalf("the state file doesn't hold Claim %s: %s", claimID, b)
	return nil
}

func finishWithBranch(summary string) factorytest.Step {
	return factorytest.CLI("finish", "{{ticket}}", "--claim", "{{claim}}", "--summary", summary, "--branch", "{{ticket}}", "--commit", "{{head}}")
}

func reviewSkippedComment(tk api.TicketView) bool {
	for _, c := range tk.Comments {
		if strings.Contains(c.Body, "skipped the code-review skill") {
			return true
		}
	}
	return false
}

func commitFile(name string) []factorytest.Step {
	return []factorytest.Step{
		factorytest.WriteFile(name, "x\n"),
		factorytest.Git("add", name),
		factorytest.Git("commit", "-q", "-m", "Change "+name),
	}
}

func TestAnImplementationFinishWithCodeChangesAndNoCodeReviewGetsAComment(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("No review")
	h.Script(id, append(commitFile("main.go"), finishWithBranch("Done."))...)
	h.Start()
	tk := h.WantState(id, domain.InReview)
	if !reviewSkippedComment(tk) {
		t.Errorf("no comment says the review was skipped: %+v", tk.Comments)
	}
}

func TestAnImplementationFinishThatCalledCodeReviewGetsNoComment(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Reviewed")
	h.Script(id, append(append(commitFile("main.go"),
		factorytest.ToolUse("Skill", `{"skill":"mattpocock-skills:code-review"}`)),
		finishWithBranch("Done."))...)
	h.Start()
	tk := h.WantState(id, domain.InReview)
	if reviewSkippedComment(tk) {
		t.Errorf("a reviewed finish got a comment: %+v", tk.Comments)
	}
}

func TestADocsOnlyImplementationFinishWithNoCodeReviewGetsNoComment(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Docs")
	h.Script(id, append(commitFile("NOTES.md"), finishWithBranch("Done."))...)
	h.Start()
	tk := h.WantState(id, domain.InReview)
	if reviewSkippedComment(tk) {
		t.Errorf("a docs-only finish got a comment: %+v", tk.Comments)
	}
}
