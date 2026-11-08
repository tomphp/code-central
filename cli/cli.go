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
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/store"
)

// Env is everything a command touches outside itself, so tests can run
// commands in-process.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
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
		"init":          {usage: "init [--key <key>] [--local | --committed]  set this repository up as a Project (--local keeps init's files out of git)", run: runInit},
		"create-spec":   {op: "create_spec", usage: "create-spec --title <t> [--body <md> | --file <path|->] [--full]", run: runCreateSpec},
		"create-ticket": {op: "create_ticket", usage: "create-ticket --spec <id> --title <t> [--state Draft|Ready] [--label <l>]... [--blocked-by <id>]... [--description <md> | --file <path|->]", run: runCreateTicket},
		"get-ticket":    {op: "get_ticket", usage: "get-ticket <id>", run: runGetTicket},
		"find-tickets":  {op: "find_tickets", usage: "find-tickets [--state <s>]... [--title <t>] [--label <l>]... [--tag <key>=<value>]... [--spec <id>] [--claimable[=false]]", run: runFindTickets},

		"edit-spec":     {op: "edit_spec", usage: "edit-spec <id> [--title <t>] [--body <md> | --file <path|->] [--full]", run: runEditSpec},
		"edit-ticket":   {op: "edit_ticket", usage: "edit-ticket <id> [--title <t>] [--description <md> | --file <path|->]", run: runEditTicket},
		"add-labels":    {op: "add_labels", usage: "add-labels <id> <label>...", run: runAddLabels},
		"remove-labels": {op: "remove_labels", usage: "remove-labels <id> <label>...", run: runRemoveLabels},
		"set-tags":      {op: "set_tags", usage: "set-tags <id> <key>=<value>...", run: runSetTags},
		"remove-tags":   {op: "remove_tags", usage: "remove-tags <id> <key>...", run: runRemoveTags},
		"comment":       {op: "comment", usage: "comment <id> [--body <md> | --file <path|->]", run: runComment},
		"approve":       {op: "approve", usage: "approve <id>", run: runApprove},
		"defer":         {op: "defer", usage: "defer <id> [--reason <text> | --file <path|->]", run: runDefer},
		"claim":         {op: "claim", usage: "claim [--spec <id> | --ticket <id>] [--session <id>] [--working-dir <dir>] [--resume-command <command>]", run: runClaim},
		"finish":        {op: "finish", usage: "finish <id> --claim <claim-id> [--summary <md> | --file <path|->] [--branch <branch>] [--commit <sha>]", run: runFinish},

		"set-session": {op: "set_session", usage: "set-session <id> --claim <claim-id> [--session <id>] [--working-dir <dir>] [--resume-command <command>]  (session defaults to $CLAUDE_CODE_SESSION_ID)", run: runSetSession},
		"ping":        {usage: "ping [--session <id>]  mark this session active (reads hook JSON on stdin); always exits 0", run: runPing},
		"hook":        {usage: "hook session-id  PreToolUse hook: add session_id and client_version to the tool input", run: runHook},
		// Failure Reports and giving up (ticket 30).
		"report-failure": {op: "report_failure", usage: "report-failure <id> --claim <claim-id> --what-broke <md> --why <md> --what-to-fix <md> --where-work-stands <md> | --file <json-path|->", run: runReportFailure},
		"give-up":        {op: "give_up", usage: "give-up <id> --claim <claim-id> [--note <md> | --file <path|->]", run: runGiveUp},
		"send-back":      {op: "send_back", usage: "send-back <id> [--note <md> | --file <path|->]  Accepted work that can't land (a merge conflict) back to Ready; the note is required", run: runSendBack},
		// Rounds and wait (ticket 29).
		"post-round":      {op: "post_round", usage: "post-round <id> --claim <claim-id> [--session <id>] [--questions <json> | --file <path|->]  Questions as a JSON array of {title, body, recommended?}, on stdin by default", run: runPostRound},
		"pick-up-answers": {op: "pick_up_answers", usage: "pick-up-answers <id> --claim <claim-id>", run: runPickUpAnswers},
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
	for n := range commands {
		names = append(names, n)
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
	project, err := ResolveProject(e.Dir, projectFlag, e.Getenv)
	if err != nil {
		return scope{}, err
	}
	if project == "" {
		return scope{}, usagef("no Project: pass --project <key>, set CODE_CENTRAL_PROJECT, or run `code-central init` in the repository")
	}
	return scope{project: project, client: newClient(e)}, nil
}

// ResolveProject is the Project Key every client of the service resolves:
// projectFlag, else CODE_CENTRAL_PROJECT, else the key in the nearest
// .mcp.json at or above dir. It is "" when none of them names one.
func ResolveProject(dir, projectFlag string, getenv func(string) string) (string, error) {
	if projectFlag != "" {
		return projectFlag, nil
	}
	if p := getenv("CODE_CENTRAL_PROJECT"); p != "" {
		return p, nil
	}
	return nearestKey(dir)
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
	var labels, blockedBy strs
	fs.Var(&labels, "label", "")
	fs.Var(&blockedBy, "blocked-by", "")
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
	in := core.CreateTicketInput{
		Spec: core.Ref(*spec), Title: *title, Description: text, State: store.State(*state), Labels: labels,
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
	claimable := fs.Bool("claimable", false, "")
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
		in.State = append(in.State, store.State(s))
	}
	if in.Tags, err = tagFlags(tags); err != nil {
		return err
	}
	if isSet(fs, "claimable") {
		in.Claimable = claimable
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
	v, err := sc.client.Finish(ctx, sc.project, core.FinishInput{Ticket: core.Ref(pos[0]), ClaimID: *claim, Summary: text, Branch: *branch, Commit: *commit})
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
