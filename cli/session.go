package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/core"
)

// Sessions, ping and the hooks init writes (ticket 31).

// sessionEnv is set by Claude Code in Bash and hook subprocesses.
const sessionEnv = "CLAUDE_CODE_SESSION_ID"

// sessionID is the --session flag's value, else CLAUDE_CODE_SESSION_ID.
func sessionID(e Env, flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return e.Getenv(sessionEnv)
}

// resumeFlags are --working-dir and --resume-command, on claim and
// set-session. A flag left off leaves the stored value alone.
type resumeFlags struct {
	fs                        *flag.FlagSet
	workingDir, resumeCommand *string
}

func addResumeFlags(fs *flag.FlagSet) resumeFlags {
	return resumeFlags{fs: fs, workingDir: fs.String("working-dir", "", ""), resumeCommand: fs.String("resume-command", "", "")}
}

// values are the flags given, nil for each left off. Call after parsing.
func (r resumeFlags) values() (workingDir, resumeCommand *string) {
	r.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "working-dir":
			workingDir = r.workingDir
		case "resume-command":
			resumeCommand = r.resumeCommand
		}
	})
	return workingDir, resumeCommand
}

func runSetSession(ctx context.Context, e Env, args []string) error {
	fs := flagSet("set-session")
	claim := fs.String("claim", "", "")
	session := fs.String("session", "", "")
	resume := addResumeFlags(fs)
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	in := core.SetSessionInput{Ticket: ref, ClaimID: *claim, SessionID: sessionID(e, *session)}
	in.WorkingDir, in.ResumeCommand = resume.values()
	v, err := sc.client.SetSession(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

// pingLimit is how long ping may take before it gives up.
const pingLimit = 300 * time.Millisecond

// hookInput is the part of a Claude Code hook's stdin JSON we read.
type hookInput struct {
	SessionID string         `json:"session_id"`
	Cwd       string         `json:"cwd"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

// runPing marks this session's Claims as active. It runs after every tool
// call, so it never holds one up: it gives up after pingLimit, prints
// nothing and always exits 0.
func runPing(ctx context.Context, e Env, args []string) error {
	after := e.After
	if after == nil {
		after = time.After
	}
	deadline := after(pingLimit)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ping(ctx, e, args)
	}()
	select {
	case <-done:
	case <-deadline:
	}
	return nil
}

func ping(ctx context.Context, e Env, args []string) {
	fs := flagSet("ping")
	session := fs.String("session", "", "")
	if _, err := parse(fs, args); err != nil {
		return
	}
	id := *session
	if id == "" {
		var in hookInput
		if json.NewDecoder(e.Stdin).Decode(&in) == nil {
			id = in.SessionID
		}
	}
	if id = sessionID(e, id); id == "" {
		return
	}
	client.New(serviceURL(e)).Ping(ctx, id)
}

// runHook is `code-central hook <name>`, run by Claude Code hooks. A hook
// never blocks the tool call: every failure exits 0 with no output.
func runHook(_ context.Context, e Env, args []string) error {
	if len(args) != 1 || args[0] != "session-id" {
		return nil
	}
	var in hookInput
	if json.NewDecoder(e.Stdin).Decode(&in) != nil || in.SessionID == "" {
		return nil
	}
	input := in.ToolInput
	if input == nil {
		input = map[string]any{}
	}
	input["session_id"] = in.SessionID
	// claim and set_session also get the session's working directory,
	// unless the agent gave one. post_round takes none, and decodes
	// strictly.
	if _, given := input["working_dir"]; !given && in.Cwd != "" && takesWorkingDir(in.ToolName) {
		input["working_dir"] = in.Cwd
	}
	// updatedInput replaces the whole tool input. No permissionDecision:
	// the call goes through the normal permission flow.
	out := map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PreToolUse",
		"updatedInput":  input,
	}}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	fmt.Fprintf(e.Stdout, "%s\n", b)
	return nil
}

// takesWorkingDir reports whether the MCP tool takes a working_dir: claim
// and set_session.
func takesWorkingDir(tool string) bool {
	return strings.HasSuffix(tool, "__claim") || strings.HasSuffix(tool, "__set_session")
}

// The hooks init writes into .claude/settings.json. The commands are
// stable across versions; the matchers name the MCP server init writes.
const (
	settingsFile      = ".claude/settings.json"
	localSettingsFile = ".claude/settings.local.json"
	sessionHookCmd    = "code-central hook session-id"
	pingHookCmd       = "code-central ping"
)

var ourHooks = []struct{ event, matcher, command string }{
	{"PreToolUse", "mcp__" + serverName + "__claim|mcp__" + serverName + "__set_session|mcp__" + serverName + "__post_round", sessionHookCmd},
	{"PostToolUse", "*", pingHookCmd},
}

// writeHooks merges Code Central's hooks into the settings file at path
// (.claude/settings.json, or settings.local.json for a local Project),
// keeping every other setting and hook, and replacing any earlier copy of
// ours so re-running init never duplicates them. With add false it only
// removes ours, and leaves a missing file missing.
func writeHooks(path string, add bool) error {
	doc := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !add {
			return nil
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s isn't valid JSON (%v); fix it and run init again", path, err)
		}
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		doc["hooks"] = hooks
	}
	for _, h := range ourHooks {
		groups, _ := hooks[h.event].([]any)
		kept := []any{}
		for _, g := range groups {
			if !isOurs(g) {
				kept = append(kept, g)
			}
		}
		if add {
			kept = append(kept, map[string]any{
				"matcher": h.matcher,
				"hooks":   []any{map[string]any{"type": "command", "command": h.command}},
			})
		}
		if len(kept) == 0 {
			delete(hooks, h.event)
		} else {
			hooks[h.event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(doc, "hooks")
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// isOurs reports whether a hook group runs only Code Central's commands.
func isOurs(group any) bool {
	g, _ := group.(map[string]any)
	hs, _ := g["hooks"].([]any)
	if len(hs) == 0 {
		return false
	}
	for _, h := range hs {
		m, _ := h.(map[string]any)
		cmd, _ := m["command"].(string)
		if cmd != sessionHookCmd && cmd != pingHookCmd && !strings.HasPrefix(cmd, "code-central hook ") {
			return false
		}
	}
	return true
}
