package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

func TestTheFactoryClaimsAReadyTicketAndRunsOneSessionToInReview(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Add a button")
	h.Script(id,
		factorytest.WriteFile("button.txt", "button\n"),
		factorytest.Git("add", "button.txt"),
		factorytest.Git("commit", "-q", "-m", "Add a button"),
		factorytest.Finish("Added the button."),
		factorytest.Result("success"))
	mainTip := h.Git("rev-parse", "main")

	r := h.Start()
	r.WaitForOutput("base: main")
	tk := h.WaitForState(id, store.InReview)

	calls := h.Calls()
	if len(calls) != 1 {
		t.Fatalf("claude ran %d times, want once: %+v", len(calls), calls)
	}
	c := calls[0]
	if c.Dir != h.Worktree(id) {
		t.Errorf("claude ran in %s, want the worktree %s", c.Dir, h.Worktree(id))
	}
	if !strings.HasPrefix(c.Prompt, "/implement "+id) {
		t.Errorf("prompt %q doesn't start /implement %s", c.Prompt, id)
	}
	if !c.Has("-p") || c.Session == "" {
		t.Errorf("args %q: want -p and --session-id", c.Args)
	}
	claim := tk.Claims[len(tk.Claims)-1]
	if claim.SessionID != c.Session || claim.WorkingDir != h.Worktree(id) || claim.ResumeCommand != "claude --resume "+c.Session {
		t.Errorf("claim = %+v; want session %s, working_dir %s and its resume command", claim, c.Session, h.Worktree(id))
	}
	if b := h.GitIn(h.Worktree(id), "rev-parse", "--abbrev-ref", "HEAD"); b != id {
		t.Errorf("worktree is on %q, want branch %s", b, id)
	}
	if parent := h.Git("rev-parse", id+"^"); parent != mainTip {
		t.Errorf("branch %s starts at %s, want main's tip %s", id, parent, mainTip)
	}
	r.WaitForOutput(id + ` turn ended (implementation): "Add a button": exit 0, success; now In Review; ran `)
}

// onlyCall runs the Factory with args until id is In Review and returns
// the one claude run and the Factory.
func onlyCall(t *testing.T, args ...string) (factorytest.Call, *factorytest.Running) {
	t.Helper()
	h := factorytest.New(t)
	id := h.ReadyTicket("Work")
	r := h.Start(args...)
	h.WaitForState(id, store.InReview)
	calls := h.Calls()
	if len(calls) != 1 {
		t.Fatalf("claude ran %d times, want once", len(calls))
	}
	return calls[0], r
}

func TestEverySessionRunsInAutoModeWithNoPermissionPrompts(t *testing.T) {
	t.Parallel()
	c, r := onlyCall(t)
	if m, _ := c.Flag("--permission-mode"); m != "auto" {
		t.Errorf("--permission-mode %q, want auto", m)
	}
	if p, _ := c.Flag("--permission-prompts"); p != "none" {
		t.Errorf("--permission-prompts %q, want none", p)
	}
	if c.Has("--model") || c.Has("--bare") {
		t.Errorf("args %q: want no --model when unset, and never --bare", c.Args)
	}
	if strings.Contains(r.Stdout(), "bypassPermissions") {
		t.Errorf("a warning without --bypass-permissions: %q", r.Stdout())
	}
}

func TestATurnHasNoTimeTurnOrBudgetLimit(t *testing.T) {
	t.Parallel()
	c, _ := onlyCall(t)
	for _, limit := range []string{"--max-turns", "--max-budget-usd", "--timeout"} {
		if c.Has(limit) {
			t.Errorf("args %q: want no %s", c.Args, limit)
		}
	}
}

func TestBypassPermissionsRunsEverySessionInBypassModeAndWarns(t *testing.T) {
	t.Parallel()
	c, r := onlyCall(t, "--bypass-permissions")
	if m, _ := c.Flag("--permission-mode"); m != "bypassPermissions" {
		t.Errorf("--permission-mode %q, want bypassPermissions", m)
	}
	if !strings.Contains(r.Stdout(), "warning: --bypass-permissions") {
		t.Errorf("no warning line: %q", r.Stdout())
	}
	if c.Has("--bare") {
		t.Errorf("args %q: never --bare", c.Args)
	}
}

func TestModelIsPassedToTheSession(t *testing.T) {
	t.Parallel()
	c, _ := onlyCall(t, "--model", "opus")
	if m, ok := c.Flag("--model"); m != "opus" {
		t.Errorf("--model %q (given %v), want opus", m, ok)
	}
	if c.Has("--bare") {
		t.Errorf("args %q: never --bare", c.Args)
	}
}
