package factory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestATurnThatExitsWithItsClaimInProgressBecomesAFailureReport(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Too big")
	h.Script(id,
		factorytest.WriteFile("half.txt", "half\n"),
		factorytest.Git("add", "half.txt"),
		factorytest.Git("commit", "-q", "-m", "Half a feature"),
		factorytest.Say("I ran out of turns halfway through."),
		factorytest.Step{Stdout: `{"type":"result","subtype":"error_max_turns","is_error":true,"terminal_reason":"max_turns"}`},
		factorytest.Exit(3))
	r := h.Start()
	r.WaitForOutput(id + ` turn ended (implementation): "Too big": exit 3, error_max_turns (max_turns); now Failed; ran `)

	tk := h.WaitForState(id, store.Failed)
	if len(tk.FailureReports) != 1 {
		t.Fatalf("%d Failure Reports, want 1", len(tk.FailureReports))
	}
	fr := tk.FailureReports[0]
	all := strings.Join([]string{fr.WhatBroke, fr.Why, fr.WhatToFix, fr.WhereWorkStands}, "\n")
	commit := h.GitIn(h.Worktree(id), "rev-parse", "--short", "HEAD")
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
	h := factorytest.New(t)
	id := h.ReadyTicket("Needs npm")
	h.Script(id, factorytest.ResultDenied(
		factorytest.Denied("npm install"),
		factorytest.Denied("rm -rf build"),
		factorytest.Denied("npm install")))
	r := h.Start()

	tk := h.WaitForState(id, store.NeedsAnswer)
	r.WaitForOutput(id + " turn ended")
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
	h := factorytest.New(t)
	id := h.ReadyTicket("Needs npm")
	h.Script(id, factorytest.ResultDenied(
		factorytest.Denied("npm install"),
		factorytest.Denied("rm -rf ~/.cache"),
		factorytest.Denied("curl https://example.com")))
	r := h.Start()
	tk := h.WaitForState(id, store.NeedsAnswer)
	r.WaitForOutput(id + " turn ended")

	h.Script(id, factorytest.Finish("Installed."), factorytest.Result("success"))
	for i, answer := range []string{"allow", "deny it wipes my cache", "Allow Bash(curl:*)"} {
		q := tk.Rounds[0].Questions[i]
		if _, err := h.Human().Answer(context.Background(), h.Project, core.AnswerInput{Ticket: core.Ref(id), Question: q.Number, Text: answer}); err != nil {
			t.Fatal(err)
		}
	}
	r.Poll()
	h.WaitForState(id, store.InReview)

	resume := h.Calls()[1]
	if got, _ := resume.Flag("--allowedTools"); got != "Bash(npm install),Bash(curl:*)" {
		t.Errorf("--allowedTools %q, want the allowed call and the allowed pattern", got)
	}
	if !strings.Contains(resume.Prompt, "it wipes my cache") {
		t.Errorf("prompt %q: want the reason for the denial", resume.Prompt)
	}
	if got := stateAllowedTools(t, h, resume.Claim); !slices.Equal(got, []string{"Bash(npm install)", "Bash(curl:*)"}) {
		t.Errorf("the state file allows %q for the Claim", got)
	}

	other := h.ReadyTicket("Other work")
	r.Poll()
	h.WaitForState(other, store.InReview)
	if c := callsFor(h, other)[0]; c.Has("--allowedTools") {
		t.Errorf("another Claim's run got --allowedTools: %q", c.Args)
	}
}

func TestDenialsOnATurnThatFinishedAreListedInAComment(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Mostly done")
	h.Script(id, factorytest.Finish("Done, but I couldn't run the linter."), factorytest.ResultDenied(
		factorytest.Denied("golangci-lint run"),
		factorytest.Denied("golangci-lint run")))
	r := h.Start()
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + " turn ended")

	var tk core.TicketView
	var body string
	h.WaitFor("a comment listing the denials", func() bool {
		tk = h.Ticket(id)
		for _, c := range tk.Comments {
			if strings.Contains(c.Body, "denied") {
				body = c.Body
				return true
			}
		}
		return false
	})
	if strings.Count(body, "Bash(golangci-lint run)") != 1 {
		t.Errorf("comment %q: want the denied call listed once", body)
	}
	if len(tk.Rounds) != 0 {
		t.Errorf("a finished turn's denials got a Round: %+v", tk.Rounds)
	}
}

