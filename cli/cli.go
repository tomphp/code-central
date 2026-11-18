// Package cli is the code-central command. Operation commands are the
// kebab-case names of core's agent operations; each reaches the service
// through the client package and prints JSON. Errors go to stderr as the
// one error JSON shape with exit code 2; usage errors exit 1. The package
// is commands only: serve reads its flags and the local settings and runs
// the assembled service, app.Run.
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
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/home"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/ops"
)

// Env is everything a command touches outside itself, so tests can run
// commands in-process.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	// UserHomeDir is the profile folder the default paths fall back to
	// when HOME isn't set (home.Lookup); main passes os.UserHomeDir. nil
	// means none, so tests never find the real one.
	UserHomeDir func() (string, error)
	// Dir is the working directory; "" means the process's.
	Dir string

	// ServeAddrs overrides the addresses serve listens on (default
	// 127.0.0.1:7733 and [::1]:7733). Tests use "127.0.0.1:0".
	ServeAddrs []string
	// OnListening, when set, is called with serve's bound addresses.
	OnListening func(addrs []string)
	// After, when set, replaces time.After for ping's time limit and
	// wait's back-off, so tests never wait on real time.
	After func(time.Duration) <-chan time.Time
	// Version, when set, replaces the stamped Version (tests).
	Version string

	// Program is the binary as invoked (os.Args[0]); service install
	// records it. "" means os.Executable.
	Program string
	// GOOS picks launchd or systemd for `service`; "" means runtime.GOOS.
	GOOS string
	// RunService runs launchctl or systemctl for `service`; nil runs the
	// real command. Tests always inject it.
	RunService func(ctx context.Context, name string, args ...string) error
	// Notifier shows serve's OS Notifications. nil sends none, so tests
	// never show a real one; main passes notify.OS().
	Notifier notify.Sender
}

