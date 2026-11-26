// Package claude is the Claude Code Harness: it runs a Turn as claude -p
// and reads how it ended from the stream-json claude prints.
package claude

import (
	"context"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/factory/harness"
)

// Options are what every Turn of claude is run with.
type Options struct {
	// Path is the claude to run.
	Path    string
	Environ []string
	Stderr  io.Writer
	// Clock times Session Events and dates model responses and Plan
	// Limits; nil is the system clock.
	Clock clock.Clock
	// Bypass runs Turns with permissions bypassed rather than in auto mode.
	Bypass bool
	// Args are the Claude Args, passed after the Factory's own flags.
	Args []string
}

// Harness runs Turns of claude.
type Harness struct {
	o Options
	// efforts are the effort levels Discover found in claude --help;
	// none accepts any.
	efforts []string
}

var _ harness.Harness = (*Harness)(nil)

// New is the Harness of claude as o says.
func New(o Options) *Harness {
	if o.Clock == nil {
		o.Clock = clock.System{}
	}
	return &Harness{o: o}
}

// Name is claude.
func (h *Harness) Name() string { return "claude" }

// MCPConfig is .mcp.json, where claude finds the Code Central MCP server.
func (*Harness) MCPConfig() string { return ".mcp.json" }

// Capabilities: claude reports Plan Limits in its rate_limit_events, and
// its Skill tool calls say whether code-review ran.
func (h *Harness) Capabilities() harness.Capabilities {
	return harness.Capabilities{PlanLimits: true, ReviewDetection: true}
}

// Run runs t as claude -p. An effort claude doesn't accept starts
// nothing.
func (h *Harness) Run(ctx context.Context, t harness.Turn) harness.Outcome {
	if t.Effort != "" && len(h.efforts) > 0 && !slices.Contains(h.efforts, t.Effort) {
		return harness.Outcome{Kind: harness.EffortRejected, Efforts: h.efforts}
	}
	t = t.WithDefaults()
	t.BeforeStart()
	cmd := exec.Command(h.o.Path, h.args(t)...)
	cmd.Dir = t.Dir
	cmd.Env = h.o.Environ
	cmd.Stderr = h.o.Stderr
	var ended end
	exit, err := t.Exec(ctx, cmd, func(stdout io.Reader) {
		ended = readStream(stdout, t, h.o.Clock.Now)
	})
	if err != nil {
		t.Linef("can't start claude: %v", err)
		return harness.Outcome{Kind: harness.Failed, StartErr: err}
	}
	ended.exit = exit
	return ended.outcome(ctx.Err() != nil)
}

// args are claude's args for t: -p, the session to start or resume, the
// flags, then the prompt.
func (h *Harness) args(t harness.Turn) []string {
	session := "--session-id"
	if t.Resume {
		session = "--resume"
	}
	args := append([]string{"-p", session, t.Session}, h.flags(t)...)
	return append(args, t.Prompt)
}

// flags are the output format, the permission mode, the Turn's model and
// effort when set, and its allowed tools, system prompt and added folders
// when there are any, then the Claude Args and a -- that ends the flags,
// so that a flag taking several values, as --add-dir does, can't swallow
// the prompt that follows.
func (h *Harness) flags(t harness.Turn) []string {
	args := []string{"--output-format", "stream-json", "--verbose"}
	if h.o.Bypass {
		args = append(args, "--permission-mode", "bypassPermissions")
	} else {
		args = append(args, "--permission-mode", "auto", "--permission-prompts", "none")
	}
	if t.Model != "" {
		args = append(args, "--model", t.Model)
	}
	if t.Effort != "" {
		args = append(args, "--effort", t.Effort)
	}
	if len(t.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(t.AllowedTools, ","))
	}
	if t.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", t.SystemPrompt)
	}
	for _, d := range t.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = append(args, h.o.Args...)
	return append(args, "--")
}
