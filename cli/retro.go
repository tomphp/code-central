package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Starting a Retro.
func init() {
	commands["submit-retro"] = command{op: "submit_retro", usage: "submit-retro <ticket|spec> [--note <md>] [--full]  start a Retro on a finished Ticket or Spec", run: runSubmitRetro}
}

func runSubmitRetro(ctx context.Context, e Env, args []string) error {
	fs := flagSet("submit-retro")
	note := fs.String("note", "", "")
	full := fs.Bool("full", false, "")
	sc, target, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.SubmitRetro(ctx, sc.project, core.SubmitRetroInput{Target: target, Note: *note, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
