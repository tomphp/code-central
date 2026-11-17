package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Landing Accepted work (ADR 0006).
func init() {
	commands["land"] = command{op: "land", usage: "land <id> --commit <sha> [--repository <name>] [--full]  the work is in the base: Accepted or In Review to Done, recording the commit that put it there; with --repository, that Repository's landed commit, Done once every Changed Repository has landed", run: runLand}
}

func runLand(ctx context.Context, e Env, args []string) error {
	fs := flagSet("land")
	commit := fs.String("commit", "", "")
	repository := fs.String("repository", "", "")
	full := fs.Bool("full", false, "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.Land(ctx, sc.project, core.LandInput{Ticket: ref, Commit: *commit, Repository: *repository, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