func TestAnOutagePausesClaimingAndResumingAndSaysWhatItIsWaitingOn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Backoff = time.Hour
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WaitForOutput(cut + " turn ended")

	next := h.ReadyTicket("Next")
	r.Poll()
	if s := h.Ticket(next).State; s != store.Ready || len(h.Calls()) != 1 {
		t.Errorf("%s is %s after %d runs; want nothing claimed or resumed during an outage", next, s, len(h.Calls()))
	}
	tk := h.Ticket(cut)
	if tk.State != store.InProgress || len(tk.FailureReports) != 0 {
		t.Errorf("%s is %s with %d Failure Reports; a cut-off turn is never failed", cut, tk.State, len(tk.FailureReports))
	}
	r.WaitForOutput("waiting on rate_limit")
}

func TestACutOffTurnIsResumedWithBackoffUntilTheOutageIsOver(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("overloaded_error"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	turnsEnded := func(n int) {
		t.Helper()
		h.WaitFor(fmt.Sprintf("%d turns to end", n), func() bool { return strings.Count(r.Stdout(), cut+" turn ended") == n })
	}
	turnsEnded(1)
	r.WaitForOutput("in 1ms")
	next := h.ReadyTicket("Next")

	// Still overloaded: the backoff doubles.
	r.Poll()
	turnsEnded(2)
	r.WaitForOutput("in 2ms")

	h.Script(cut, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()
	h.WaitFor("the second resume", func() bool { return len(h.Calls()) == 3 })
	first := h.Calls()[0]
	for _, c := range h.Calls()[1:] {
		if s, _ := c.Flag("--resume"); s != first.Session || c.Claim != first.Claim {
			t.Errorf("args %q: want the session %s resumed under its Claim", c.Args, first.Session)
		}
		if !strings.Contains(c.Prompt, "Your last turn was cut off; carry on.") {
			t.Errorf("prompt %q: want it told its last turn was cut off", c.Prompt)
		}
	}
	if s := h.Ticket(next).State; s != store.Ready {
		t.Errorf("%s is %s; nothing is claimed until a turn gets through", next, s)
	}

	h.Continue(cut)
	tk := h.WaitForState(cut, store.InReview)
	turnsEnded(3)
	r.WaitForOutput("the outage is over")
	r.Poll()
	h.WaitForState(next, store.InReview)
	if len(tk.FailureReports) != 0 {
		t.Errorf("a cut-off turn is never failed: %+v", tk.FailureReports)
	}
}

func TestAnExpiredLoginStopsTheFactory(t *testing.T) {
	t.Parallel()
	unixStopSequence(t)
	h := factorytest.New(t)
	busy := h.ReadyTicket("Busy")
	h.Script(busy, factorytest.Wait())
	r := h.Start("--parallel", "2")
	h.WaitFor(busy+"'s turn", func() bool { return len(h.Calls()) == 1 })
	expired := h.ReadyTicket("Expired")
	h.Script(expired, factorytest.APIError("authentication_failed"), factorytest.Result("success"), factorytest.Exit(1))
	r.Poll()

	if code := r.Exited(); code == 0 {
		t.Errorf("exit code %d; want the Factory to stop with an error", code)
	}
	if !strings.Contains(r.Stderr(), "authentication_failed") || !strings.Contains(r.Stderr(), "log in") {
		t.Errorf("stderr %q: want it to say the login expired", r.Stderr())
	}
	if got := h.Signals(busy); !slices.Equal(got, []string{"INT", "TERM"}) {
		t.Errorf("the other turn got %q, want it stopped", got)
	}
	for _, id := range []string{busy, expired} {
		if tk := h.Ticket(id); tk.State != store.InProgress || len(tk.FailureReports) != 0 {
			t.Errorf("%s is %s with %d Failure Reports; want its Claim kept", id, tk.State, len(tk.FailureReports))
		}
	}
	if got := stateTickets(t, h); len(got) != 2 {
		t.Errorf("the state file holds %q; want both Claims kept", got)
	}
}

// stateAllowedTools is the allowed tools the Factory's state file holds
// for claimID.
func stateAllowedTools(t *testing.T, h *factorytest.Harness, claimID string) []string {
	t.Helper()
	b, err := os.ReadFile(h.StateFile())
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
