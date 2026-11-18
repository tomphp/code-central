package cli

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ops"
)

// Commands for Blockers, cancelling and finding work (ticket 28). This
// file sorts after cli.go, so commands is already built. All but cancel,
// whose reason is long input, are derived from their definitions.
func init() {
	for name, c := range map[string]command{
		"add-blocker":    derive(ops.AddBlocker, (*client.Client).AddBlocker, "make <id> wait on <blocker-id> until it is Done"),
		"remove-blocker": derive(ops.RemoveBlocker, (*client.Client).RemoveBlocker, ""),
		"cancel":         handWritten(ops.Cancel, "cancel <id> <replacement-id>... [--reason <text> | --file <path|->] [--full]", runCancel),
		"get-spec":       derive(ops.GetSpec, (*client.Client).GetSpec, ""),
		"find-specs":     derive(ops.FindSpecs, (*client.Client).FindSpecs, ""),
	} {
		commands[name] = c
	}
}

// tagFlags turns repeated --tag <key>=<value> flags into a Tag filter.
func tagFlags(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	tags := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, usagef("a Tag is <key>=<value>, not %q", p)
		}
		tags[k] = v
	}
	return tags, nil
}

func runCancel(ctx context.Context, e Env, args []string) error {
	fs := flagSet("cancel")
	reason := fs.String("reason", "", "")
	file := fs.String("file", "", "")
	full := fs.Bool("full", false, "")
	sc, ref, reps, err := itemCommand(e, fs, args, 0, -1)
	if err != nil {
		return err
	}
	text, err := longInput(e, *reason, *file)
	if err != nil {
		return err
	}
	in := core.CancelInput{Ticket: ref, Reason: text, Full: *full}
	for _, r := range reps {
		in.Replacements = append(in.Replacements, core.Ref(r))
	}
	v, err := sc.client.Cancel(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
