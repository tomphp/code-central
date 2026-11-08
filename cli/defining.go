package cli

import (
	"context"
	"flag"
	"strings"

	"github.com/tomoram/code-central/core"
)

// Commands for defining work: edits, Labels, Tags, comments, approve and
// defer. Each takes the Spec or Ticket ID as its first argument.

// itemCommand parses args for a command on one item, returning the scope,
// the item's ref and the remaining positional arguments.
func itemCommand(e Env, fs *flag.FlagSet, args []string, minRest, maxRest int) (scope, core.Ref, []string, error) {
	project := addScope(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return scope{}, "", nil, err
	}
	if len(pos) == 0 {
		return scope{}, "", nil, usagef("expected a Spec or Ticket ID")
	}
	rest := pos[1:]
	if len(rest) < minRest {
		return scope{}, "", nil, usagef("expected at least %d argument(s) after the ID", minRest)
	}
	if maxRest >= 0 && len(rest) > maxRest {
		return scope{}, "", nil, usagef("unexpected argument %q", rest[maxRest])
	}
	sc, err := resolveScope(e, *project)
	return sc, core.Ref(pos[0]), rest, err
}

// isSet reports whether the flag was given, so an empty value can be told
// apart from no value.
func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// optionalText is a flag's value when it or --file was given, else nil.
func optionalText(e Env, fs *flag.FlagSet, name, inline, file string) (*string, error) {
	if !isSet(fs, name) && file == "" {
		return nil, nil
	}
	s, err := longInput(e, inline, file)
	return &s, err
}

func runEditSpec(ctx context.Context, e Env, args []string) error {
	fs := flagSet("edit-spec")
	title := fs.String("title", "", "")
	body := fs.String("body", "", "")
	file := fs.String("file", "", "")
	full := fs.Bool("full", false, "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	in := core.EditSpecInput{Spec: ref, Full: *full}
	if isSet(fs, "title") {
		in.Title = title
	}
	if in.Body, err = optionalText(e, fs, "body", *body, *file); err != nil {
		return err
	}
	v, err := sc.client.EditSpec(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runEditTicket(ctx context.Context, e Env, args []string) error {
	fs := flagSet("edit-ticket")
	title := fs.String("title", "", "")
	desc := fs.String("description", "", "")
	file := fs.String("file", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	in := core.EditTicketInput{Ticket: ref}
	if isSet(fs, "title") {
		in.Title = title
	}
	if in.Description, err = optionalText(e, fs, "description", *desc, *file); err != nil {
		return err
	}
	v, err := sc.client.EditTicket(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runAddLabels(ctx context.Context, e Env, args []string) error {
	sc, ref, labels, err := itemCommand(e, flagSet("add-labels"), args, 1, -1)
	if err != nil {
		return err
	}
	v, err := sc.client.AddLabels(ctx, sc.project, core.LabelsInput{Target: ref, Labels: labels})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runRemoveLabels(ctx context.Context, e Env, args []string) error {
	sc, ref, labels, err := itemCommand(e, flagSet("remove-labels"), args, 1, -1)
	if err != nil {
		return err
	}
	v, err := sc.client.RemoveLabels(ctx, sc.project, core.LabelsInput{Target: ref, Labels: labels})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runSetTags(ctx context.Context, e Env, args []string) error {
	sc, ref, pairs, err := itemCommand(e, flagSet("set-tags"), args, 1, -1)
	if err != nil {
		return err
	}
	tags := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return usagef("a Tag is <key>=<value>, not %q", p)
		}
		tags[k] = v
	}
	v, err := sc.client.SetTags(ctx, sc.project, core.SetTagsInput{Target: ref, Tags: tags})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runRemoveTags(ctx context.Context, e Env, args []string) error {
	sc, ref, keys, err := itemCommand(e, flagSet("remove-tags"), args, 1, -1)
	if err != nil {
		return err
	}
	v, err := sc.client.RemoveTags(ctx, sc.project, core.RemoveTagsInput{Target: ref, Keys: keys})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runComment(ctx context.Context, e Env, args []string) error {
	fs := flagSet("comment")
	body := fs.String("body", "", "")
	file := fs.String("file", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *body, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.Comment(ctx, sc.project, core.CommentInput{Ticket: ref, Body: text})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runApprove(ctx context.Context, e Env, args []string) error {
	sc, ref, _, err := itemCommand(e, flagSet("approve"), args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.Approve(ctx, sc.project, core.TicketInput{Ticket: ref})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runDefer(ctx context.Context, e Env, args []string) error {
	fs := flagSet("defer")
	reason := fs.String("reason", "", "")
	file := fs.String("file", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *reason, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.Defer(ctx, sc.project, core.DeferInput{Ticket: ref, Reason: text})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
