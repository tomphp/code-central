package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Submitting a Prototype.
func init() {
	commands["submit-prototype"] = command{op: "submit_prototype", usage: "submit-prototype <spec> [--text <md> | --file <path|->] [--full]  submit a Prototype on the Spec; its first line becomes the title", run: runSubmitPrototype}
}

func runSubmitPrototype(ctx context.Context, e Env, args []string) error {
	fs := flagSet("submit-prototype")
	inline := fs.String("text", "", "")
	file := fs.String("file", "", "")
	full := fs.Bool("full", false, "")
	sc, spec, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *inline, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.SubmitPrototype(ctx, sc.project, core.SubmitPrototypeInput{Spec: spec, Text: text, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
