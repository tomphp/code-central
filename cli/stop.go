package cli

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/client"
)

// The stop and resume commands (Spec code-central-1233) sit outside the
// operation table, like repository: they call the human's stop and resume
// through the client, as the human, so agents still get no command for
// them.
func init() {
	commands["stop"] = command{
		usage: "stop <id> [--note <md>] [--project <key>]  stop the Factory's session on an In Progress or Answered Ticket, keeping its State and Claim until resumed or released",
		run: func(ctx context.Context, e Env, args []string) error {
			return runMark(ctx, e, "stop", args, func(c *client.Client, project string, ticket api.Ref, note string) (api.Written[api.TicketView], error) {
				return c.Stop(ctx, project, api.StopInput{Ticket: ticket, Note: note})
			})
		},
	}
	commands["resume"] = command{
		usage: "resume <id> [--note <md>] [--project <key>]  resume a Stopped Claim, so the Factory takes it up again",
		run: func(ctx context.Context, e Env, args []string) error {
			return runMark(ctx, e, "resume", args, func(c *client.Client, project string, ticket api.Ref, note string) (api.Written[api.TicketView], error) {
				return c.Resume(ctx, project, api.ResumeInput{Ticket: ticket, Note: note})
			})
		},
	}
}

// mark makes a stop or resume of ticket, with its note.
type mark func(c *client.Client, project string, ticket api.Ref, note string) (api.Written[api.TicketView], error)

// runMark parses a stop or resume and makes it, as the human.
func runMark(ctx context.Context, e Env, name string, args []string, write mark) error {
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
	w, err := write(sc.client, sc.project, api.Ref(pos[0]), *note)
	if err != nil {
		return err
	}
	return printJSON(e, w)
}
