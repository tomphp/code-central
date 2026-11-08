package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Landing Accepted work (ADR 0006).
func init() {
	commands["land"] = command{op: "land", usage: "land <id> --commit <sha>  the work is in the base: Accepted or In Review to Done, recording the commit that put it there", run: runLand}
}

func runLand(ctx context.Context, e Env, args []string) error {
	fs := flagSet("land")
	commit := fs.String("commit", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.Land(ctx, sc.project, core.LandInput{Ticket: ref, Commit: *commit})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
