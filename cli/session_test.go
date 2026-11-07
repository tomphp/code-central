package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// claimable starts a server with Project acme and Ready Ticket acme-2.
func claimable(t *testing.T) (*codecentraltest.Server, map[string]string) {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	bg := context.Background()
	srv.Client.CreateProject(bg, core.CreateProjectInput{Key: "acme"})
	srv.Client.CreateSpec(bg, "acme", core.CreateSpecInput{Title: "Login"})
	if _, err := srv.Client.CreateTicket(bg, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: "Ready"}); err != nil {
		t.Fatal(err)
	}
	return srv, map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
}

func TestTheSessionIDDefaultsToClaudeCodesAndTheFlagWins(t *testing.T) {
	_, env := claimable(t)
	env["CLAUDE_CODE_SESSION_ID"] = "from-env"
	res := decode[core.ClaimResult](t, run(t, env, "", "claim"))
	if s := res.Ticket.Claims[0].SessionID; s != "from-env" {
		t.Fatalf("claim recorded session %q, want from-env", s)
	}
	v := decode[core.TicketView](t, run(t, env, "", "set-session", "acme-2", "--claim", res.ClaimID, "--session", "from-flag"))
	if s := v.Claims[0].SessionID; s != "from-flag" {
		t.Fatalf("set-session recorded %q, want from-flag", s)
	}
	v = decode[core.TicketView](t, run(t, env, "", "set-session", "acme-2", "--claim", res.ClaimID))
	if s := v.Claims[0].SessionID; s != "from-env" {
		t.Fatalf("set-session recorded %q, want from-env", s)
	}
}

// silentSince reads acme-2's silent_since after the fake clock moves on.
func silentAfter(t *testing.T, srv *codecentraltest.Server, d time.Duration) *time.Time {
	t.Helper()
	srv.Clock.Advance(d)
	v, err := srv.Client.GetTicket(context.Background(), "acme", "2")
	if err != nil {
		t.Fatal(err)
	}
	return v.SilentSince
}

func TestPingMarksActivityFromTheFlagOrTheHooksJSON(t *testing.T) {
	srv, env := claimable(t)
	decode[core.ClaimResult](t, run(t, env, "", "claim", "--session", "s1"))

	srv.Clock.Advance(40 * time.Minute)
	if r := run(t, env, "", "ping", "--session", "s1"); r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Fatalf("ping: %+v", r)
	}
	if s := silentAfter(t, srv, 29*time.Minute); s != nil {
		t.Fatalf("pinged 29 minutes ago, but Silent since %v", s)
	}

	hook := `{"session_id":"s1","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`
	if r := run(t, env, hook, "ping"); r.code != 0 {
		t.Fatalf("ping: %+v", r)
	}
	if s := silentAfter(t, srv, 29*time.Minute); s != nil {
		t.Fatalf("pinged from the hook 29 minutes ago, but Silent since %v", s)
	}
}

func TestPingAlwaysExitsZero(t *testing.T) {
	for name, tc := range map[string]struct {
		env   map[string]string
		stdin string
		args  []string
	}{
		"no service":   {map[string]string{"CODE_CENTRAL_URL": "http://127.0.0.1:1"}, "", []string{"--session", "s1"}},
		"not JSON":     {nil, "nonsense", nil},
		"no session":   {nil, "{}", nil},
		"unknown flag": {nil, "", []string{"--nope"}},
	} {
		t.Run(name, func(t *testing.T) {
			r := run(t, tc.env, tc.stdin, append([]string{"ping"}, tc.args...)...)
			if r.code != 0 || r.stdout != "" || r.stderr != "" {
				t.Fatalf("ping: %+v", r)
			}
		})
	}
}

func TestPingGivesUpAfter300Milliseconds(t *testing.T) {
	arrived := make(chan struct{})
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) // so the server notices the client hanging up
		close(arrived)
		<-r.Context().Done() // never answers until the client gives up
	}))
	defer hung.Close()

	deadline := make(chan time.Time)
	var waited time.Duration
	done := make(chan int)
	go func() {
		done <- cli.Run(context.Background(), []string{"ping", "--session", "s1"}, cli.Env{
			Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard,
			Getenv: func(k string) string { return map[string]string{"CODE_CENTRAL_URL": hung.URL}[k] },
			Dir:    t.TempDir(),
			After:  func(d time.Duration) <-chan time.Time { waited = d; return deadline },
		})
	}()
	<-arrived
	select {
	case code := <-done:
		t.Fatalf("ping returned %d before its time limit", code)
	default:
	}
	close(deadline)
	if code := <-done; code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if waited != 300*time.Millisecond {
		t.Fatalf("time limit %v, want 300ms", waited)
	}

	// A stdin that never ends (no hook, a terminal) is given up on too.
	stdin, w := io.Pipe()
	defer w.Close()
	deadline = make(chan time.Time)
	go func() {
		done <- cli.Run(context.Background(), []string{"ping"}, cli.Env{
			Stdin: stdin, Stdout: io.Discard, Stderr: io.Discard,
			Getenv: func(string) string { return "" }, Dir: t.TempDir(),
			After: func(time.Duration) <-chan time.Time { return deadline },
		})
	}()
	close(deadline)
	if code := <-done; code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}

