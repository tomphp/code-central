package cli

import (
	"context"

	"github.com/tomoram/code-central/api"
)

// The stop and resume commands (Spec code-central-1233) sit outside the
// operation table, like repository: they call the human's stop and resume
// through the client, as the human, so agents still get no command for
// them.
func init() {
	commands["stop"] = command{
		usage: "stop <id> [--note <md>] [--project <key>]  stop the Factory's session on an In Progress or Answered Ticket, keeping its State and Claim until resumed or released",
		run: func(ctx context.Context, e Env, args []string) error {
			return runMark(ctx, e, "stop", args, func(sc scope, in api.StopInput) (api.Written[api.TicketView], error) {
				return sc.client.Stop(ctx, sc.project, in)
			})
		},
	}
	commands["resume"] = command{
		usage: "resume <id> [--note <md>] [--project <key>]  resume a Stopped Claim, so the Factory takes it up again",
		run: func(ctx context.Context, e Env, args []string) error {
			return runMark(ctx, e, "resume", args, func(sc scope, in api.StopInput) (api.Written[api.TicketView], error) {
				return sc.client.Resume(ctx, sc.project, api.ResumeInput(in))
			})
		},
	}
}

// runMark parses a stop or resume and makes it, as the human.
func runMark(ctx context.Context, e Env, name string, args []string, mark func(scope, api.StopInput) (api.Written[api.TicketView], error)) error {
	fs := flagSet(name)
	project := addScope(fs)
	note := fs.String("note", "", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("%s takes one Ticket", name)
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	sc.client.Actor = api.Human
	w, err := mark(sc, api.StopInput{Ticket: api.Ref(pos[0]), Note: *note})
	if err != nil {
		return err
	}
	return printJSON(e, w)
}