// home is where the default paths are found.
func (e Env) home() home.Lookup {
	return home.Lookup{Getenv: e.Getenv, UserHomeDir: e.UserHomeDir}
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
// they expose; the cross-door suite checks them against ops.All().
type command struct {
	op    string // core operation name, "" for CLI-only commands
	usage string
	// help is what the command does, which --help prints after the usage:
	// the operation's one-line documentation.
	help string
	// hidden leaves the command out of the usage: the service runs it.
	hidden bool
	run    func(ctx context.Context, e Env, args []string) error
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

// An operation's command is derived from its definition where the input
// type's cli tags give today's command (derive.go). The others are
// hand-written: long input through --file or stdin, get-ticket's and
// find-tickets' default parts and brief answer, and claim's and
// set-session's session from the environment.
func init() {
	commands = map[string]command{
		"init":          {usage: "init [--key <key>] [--local | --committed]  set this repository up as a Project (--local keeps init's files out of git)", run: runInit},
		"create-spec":   handWritten(ops.CreateSpec, "create-spec --title <t> [--body <md> | --file <path|->] [--full]", runCreateSpec),
		"create-ticket": handWritten(ops.CreateTicket, "create-ticket --spec <id> --title <t> [--state Draft|Ready] [--label <l>]... [--blocked-by <id>]... [--tag <key>=<value>]... [--description <md> | --file <path|->] [--full]", runCreateTicket),
		"get-ticket":    handWritten(ops.GetTicket, "get-ticket <id> [--part claims|failure_reports|comments|rounds|moves|edits]...", runGetTicket),
		"find-tickets":  handWritten(ops.FindTickets, "find-tickets [--state <s>]... [--title <t>] [--label <l>]... [--tag <key>=<value>]... [--spec <id>] [--claimable[=false]] [--full]", runFindTickets),

		"edit-spec":     handWritten(ops.EditSpec, "edit-spec <id> [--title <t>] [--body <md> | --file <path|->] [--full]", runEditSpec),
		"edit-ticket":   handWritten(ops.EditTicket, "edit-ticket <id> [--title <t>] [--description <md> | --file <path|->] [--full]", runEditTicket),
		"add-labels":    derive(ops.AddLabels, (*client.Client).AddLabels, ""),
		"remove-labels": derive(ops.RemoveLabels, (*client.Client).RemoveLabels, ""),
		"set-tags":      derive(ops.SetTags, (*client.Client).SetTags, ""),
		"remove-tags":   derive(ops.RemoveTags, (*client.Client).RemoveTags, ""),
		"comment":       handWritten(ops.Comment, "comment <id> [--body <md> | --file <path|->] [--full]", runComment),
		"approve":       derive(ops.Approve, (*client.Client).Approve, ""),
		"defer":         handWritten(ops.Defer, "defer <id> [--reason <text> | --file <path|->] [--full]", runDefer),
		"claim":         handWritten(ops.Claim, "claim [--spec <id> | --ticket <id>] [--session <id>] [--working-dir <dir>] [--resume-command <command>]", runClaim),
		"finish":        handWritten(ops.Finish, "finish <id> --claim <claim-id> [--summary <md> | --file <path|->] [--branch <branch>] [--commit <sha>] [--repositories <json>] [--recommendation implement|split] [--full]  repositories as a JSON array of {name, branch, commit}", runFinish),

		"set-session": handWritten(ops.SetSession, "set-session <id> --claim <claim-id> [--session <id>] [--working-dir <dir>] [--resume-command <command>] [--full]  (session defaults to $CLAUDE_CODE_SESSION_ID)", runSetSession),
		"ping":        {usage: "ping [--session <id>]  mark this session active (reads hook JSON on stdin); always exits 0", run: runPing},
		"hook":        {usage: "hook session-id  PreToolUse hook: add session_id and client_version to the tool input", run: runHook},
		// Failure Reports and giving up (ticket 30).
		"report-failure": handWritten(ops.ReportFailure, "report-failure <id> --claim <claim-id> --what-broke <md> --why <md> --what-to-fix <md> --where-work-stands <md> | --file <json-path|-> [--full]", runReportFailure),
		"give-up":        handWritten(ops.GiveUp, "give-up <id> --claim <claim-id> [--note <md> | --file <path|->] [--full]", runGiveUp),
		"send-back":      handWritten(ops.SendBack, "send-back <id> [--note <md> | --file <path|->] [--full]  Accepted work that can't land (a merge conflict) back to Ready; the note is required", runSendBack),
		// Rounds and wait (ticket 29).
		"post-round":      handWritten(ops.PostRound, "post-round <id> --claim <claim-id> [--session <id>] [--questions <json> | --file <path|->] [--full]  Questions as a JSON array of {title, body, recommended?}, on stdin by default", runPostRound),
		"pick-up-answers": derive(ops.PickUpAnswers, (*client.Client).PickUpAnswers, ""),
		"wait":            {usage: "wait <id>  wait for the Ticket to leave Needs Answer: exit 0 when Answered, 3 when it left for another State", run: runWait},
	}
}

// Run runs one command and returns its exit code.
func Run(ctx context.Context, args []string, e Env) int {
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.After == nil {
		e.After = time.After
	}
	if e.Dir == "" {
		if wd, err := os.Getwd(); err == nil {
			e.Dir = wd
		}
	}
	if len(args) > 1 && args[0] == "help" {
		return runHelpTopic(e, args[1])
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		w, code := e.Stdout, exitOK
		if len(args) == 0 {
			w, code = e.Stderr, exitUsage
		}
		printUsage(w)
		return code
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Fprintf(e.Stdout, "code-central %s\n", e.version())
		return exitOK
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
	var ec exitCode
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprintf(e.Stdout, "usage: code-central %s\n", cmd.usage)
		if cmd.help != "" {
			fmt.Fprintf(e.Stdout, "\n%s\n", cmd.help)
		}
		return exitOK
	case errors.As(err, &ec):
		return int(ec)
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
	for n, c := range commands {
		if !c.hidden {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	fmt.Fprintln(w, "usage: code-central <command> [flags]")
	fmt.Fprintln(w, "\nOperation commands take --project <key> (else CODE_CENTRAL_PROJECT, else the key")
	fmt.Fprintln(w, "in the nearest .mcp.json) and talk to")
	fmt.Fprintln(w, "the service at CODE_CENTRAL_URL (default "+client.DefaultURL+").\n\nCommands:")
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", commands[n].usage)
	}
	fmt.Fprintln(w, "\nHelp topics: code-central help sandboxes")
}

// flagSet is a FlagSet that reports errors instead of printing them.
func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parse parses flags that may be interspersed with positional arguments.
// -h or --help is flag.ErrHelp, for Run to print the command's help.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
			return nil, err
		} else if err != nil {
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
	project, err := client.ResolveProject(e.Dir, projectFlag, e.Getenv)
	if err != nil {
		return scope{}, err
	}
	if project == "" {
		return scope{}, usagef("no Project: pass --project <key>, set CODE_CENTRAL_PROJECT, or run `code-central init` in the repository")
	}
	return scope{project: project, client: newClient(e)}, nil
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
	text, err := longInput(e, *body, *file)
	if err != nil {
		return err
	}
	v, err := sc.client.CreateSpec(ctx, sc.project, core.CreateSpecInput{Title: *title, Body: text, Full: *full})
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
	full := fs.Bool("full", false, "")
	var labels, blockedBy, tagPairs strs
	fs.Var(&labels, "label", "")
	fs.Var(&blockedBy, "blocked-by", "")
	fs.Var(&tagPairs, "tag", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	tags, err := tagFlags(tagPairs)
	if err != nil {
		return err
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	text, err := longInput(e, *desc, *file)
	if err != nil {
		return err
	}
	in := core.CreateTicketInput{
		Spec: core.Ref(*spec), Title: *title, Description: text, State: domain.State(*state), Labels: labels, Tags: tags, Full: *full,
	}
	for _, b := range blockedBy {
		in.BlockedBy = append(in.BlockedBy, core.Ref(b))
	}
	v, err := sc.client.CreateTicket(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runGetTicket(ctx context.Context, e Env, args []string) error {
	fs := flagSet("get-ticket")
	project := addScope(fs)
	var parts strs
	fs.Var(&parts, "part", "")
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
	// The HTTP API returns the whole Ticket when no part is named, so the
	// CLI names get_ticket's default parts itself, and briefs them.
	brief := len(parts) == 0
	if brief {
		parts = core.DefaultTicketParts()
	}
	v, err := sc.client.GetTicket(ctx, sc.project, core.Ref(pos[0]), parts...)
	if err != nil {
		return err
	}
	if brief {
		v = v.Brief()
	}
	return printJSON(e, v)
}

func runFindTickets(ctx context.Context, e Env, args []string) error {
	fs := flagSet("find-tickets")
	project := addScope(fs)
	title := fs.String("title", "", "")
	spec := fs.String("spec", "", "")
	claimable := fs.Bool("claimable", false, "")
	full := fs.Bool("full", false, "")
	var states, labels, tags strs
	fs.Var(&states, "state", "")
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
	in := core.FindTicketsInput{Title: *title, Labels: labels, Spec: core.Ref(*spec)}
	for _, s := range states {
		in.State = append(in.State, domain.State(s))
	}
	if in.Tags, err = tagFlags(tags); err != nil {
		return err
	}
	if isSet(fs, "claimable") {
		in.Claimable = claimable
	}
	if *full {
		// As get-ticket does: the default parts, brief.
		views, err := sc.client.FindTicketViews(ctx, sc.project, in, core.DefaultTicketParts()...)
		if err != nil {
			return err
		}
		for i := range views {
			views[i] = views[i].Brief()
		}
		return printJSON(e, views)
	}
	v, err := sc.client.FindTickets(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runClaim(ctx context.Context, e Env, args []string) error {
	fs := flagSet("claim")
	project := addScope(fs)
	spec := fs.String("spec", "", "")
	ticket := fs.String("ticket", "", "")
	session := fs.String("session", "", "")
	resume := addResumeFlags(fs)
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
	in := core.ClaimInput{Spec: core.Ref(*spec), Ticket: core.Ref(*ticket), SessionID: sessionID(e, *session)}
	in.WorkingDir, in.ResumeCommand = resume.values()
	v, err := sc.client.Claim(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

func runFinish(ctx context.Context, e Env, args []string) error {
	fs := flagSet("finish")
	project := addScope(fs)
	claim := fs.String("claim", "", "")
	summary := fs.String("summary", "", "")
	file := fs.String("file", "", "")
	branch := fs.String("branch", "", "")
	commit := fs.String("commit", "", "")
	repositories := fs.String("repositories", "", "")
	recommendation := fs.String("recommendation", "", "")
	full := fs.Bool("full", false, "")
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
	text, err := longInput(e, *summary, *file)
	if err != nil {
		return err
	}
	in := core.FinishInput{Ticket: core.Ref(pos[0]), ClaimID: *claim, Summary: text, Branch: *branch, Commit: *commit, Recommendation: domain.Recommendation(*recommendation), Full: *full}
	if *repositories != "" {
		if err := decodeField(ctx, *repositories, "repositories", &in.Repositories); err != nil {
			return err
		}
	}
	v, err := sc.client.Finish(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
