package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Map.
func init() {
	commands["submit-map"] = command{op: "submit_map", usage: "submit-map [--text <md> | --file <path|->] [--full]  submit a Map; its first line becomes the Spec's title", run: runSubmitMap}
}

func runSubmitMap(ctx context.Context, e Env, args []string) error {
	fs := flagSet("submit-map")
	project := addScope(fs)
	inline := fs.String("text", "", "")
	file := fs.String("file", "", "")
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
	v, err := sc.client.SubmitMap(ctx, sc.project, core.SubmitMapInput{Text: text, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