func TestHookSessionIDAddsTheSessionToTheToolInput(t *testing.T) {
	in := `{"session_id":"abc","hook_event_name":"PreToolUse","tool_name":"mcp__code-central__claim","tool_input":{"spec":"acme-1","session_id":"stale"}}`
	r := run(t, nil, in, "hook", "session-id")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName      string         `json:"hookEventName"`
			UpdatedInput       map[string]any `json:"updatedInput"`
			PermissionDecision *string        `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("%v: %s", err, r.stdout)
	}
	h := out.HookSpecificOutput
	// updatedInput replaces the whole input, so every field is kept; no
	// permission decision is made, so the normal permission flow applies.
	want := map[string]any{"spec": "acme-1", "session_id": "abc"}
	if h.HookEventName != "PreToolUse" || len(h.UpdatedInput) != 2 || h.UpdatedInput["spec"] != want["spec"] ||
		h.UpdatedInput["session_id"] != want["session_id"] || h.PermissionDecision != nil {
		t.Fatalf("hook output %s", r.stdout)
	}
}

func TestHookSessionIDNeverBlocks(t *testing.T) {
	for name, stdin := range map[string]string{
		"not JSON":   "nonsense",
		"no session": `{"tool_input":{"spec":"acme-1"}}`,
		"empty":      "",
	} {
		t.Run(name, func(t *testing.T) {
			if r := run(t, nil, stdin, "hook", "session-id"); r.code != 0 || r.stdout != "" {
				t.Fatalf("hook: %+v", r)
			}
		})
	}
	// No tool_input still gets one holding the session.
	r := run(t, nil, `{"session_id":"abc"}`, "hook", "session-id")
	if r.code != 0 || !strings.Contains(r.stdout, `"updatedInput":{"session_id":"abc"}`) {
		t.Fatalf("hook: %+v", r)
	}
	if r := run(t, nil, "", "hook", "nope"); r.code != 0 {
		t.Fatalf("an unknown hook exits %d, want 0", r.code)
	}
}

// settings reads dir/.claude/settings.json.
func settings(t *testing.T, dir string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("settings.json: %v\n%s", err, b)
	}
	return doc
}

// hookCommands lists event -> matcher -> commands.
func hookCommands(doc map[string]any) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	hooks, _ := doc["hooks"].(map[string]any)
	for event, groups := range hooks {
		out[event] = map[string][]string{}
		for _, g := range groups.([]any) {
			g := g.(map[string]any)
			m, _ := g["matcher"].(string)
			for _, h := range g["hooks"].([]any) {
				out[event][m] = append(out[event][m], h.(map[string]any)["command"].(string))
			}
		}
	}
	return out
}

func TestInitWritesTheHooks(t *testing.T) {
	srv := codecentraltest.NewServer(t)
	dir := gitRepo(t, "api")
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)
	existing := `{"permissions":{"allow":["Bash(ls)"]},"hooks":{"PostToolUse":[{"matcher":"Write","hooks":[{"type":"command","command":"gofmt -w"}]}]}}`
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(existing), 0o644)

	ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
	doc := settings(t, dir)
	want := map[string]map[string][]string{
		"PreToolUse":  {"mcp__code-central__claim|mcp__code-central__post_round": {"code-central hook session-id"}},
		"PostToolUse": {"Write": {"gofmt -w"}, "*": {"code-central ping"}},
	}
	got := hookCommands(doc)
	if b, w := mustJSON(got), mustJSON(want); !bytes.Equal(b, w) {
		t.Fatalf("hooks\n got %s\nwant %s", b, w)
	}
	if allow := doc["permissions"].(map[string]any)["allow"].([]any); len(allow) != 1 {
		t.Fatalf("lost existing settings: %v", doc)
	}

	// Writing them again replaces ours rather than adding copies.
	os.Remove(filepath.Join(dir, ".mcp.json"))
	ok(t, runIn(t, dir, map[string]string{"CODE_CENTRAL_URL": srv.URL}, "\ny\n", "init"))
	if b, w := mustJSON(hookCommands(settings(t, dir))), mustJSON(want); !bytes.Equal(b, w) {
		t.Fatalf("after a second init\n got %s\nwant %s", b, w)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
