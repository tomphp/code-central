package factory_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// hasInOrder checks that c has want in a row after the Factory's flags,
// and ends with -- and the prompt.
func hasInOrder(t *testing.T, c factorytest.Call, want ...string) {
	t.Helper()
	got := strings.Join(c.Args, "\x00")
	if !strings.Contains(got, strings.Join(want, "\x00")) {
		t.Errorf("args %q don't have %q in a row", c.Args, want)
	}
	n := len(c.Args)
	if n < 2 || c.Args[n-2] != "--" || c.Args[n-1] != c.Prompt {
		t.Errorf("args %q don't end with -- and the prompt", c.Args)
	}
	if i := strings.Index(got, want[0]); i < strings.Index(got, "--output-format") {
		t.Errorf("args %q put the Claude Args before the Factory's flags", c.Args)
	}
}

func TestClaudeArgsReachEveryStartAndResumeBeforeThePrompt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id, first, r := answeredTicket(t, h, "--", "--chrome", "--add-dir", "a", "b")
	r.Poll()
	h.WaitForState(id, store.InReview)
	calls := h.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d calls, want a start and a resume", len(calls))
	}
	hasInOrder(t, first, "--chrome", "--add-dir", "a", "b")
	hasInOrder(t, calls[1], "--chrome", "--add-dir", "a", "b")
	if !calls[1].Has("--resume") {
		t.Errorf("second call %q isn't a resume", calls[1].Args)
	}
}

func TestTheFactoryRefusesClaudeArgsItSetsItself(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	for _, tc := range [][]string{
		{"--model", "use the Factory's --model"},
		{"--model=opus", "use the Factory's --model"},
		{"--effort", "use the Factory's --effort"},
		{"--permission-mode", "use --bypass-permissions"},
		{"--permission-prompts", "use --bypass-permissions"},
		{"--dangerously-skip-permissions", "use --bypass-permissions"},
		{"--resume", "the Factory sets --resume itself"},
		{"-p", "the Factory sets -p itself"},
		{"--allowed-tools=Bash", "the Factory sets --allowed-tools itself"},
		{"--system-prompt-file", "the Factory sets --system-prompt-file itself"},
		{"--no-session-persistence", "the Factory sets --no-session-persistence itself"},
		{"--bare", "the Factory sets --bare itself"},
		{"--", "ends the flags itself"},
	} {
		refuses(t, h, h.Repo, tc[1], "--", "--chrome", tc[0])
	}
	refuses(t, h, h.Repo, `unexpected argument "stray"`, "stray")
}

func TestTheFactoryRefusesToStartWhenClaudeNamesAClaudeArgUnknown(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.ClaudeRejects("--chrom")
	refuses(t, h, h.Repo, "error: unknown option '--chrom'", "--", "--chrom")
}

func TestTheFactoryWarnsAndCarriesOnWhenTheClaudeArgsCheckIsNotUnderstood(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.ClaudeSays("something new")
	id := h.ReadyTicket("Work")
	r := h.Start("--", "--chrome")
	h.WaitForState(id, store.InReview)
	if out := r.Stderr() + r.Stdout(); !strings.Contains(out, "can't tell whether claude accepts the Claude Args") || !strings.Contains(out, "something new") {
		t.Errorf("no warning with claude's output:\n%s", out)
	}
}

func TestTheClaudeArgsCheckIsNotATurnAndOnlyRunsWithClaudeArgs(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.ClaudeSays("would warn")
	id := h.ReadyTicket("Work")
	r := h.Start()
	h.WaitForState(id, store.InReview)
	if strings.Contains(r.Stderr()+r.Stdout(), "Claude Args") {
		t.Errorf("a Factory with no Claude Args mentioned them:\n%s%s", r.Stderr(), r.Stdout())
	}
	if n := len(h.Calls()); n != 1 {
		t.Errorf("%d calls, want only the turn", n)
	}
}

func TestTheFactoryPrintsTheClaudeArgsItPasses(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	r := h.Start("--", "--chrome", "--append-dir=x", "--add-dir", "my dir")
	r.WaitForOutput(`passing to every claude turn: --chrome --append-dir=x --add-dir "my dir"`)
}

func TestAFailureReportForATurnWithNoResultListsTheClaudeArgs(t *testing.T) {
	t.Parallel()
	for _, withArgs := range []bool{true, false} {
		h := factorytest.New(t)
		id := h.ReadyTicket("Rejected")
		h.Script(id, factorytest.Exit(1))
		var args []string
		if withArgs {
			args = []string{"--", "--chrome"}
		}
		h.Start(args...)
		fr := h.WaitForState(id, store.Failed).FailureReports[0]
		if got := strings.Contains(fr.WhatToFix, "--chrome"); got != withArgs {
			t.Errorf("with Claude Args %v, what-to-fix is %q", withArgs, fr.WhatToFix)
		}
	}
}
