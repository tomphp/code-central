package cli

import (
	"context"
	"strings"

	"github.com/tomoram/code-central/core"
)

// Commands for Blockers, cancelling and finding work (ticket 28). This
// file sorts after cli.go, so commands is already built.
func init() {
	for name, c := range map[string]command{
		"add-blocker":    {op: "add_blocker", usage: "add-blocker <id> <blocker-id>  make <id> wait on <blocker-id> until it is Done", run: runAddBlocker},
		"remove-blocker": {op: "remove_blocker", usage: "remove-blocker <id> <blocker-id>", run: runRemoveBlocker},
		"cancel":         {op: "cancel", usage: "cancel <id> <replacement-id>... [--reason <text> | --file <path|->]", run: runCancel},
		"get-spec":       {op: "get_spec", usage: "get-spec <id> [--part body|tickets|edits]... [--section <heading>]...", run: runGetSpec},
		"find-specs":     {op: "find_specs", usage: "find-specs [--title <t>] [--label <l>]... [--tag <key>=<value>]... [--finished] [--requirements]", run: runFindSpecs},
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

func blockerCommand(e Env, name string, args []string, op func(scope, core.BlockerInput) (core.TicketView, error)) error {
	sc, ref, rest, err := itemCommand(e, flagSet(name), args, 1, 1)
	if err != nil {
		return err
	}
	v, err := op(sc, core.BlockerInput{Ticket: ref, Blocker: core.Ref(rest[0])})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runAddBlocker(ctx context.Context, e Env, args []string) error {
	return blockerCommand(e, "add-blocker", args, func(sc scope, in core.BlockerInput) (core.TicketView, error) {
		return sc.client.AddBlocker(ctx, sc.project, in)
	})
}

func runRemoveBlocker(ctx context.Context, e Env, args []string) error {
	return blockerCommand(e, "remove-blocker", args, func(sc scope, in core.BlockerInput) (core.TicketView, error) {
		return sc.client.RemoveBlocker(ctx, sc.project, in)
	})
}

func runCancel(ctx context.Context, e Env, args []string) error {
	fs := flagSet("cancel")
	reason := fs.String("reason", "", "")
	file := fs.String("file", "", "")
	sc, ref, reps, err := itemCommand(e, fs, args, 0, -1)
	if err != nil {
		return err
	}
	text, err := longInput(e, *reason, *file)
	if err != nil {
		return err
	}
	in := core.CancelInput{Ticket: ref, Reason: text}
	for _, r := range reps {
		in.Replacements = append(in.Replacements, core.Ref(r))
	}
	v, err := sc.client.Cancel(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runGetSpec(ctx context.Context, e Env, args []string) error {
	fs := flagSet("get-spec")
	var parts, sections strs
	fs.Var(&parts, "part", "")
	fs.Var(&sections, "section", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.GetSpec(ctx, sc.project, core.GetSpecInput{Spec: ref, Parts: parts, Sections: sections})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runFindSpecs(ctx context.Context, e Env, args []string) error {
	fs := flagSet("find-specs")
	project := addScope(fs)
	title := fs.String("title", "", "")
	finished := fs.Bool("finished", false, "")
	requirements := fs.Bool("requirements", false, "")
	var labels, tags strs
	fs.Var(&labels, "label", "")
	fs.Var(&tags, "tag", "")
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
	in := core.FindSpecsInput{Title: *title, Labels: labels, Finished: *finished, Requirements: *requirements}
	if in.Tags, err = tagFlags(tags); err != nil {
		return err
	}
	v, err := sc.client.FindSpecs(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
