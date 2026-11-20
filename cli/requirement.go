package cli

import (
	"context"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/ops"
)

// Submitting a Requirement.
func init() {
	commands["submit-requirement"] = handWritten(ops.SubmitRequirement, "submit-requirement [--text <md> | --file <path|->] [--map <spec>] [--skip-grilling] [--draft] [--full]  submit a Requirement, from a finished Map with --map; its first line becomes the Spec's title", runSubmitRequirement)
}

func runSubmitRequirement(ctx context.Context, e Env, args []string) error {
	fs := flagSet("submit-requirement")
	project := addScope(fs)
	inline := fs.String("text", "", "")
	file := fs.String("file", "", "")
	mapRef := fs.String("map", "", "")
	skip := fs.Bool("skip-grilling", false, "")
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
	v, err := sc.client.SubmitRequirement(ctx, sc.project, api.SubmitRequirementInput{Text: text, Map: api.Ref(*mapRef), SkipGrilling: *skip, Draft: *draft, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
