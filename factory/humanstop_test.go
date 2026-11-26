package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
)

const stoppedPrompt = "A human stopped your last turn and has now resumed you; carry on."

func stop(t *testing.T, h *factorytest.Rig, id string) {
	t.Helper()
	if _, err := h.Human().Stop(context.Background(), h.Project, api.StopInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
}

func resume(t *testing.T, h *factorytest.Rig, id string) {
	t.Helper()
	if _, err := h.Human().Resume(context.Background(), h.Project, api.ResumeInput{Ticket: api.Ref(id)}); err != nil {
		t.Fatal(err)
	}
}

// wantHeld checks id's Claim is kept: In Progress or Answered, Stopped,
// in the state file and with no Failure Report.
func wantHeld(t *testing.T, h *factorytest.Rig, id string) {
	t.Helper()
	tk := h.Ticket(id)
	if len(tk.Claims) != 1 || !tk.Claims[0].Stopped || len(tk.FailureReports) != 0 {
		t.Errorf("%s is %s with Claims %+v and %d Failure Reports; want its one Claim kept, Stopped", id, tk.State, tk.Claims, len(tk.FailureReports))
	}
	if got := stateTickets(t, h); !slices.Contains(got, id) {
		t.Errorf("state file holds %q; want %s's Stopped Claim kept", got, id)
	}
}

func TestAStopStopsTheTurnOnAWakeKeepsTheClaimAndFreesItsSlot(t *testing.T) {
	t.Parallel()
	h := wakingRig(t)
	stopped := h.ReadyTicket("Stopped")
	other := h.ReadyTicket("Carries on")
	h.Script(stopped, factorytest.WriteFile("wip.txt", "wip\n"), factorytest.Wait())
	h.Script(other, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start("--parallel", "2")
	h.WantCalls(2)
	head := h.WorktreeHead(stopped)

	stop(t, h, stopped)
	r.PassKillGrace()
	r.Settle()
	next := h.ReadyTicket("Next")
	r.Settle()

	wantStopped(t, h, stopped)
	wantHeld(t, h, stopped)
	r.WantOutput(stopped + ": stopped by the human: stopping its turn (Claim kept)")
	waitForTranscript(h, r, stopped, "stopped by the human: stopping its turn (Claim kept)")
	if got := h.Signals(other); len(got) != 0 {
		t.Errorf("%s's turn got %q; other turns carry on", other, got)
	}
	h.WantState(next, domain.InReview)
	if b, err := os.ReadFile(filepath.Join(h.Worktree(stopped), "wip.txt")); err != nil || string(b) != "wip\n" || h.WorktreeHead(stopped) != head {
		t.Errorf("%s's worktree changed (wip.txt %q, %v); want it left as the turn left it", stopped, b, err)
	}

	r.Poll()
	r.Poll()
	if n := len(callsFor(h, stopped)); n != 1 {
		t.Errorf("%s ran %d times; a Stopped Claim is never resumed", stopped, n)
	}
	if strings.Contains(r.Stdout(), stopped+": stopped by the human: holding its Claim") {
		t.Errorf("a Claim whose turn was stopped is noted as held too:\n%s", r.Stdout())
	}
	h.Continue(other)
	r.Settle()
	h.WantState(other, domain.InReview)
}

func TestAResumedClaimInProgressResumesItsSessionToldItWasStopped(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Stopped")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	first := h.WantCalls(1)[0]
	stop(t, h, id)
	r.Poll()
	wantStopped(t, h, id)

	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	resume(t, h, id)
	r.Poll()

	h.WantState(id, domain.InReview)
	c := h.WantCalls(2)[1]
	if s, _ := c.Flag("--resume"); s != first.Session || c.Claim != first.Claim || c.Dir != h.Worktree(id) {
		t.Errorf("args %q under Claim %s in %s; want session %s resumed under its Claim in its worktree", c.Args, c.Claim, c.Dir, first.Session)
	}
	if !strings.Contains(c.Prompt, stoppedPrompt) || !strings.Contains(c.Prompt, "Claim: "+first.Claim) {
		t.Errorf("prompt %q: want it told a human stopped it and resumed it, naming its Claim", c.Prompt)
	}
	r.WantOutput(id + ` resumed (implementation): "Stopped" after a Stop`)
	waitForTranscript(h, r, id, `resumed (implementation): "Stopped" after a Stop`)
}

func TestAStoppedAnsweredClaimIsHeldUntilResumedThenGetsItsAnswers(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id, first, r := answeredTicket(t, h)
	stop(t, h, id)

	r.Poll()
	r.Poll()
	if s := h.Ticket(id).State; s != domain.Answered || len(h.Calls()) != 1 {
		t.Fatalf("%s is %s after %d runs; want its answers left until a Resume", id, s, len(h.Calls()))
	}
	wantHeld(t, h, id)
	if n := strings.Count(r.Stdout(), id+": stopped by the human: holding its Claim"); n != 1 {
		t.Errorf("the holding line was printed %d times, want once:\n%s", n, r.Stdout())
	}

	resume(t, h, id)
	r.Poll()
	h.WantState(id, domain.InReview)
	c := h.WantCalls(2)[1]
	if s, _ := c.Flag("--resume"); s != first.Session || !strings.Contains(c.Prompt, "Blue") {
		t.Errorf("%s resumed with %q; want its session resumed with the answers", id, c.Args)
	}
	r.WantOutput(id + ` resumed (implementation): "Ask first" after a Stop`)
}

func TestAResumeAfterAStopWaitsAPollWhileAnotherProcessHoldsTheSession(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Taken over")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	first := h.WantCalls(1)[0]
	stop(t, h, id)
	r.Poll()
	release := h.HoldSession(first.Session)
	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	resume(t, h, id)

	r.Poll()
	if n := len(h.Calls()); n != 1 {
		t.Fatalf("claude ran %d times; want no resume while another process holds the session", n)
	}
	r.WantOutput("session " + first.Session + " is held by pid")

	release()
	r.Poll()
	h.WantState(id, domain.InReview)
}

func TestASavedStoppedClaimIsHeldNotResumedAfterARestart(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Stopped while down")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	first := h.WantCalls(1)[0]
	r.Signal(os.Interrupt)
	r.Signal(os.Interrupt)
	r.Exited()
	stop(t, h, id)

	r = h.Start()
	r.Poll()
	if n := len(h.Calls()); n != 1 {
		t.Fatalf("claude ran %d times; a saved Stopped Claim isn't resumed", n)
	}
	wantHeld(t, h, id)
	r.WantOutput(id + ": stopped by the human: holding its Claim")

	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	resume(t, h, id)
	r.Poll()
	h.WantState(id, domain.InReview)
	if c := h.WantCalls(2)[1]; c.Claim != first.Claim || !strings.Contains(c.Prompt, stoppedPrompt) {
		t.Errorf("resumed under Claim %s with %q; want Claim %s told it was stopped", c.Claim, c.Prompt, first.Claim)
	}
}

func TestAStoppedCutOffClaimIsNotResumedAfterAnOutagePause(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	h.Backoff = time.Minute
	id := h.ReadyTicket("Cut off")
	h.Script(id, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WantOutput("waiting on rate_limit")
	stop(t, h, id)

	for range 4 {
		r.Poll()
	}
	if n := len(h.Calls()); n != 1 {
		t.Errorf("claude ran %d times; a Stopped cut-off Claim isn't resumed", n)
	}
	wantHeld(t, h, id)
}

func TestAStopDuringADrainTakesEffectWhenTheTurnEndsAndHoldsOnTheNextStart(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Draining")
	h.Script(id, factorytest.Wait(), factorytest.Result("success"))
	r := h.Start()
	h.WantCalls(1)
	r.Interrupt()
	r.Settle()
	stop(t, h, id)

	h.Continue(id)
	if code := r.Exited(); code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	wantHeld(t, h, id)

	r = h.Start()
	r.Poll()
	if n := len(h.Calls()); n != 1 {
		t.Errorf("claude ran %d times; the Stopped Claim isn't resumed on the next start", n)
	}
	r.WantOutput(id + ": stopped by the human: holding its Claim")
}

func TestAClaimStoppedAgainAfterAResumeIsHeldAgain(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	id := h.ReadyTicket("Stopped twice")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	first := h.WantCalls(1)[0]
	stop(t, h, id)
	r.Poll()
	release := h.HoldSession(first.Session)
	resume(t, h, id)
	r.Poll()

	stop(t, h, id)
	r.Poll()
	release()
	r.Poll()

	r.WantOutput(id + ": stopped by the human: holding its Claim")
	if n := len(h.Calls()); n != 1 {
		t.Errorf("claude ran %d times; a Claim Stopped again isn't resumed", n)
	}
}
