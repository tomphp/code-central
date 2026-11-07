package cli

import (
	"context"
	"encoding/json"
	"errors"
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

func runSetSession(ctx context.Context, e Env, args []string) error {
	fs := flagSet("set-session")
	claim := fs.String("claim", "", "")
	session := fs.String("session", "", "")
	sc, ref, _, err := itemCommand(e, fs, args, 0, 0)
	if err != nil {
		return err
	}
	v, err := sc.client.SetSession(ctx, sc.project, core.SetSessionInput{Ticket: ref, ClaimID: *claim, SessionID: sessionID(e, *session)})
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

// The hooks init writes into .claude/settings.json. The commands are
// stable across versions; the matchers name the MCP server init writes.
const (
	settingsFile   = ".claude/settings.json"
	sessionHookCmd = "code-central hook session-id"
	pingHookCmd    = "code-central ping"
)

var ourHooks = []struct{ event, matcher, command string }{
	{"PreToolUse", "mcp__" + serverName + "__claim|mcp__" + serverName + "__post_round", sessionHookCmd},
	{"PostToolUse", "*", pingHookCmd},
}

// writeHooks merges Code Central's hooks into root/.claude/settings.json,
// keeping every other setting and hook, and replacing any earlier copy of
// ours so re-running init never duplicates them.
func writeHooks(root string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(settingsFile))
	doc := map[string]any{}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return path, err
	default:
		if err := json.Unmarshal(b, &doc); err != nil {
			return path, fmt.Errorf("%s isn't valid JSON (%v); fix it and run init again", path, err)
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
		hooks[h.event] = append(kept, map[string]any{
			"matcher": h.matcher,
			"hooks":   []any{map[string]any{"type": "command", "command": h.command}},
		})
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return path, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	return path, os.WriteFile(path, append(out, '\n'), 0o644)
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
