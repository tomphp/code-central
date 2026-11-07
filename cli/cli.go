// Package cli is the code-central command. Operation commands are the
// kebab-case names of core's agent operations; each reaches the service
// through the client package and prints JSON. Errors go to stderr as the
// one error JSON shape with exit code 2; usage errors exit 1.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Version is stamped by `make build`.
var Version = "dev"

// Env is everything a command touches outside itself, so tests can run
// commands in-process.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string

	// ServeAddrs overrides the addresses serve listens on (default
	// 127.0.0.1:7733 and [::1]:7733). Tests use "127.0.0.1:0".
	ServeAddrs []string
	// OnListening, when set, is called with serve's bound addresses.
	OnListening func(addrs []string)
}

// Exit codes.
const (
	exitOK    = 0
	exitUsage = 1
	exitError = 2
)

// usageError is a mistake in the command line, reported as text, exit 1.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error { return usageError{fmt.Sprintf(format, args...)} }

// command is one subcommand. Operation commands carry the core operation
// they expose; the cross-door suite checks them against core.Operations.
type command struct {
	op    string // core operation name, "" for CLI-only commands
	usage string
	run   func(ctx context.Context, e Env, args []string) error
}

// Commands maps each command name to the core operation it exposes, or ""
// for CLI-only commands such as serve.
func Commands() map[string]string {
	out := map[string]string{}
	for name, c := range commands {
		out[name] = c.op
	}
	return out
}

var commands map[string]command

func init() {
	commands = map[string]command{
		"serve":         {usage: "serve [--store <dir>]  run the service in the foreground", run: runServe},
		"create-spec":   {op: "create_spec", usage: "create-spec --title <t> [--body <md> | --file <path|->]", run: runCreateSpec},
		"create-ticket": {op: "create_ticket", usage: "create-ticket --spec <id> --title <t> [--state Draft|Ready] [--label <l>]... [--description <md> | --file <path|->]", run: runCreateTicket},
		"get-ticket":    {op: "get_ticket", usage: "get-ticket <id>", run: runGetTicket},
		"find-tickets":  {op: "find_tickets", usage: "find-tickets [--state <s>]... [--title <t>] [--label <l>]... [--spec <id>]", run: runFindTickets},
	}
}

// Run runs one command and returns its exit code.
func Run(ctx context.Context, args []string, e Env) int {
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		w, code := e.Stdout, exitOK
		if len(args) == 0 {
			w, code = e.Stderr, exitUsage
		}
		printUsage(w)
		return code
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(e.Stderr, "code-central: unknown command %q\n\n", args[0])
		printUsage(e.Stderr)
		return exitUsage
	}
	err := cmd.run(ctx, e, args[1:])
	var ue usageError
	var ce *core.Error
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &ue):
		fmt.Fprintf(e.Stderr, "code-central %s: %s\nusage: code-central %s\n", args[0], ue.msg, cmd.usage)
		return exitUsage
	case errors.As(err, &ce):
		b, _ := json.Marshal(ce)
		fmt.Fprintf(e.Stderr, "%s\n", b)
		return exitError
	default:
		fmt.Fprintf(e.Stderr, "code-central %s: %v\n", args[0], err)
		return exitUsage
	}
}

func printUsage(w io.Writer) {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(w, "usage: code-central <command> [flags]")
	fmt.Fprintln(w, "\nOperation commands take --project <key> (or CODE_CENTRAL_PROJECT) and talk to")
	fmt.Fprintln(w, "the service at CODE_CENTRAL_URL (default "+client.DefaultURL+").\n\nCommands:")
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", commands[n].usage)
	}
}

// flagSet is a FlagSet that reports errors instead of printing them.
func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parse parses flags that may be interspersed with positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usageError{err.Error()}
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// strs is a repeatable string flag.
type strs []string

func (s *strs) String() string     { return strings.Join(*s, ",") }
func (s *strs) Set(v string) error { *s = append(*s, v); return nil }

// scope holds what every operation command needs: the Project and a
// client for the service.
type scope struct {
	project string
	client  *client.Client
}

func addScope(fs *flag.FlagSet) *string {
	return fs.String("project", "", "Project Key (default $CODE_CENTRAL_PROJECT)")
}

func resolveScope(e Env, projectFlag string) (scope, error) {
	project := projectFlag
	if project == "" {
		project = e.Getenv("CODE_CENTRAL_PROJECT")
	}
	if project == "" {
		return scope{}, usagef("no Project: pass --project <key> or set CODE_CENTRAL_PROJECT")
	}
	url := e.Getenv("CODE_CENTRAL_URL")
	if url == "" {
		url = client.DefaultURL
	}
	return scope{project: project, client: client.New(url)}, nil
}

func printJSON(e Env, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.Stdout, "%s\n", b)
	return err
}

// longInput reads text from --file (a path, or - for stdin), else returns
// the inline flag value.
func longInput(e Env, inline, file string) (string, error) {
	if file == "" {
		return inline, nil
	}
	if inline != "" {
		return "", usagef("pass the text inline or with --file, not both")
	}
	if file == "-" {
		b, err := io.ReadAll(e.Stdin)
		return string(b), err
	}
	b, err := os.ReadFile(file)
	return string(b), err
}

func runCreateSpec(ctx context.Context, e Env, args []string) error {
	fs := flagSet("create-spec")
	project := addScope(fs)
	title := fs.String("title", "", "")
	body := fs.String("body", "", "")
	file := fs.String("file", "", "")
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
	text, err := longInput(e, *body, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.CreateSpec(ctx, sc.project, core.CreateSpecInput{Title: *title, Body: text})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runCreateTicket(ctx context.Context, e Env, args []string) error {
	fs := flagSet("create-ticket")
	project := addScope(fs)
	spec := fs.String("spec", "", "")
	title := fs.String("title", "", "")
	state := fs.String("state", "", "")
	desc := fs.String("description", "", "")
	file := fs.String("file", "", "")
	var labels strs
	fs.Var(&labels, "label", "")
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
	text, err := longInput(e, *desc, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.CreateTicket(ctx, sc.project, core.CreateTicketInput{
		Spec: core.Ref(*spec), Title: *title, Description: text, State: store.State(*state), Labels: labels,
	})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runGetTicket(ctx context.Context, e Env, args []string) error {
	fs := flagSet("get-ticket")
	project := addScope(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("expected one Ticket ID")
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	v, err := sc.client.GetTicket(ctx, sc.project, core.Ref(pos[0]))
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runFindTickets(ctx context.Context, e Env, args []string) error {
	fs := flagSet("find-tickets")
	project := addScope(fs)
	title := fs.String("title", "", "")
	spec := fs.String("spec", "", "")
	var states, labels strs
	fs.Var(&states, "state", "")
	fs.Var(&labels, "label", "")
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
	in := core.FindTicketsInput{Title: *title, Labels: labels, Spec: core.Ref(*spec)}
	for _, s := range states {
		in.State = append(in.State, store.State(s))
	}
	v, err := sc.client.FindTickets(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
