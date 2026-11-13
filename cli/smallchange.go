package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Small Change.
func init() {
	commands["submit-small-change"] = command{op: "submit_small_change", usage: "submit-small-change [--text <md> | --file <path|->] [--draft] [--full]  submit a Small Change; its first line becomes the Ticket's title", run: runSubmitSmallChange}
}

func runSubmitSmallChange(ctx context.Context, e Env, args []string) error {
	fs := flagSet("submit-small-change")
	project := addScope(fs)
	inline := fs.String("text", "", "")
	file := fs.String("file", "", "")
	draft := fs.Bool("draft", false, "")
	full := fs.Bool("full", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	text, err := longInput(e, *inline, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.SubmitSmallChange(ctx, sc.project, core.SubmitSmallChangeInput{Text: text, Draft: *draft, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
