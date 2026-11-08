package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Rounds and `wait` (ticket 29).

// exitWaitLeft is wait's exit code when the Ticket left for a State other
// than Answered.
const exitWaitLeft = 3

// exitCode makes Run exit with code after the command printed its output.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

// waitRetry is how long wait backs off after the service can't be reached.
const waitRetry = 2 * time.Second

func runPostRound(ctx context.Context, e Env, args []string) error {
	fs := flagSet("post-round")
	claim := fs.String("claim", "", "")
	session := fs.String("session", "", "")
	questions := fs.String("questions", "", "")
	file := fs.String("file", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	if *questions == "" && *file == "" {
		*file = "-" // the Questions are long input: stdin by default
	}
	text, err := longInput(e, *questions, *file)
	if err != nil {
		return err
	}
	in := core.PostRoundInput{Ticket: ref, ClaimID: *claim, SessionID: sessionID(e, *session)}
	if err := core.Decode(ctx, []byte(text), &in.Questions); err != nil {
		var ce *core.Error
		if errors.As(err, &ce) {
			ce.Field = "questions" + dotted(ce.Field)
		}
		return err
	}
	v, err := sc.client.PostRound(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func dotted(f string) string {
	if f == "" {
		return ""
	}
	return "." + f
}

func runPickUpAnswers(ctx context.Context, e Env, args []string) error {
	fs := flagSet("pick-up-answers")
	claim := fs.String("claim", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.PickUpAnswers(ctx, sc.project, core.PickUpAnswersInput{Ticket: ref, ClaimID: *claim})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

// waitAnswered is what wait prints when the Ticket is Answered.
type waitAnswered struct {
	Ticket   string      `json:"ticket"`
	State    store.State `json:"state"`
	NextStep string      `json:"next_step"`
}

// runWait loops on the long-poll until the Ticket leaves Needs Answer,
// retrying through connection errors. It takes no Claim ID: it is a read,
// and pick_up_answers finds out whether the Claim still holds.
func runWait(ctx context.Context, e Env, args []string) error {
	fs := flagSet("wait")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	for {
		res, err := sc.client.Wait(ctx, sc.project, ref, store.NeedsAnswer)
		var ce *core.Error
		if errors.As(err, &ce) && ce.Code == core.ServiceUnavailable {
			select {
			case <-e.After(waitRetry):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err != nil {
			return err
		}
		switch res.State {
		case store.NeedsAnswer:
			continue
		case store.Answered:
			return printJSON(e, waitAnswered{Ticket: res.Ticket, State: res.State, NextStep: fmt.Sprintf(
				"Call pick_up_answers on %s with your Claim ID to read the answers and carry on.", res.Ticket)})
		}
		if err := printJSON(e, res); err != nil {
			return err
		}
		return exitCode(exitWaitLeft)
	}
}
