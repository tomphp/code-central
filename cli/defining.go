package cli

import (
	"context"
	"flag"

	"github.com/tomoram/code-central/core"
)

// Commands for defining work: edits, comments and defer. Each takes the
// Spec or Ticket ID as its first argument. Labels, Tags and approve are
// derived from their definitions.

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
	full := fs.Bool("full", false, "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	in := core.EditTicketInput{Ticket: ref, Full: *full}
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

func runComment(ctx context.Context, e Env, args []string) error {
	fs := flagSet("comment")
	body := fs.String("body", "", "")
	file := fs.String("file", "", "")
	full := fs.Bool("full", false, "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *body, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.Comment(ctx, sc.project, core.CommentInput{Ticket: ref, Body: text, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runDefer(ctx context.Context, e Env, args []string) error {
	fs := flagSet("defer")
	reason := fs.String("reason", "", "")
	file := fs.String("file", "", "")
	full := fs.Bool("full", false, "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	text, err := longInput(e, *reason, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.Defer(ctx, sc.project, core.DeferInput{Ticket: ref, Reason: text, Full: *full})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
