package factory_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// setTags sets tags on ticket, as the human.
func setTags(t *testing.T, h *factorytest.Harness, ticket string, tags map[string]string) {
	t.Helper()
	if _, err := h.Human().SetTags(context.Background(), h.Project, core.SetTagsInput{Target: core.Ref(ticket), Tags: tags}); err != nil {
		t.Fatal(err)
	}
}

// ranWith checks that c passed --model and --effort as want, "" meaning
// no flag, and that ticket's Claim recorded them.
func ranWith(t *testing.T, h *factorytest.Harness, ticket string, c factorytest.Call, model, effort string) {
	t.Helper()
	for flag, want := range map[string]string{"--model": model, "--effort": effort} {
		got, ok := c.Flag(flag)
		if want == "" && ok {
			t.Errorf("args %q: want no %s", c.Args, flag)
		}
		if want != "" && got != want {
			t.Errorf("args %q: want %s %s", c.Args, flag, want)
		}
	}
	h.WaitFor("the Claim's model and effort", func() bool {
		claims := h.Ticket(ticket).Claims
		cl := claims[len(claims)-1]
		return cl.Model == model && cl.Effort == effort
	})
}

func TestATicketsTagsSetTheFirstTurnsModelAndEffortOverTheFactorysFlags(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Hard work")
	setTags(t, h, id, map[string]string{"model": "opus", "effort": "max"})
	h.Start("--model", "sonnet", "--effort", "low")
	h.WaitForState(id, store.InReview)
	ranWith(t, h, id, h.Calls()[0], "opus", "max")
}

func TestTheFactorysFlagsFillWhatATicketsTagsLeaveUnset(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	effortOnly := h.ReadyTicket("Effort only")
	modelOnly := h.ReadyTicket("Model only")
	setTags(t, h, effortOnly, map[string]string{"effort": "high"})
	setTags(t, h, modelOnly, map[string]string{"model": "fable"})
	h.Start("--model", "sonnet", "--effort", "low")
	h.WaitForState(effortOnly, store.InReview)
	h.WaitForState(modelOnly, store.InReview)
	ranWith(t, h, effortOnly, callsFor(h, effortOnly)[0], "sonnet", "high")
	ranWith(t, h, modelOnly, callsFor(h, modelOnly)[0], "fable", "low")
}

func TestWithNothingSetNoModelOrEffortIsPassed(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Work")
	h.Start()
	h.WaitForState(id, store.InReview)
	ranWith(t, h, id, h.Calls()[0], "", "")
}

func TestATagChangedWhileATicketWaitsOnARoundAppliesWhenItResumes(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, first, r := answeredTicket(t, h, "--effort", "low")
	ranWith(t, h, id, first, "", "low")
	setTags(t, h, id, map[string]string{"model": "opus"})

	r.Poll()
	h.WaitForState(id, store.InReview)
	ranWith(t, h, id, h.Calls()[1], "opus", "low")
}

func TestAnAnsweredResumeWaitsAPollWhenItsModelChoiceCantBeRead(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, _, r := answeredTicket(t, h)
	h.SetFailing("/model-choice")

	r.Poll()
	r.WaitForOutput(id + ": get_model_choice: ")
	if s := h.Ticket(id).State; s != store.Answered || len(h.Calls()) != 1 {
		t.Fatalf("%s is %s after %d runs; want it left Answered, its answers not picked up", id, s, len(h.Calls()))
	}

	h.SetFailing("")
	r.Poll()
	h.WaitForState(id, store.InReview)
}

func TestACutOffTurnResumesWithTheModelChoiceReadAfresh(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Cut off")
	h.Script(id, factorytest.APIError("overloaded_error"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start("--model", "sonnet")
	r.WaitForOutput("in 1ms")
	ranWith(t, h, id, h.Calls()[0], "sonnet", "")

	setTags(t, h, id, map[string]string{"effort": "xhigh"})
	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	h.SetFailing("/model-choice")
	h.Clock.Advance(time.Millisecond)
	r.Poll()
	r.WaitForOutput(id + ": get_model_choice: ")
	if n := len(h.Calls()); n != 1 {
		t.Fatalf("%d runs; want the resume to wait while its Model Choice can't be read", n)
	}

	h.SetFailing("")
	r.Poll()
	h.WaitForState(id, store.InReview)
	c := h.Calls()[1]
	if !strings.Contains(c.Prompt, "Your last turn was cut off") {
		t.Fatalf("prompt %q, want the cut-off resume", c.Prompt)
	}
	ranWith(t, h, id, c, "sonnet", "xhigh")
}

func TestAClaimResumedAfterARestartTakesItsModelChoiceAfresh(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Cut off by a stop")
	h.Script(id, factorytest.Wait())
	r := h.Start()
	h.WaitFor("the first turn", func() bool { return len(h.Calls()) == 1 })
	r.Signal(os.Interrupt)
	r.Signal(os.Interrupt)
	r.Exited()

	setTags(t, h, id, map[string]string{"model": "opus", "effort": "medium"})
	h.Script(id, factorytest.Finish("Done."), factorytest.Result("success"))
	h.Start()
	h.WaitForState(id, store.InReview)
	ranWith(t, h, id, h.Calls()[1], "opus", "medium")
}
