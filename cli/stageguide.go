package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Stage Guides (ticket 73).
func init() {
	commands["get-stage-guide"] = command{op: "get_stage_guide", usage: "get-stage-guide grilling|spec|tickets|implementation  print a Stage Guide", run: runStageGuide}
}

// runStageGuide needs no Project: the guides are the same for every one.
func runStageGuide(ctx context.Context, e Env, args []string) error {
	fs := flagSet("get-stage-guide")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("get-stage-guide takes one Stage: grilling, spec, tickets or implementation")
	}
	g, err := newClient(e).GetStageGuide(ctx, core.StageGuideInput{Stage: pos[0]})
	if err != nil {
		return err
	}
	return printJSON(e, g)
}
