package cli

import (
	"context"

	"github.com/tomoram/code-central/client"
)

// The published workflow (ticket 26). `workflow` is the spec's name for
// the command; `get-workflow` is the operation's kebab-case name, which
// every operation command has.
func init() {
	commands["get-workflow"] = command{op: "get_workflow", usage: "get-workflow  print the workflow (same as workflow)", run: runWorkflow}
	commands["workflow"] = command{usage: "workflow  print the workflow: States, actions and how to work a Ticket", run: runWorkflow}
}

// runWorkflow needs no Project: the workflow is the same for every one.
func runWorkflow(ctx context.Context, e Env, args []string) error {
	fs := flagSet("workflow")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	w, err := client.New(serviceURL(e)).GetWorkflow(ctx)
	if err != nil {
		return err
	}
	return printJSON(e, w)
}
