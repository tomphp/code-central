package claude_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/factory/harness"
	"github.com/tomoram/code-central/factory/harness/claude"
)

var defaultAliases = []string{"fable", "opus", "sonnet"}

// warnings collects what a Harness warns of.
type warnings []string

func (w *warnings) add(format string, args ...any) {
	*w = append(*w, fmt.Sprintf(format, args...))
}

func (w warnings) has(want string) bool {
	return slices.ContainsFunc(w, func(s string) bool { return strings.Contains(s, want) })
}

func discover(t *testing.T, help string) (harness.Accepts, warnings) {
	t.Helper()
	tt := newTurnTest(t, options{})
	tt.h.ClaudeHelp(help)
	var w warnings
	return tt.harness.Discover(tt.h.Repo, w.add), w
}

func TestTheClaudeHarnessIsCalledClaude(t *testing.T) {
	t.Parallel()
	if got := newTurnTest(t, options{}).harness.Name(); got != "claude" {
		t.Errorf("name %q, want claude", got)
	}
}

func TestDiscoverReadsTheEffortLevelsAndAliasesOfReWrappedAndReorderedHelp(t *testing.T) {
	t.Parallel()
	for name, help := range map[string]string{
		"reordered and narrow": `Usage: claude [options]

Options:
  --model <model>   Model for the current session.
                    Provide an alias for the
                    latest model (e.g. 'fable',
                    'opus', or 'sonnet') or a
                    model's full name.
  -d, --debug       Enable debug mode
  --effort <level>  Effort level for the
                    current session (low,
                    medium, high, xhigh,
                    max)
`,
		"wide": "Options:\n" +
			"  --effort <level>   Effort level for the current session (low, medium, high, xhigh, max)\n" +
			"  --model <model>    Model for the current session. Provide an alias for the latest model (e.g. \"fable\", \"opus\", or \"sonnet\") or a model's full name.\n",
		"labelled choices and curly quotes": "Options:\n" +
			"  --effort <level>  Effort level (choices: \"low\", \"medium\", \"high\", \"xhigh\", \"max\")\n" +
			"  --model <model>   An alias (“fable”, ‘opus’ or ‘sonnet’) or a full name\n",
		"bars": "Options:\n" +
			"  --model <model>   An alias ('fable', 'opus', 'sonnet')\n" +
			"  --effort <level>  Effort level (low|medium|high|xhigh|max)\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, w := discover(t, help)
			if !slices.Equal(got.Efforts, defaultLevels) || !slices.Equal(got.Aliases, defaultAliases) || len(w) != 0 {
				t.Errorf("found %+v, warning %q; want levels %v and aliases %v, no warning", got, w, defaultLevels, defaultAliases)
			}
		})
	}
}

func TestDiscoverWarnsOfWhatTheHelpDoesntSay(t *testing.T) {
	t.Parallel()
	got, w := discover(t, "Usage: claude [options] [prompt]\n\nOptions:\n  -p, --print   Print response and exit\n")
	if len(got.Efforts) != 0 || len(got.Aliases) != 0 {
		t.Errorf("found %+v in help that lists none", got)
	}
	if !w.has("can't read the effort levels from claude --help") || !w.has("can't read the model aliases from claude --help") {
		t.Errorf("warned %q, want both the levels and the aliases named", w)
	}
}

func TestDiscoverWarnsWhenClaudeCantPrintItsHelp(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	h := claude.New(claude.Options{Path: tt.h.Repo + "/no-such-claude", Environ: tt.h.Environ()})
	var w warnings
	got := h.Discover(tt.h.Repo, w.add)
	if len(got.Efforts) != 0 || len(got.Aliases) != 0 || !w.has("can't run claude --help") {
		t.Errorf("found %+v, warned %q; want nothing found and the failed run named", got, w)
	}
}

func TestCheckArgsRefusesTheFlagsTheFactorySetsItself(t *testing.T) {
	t.Parallel()
	for arg, want := range map[string]string{
		"--model":                        "use the Factory's --model",
		"--model=opus":                   "use the Factory's --model",
		"--effort":                       "use the Factory's --effort",
		"--permission-mode":              "use --bypass-permissions",
		"--permission-prompts":           "use --bypass-permissions",
		"--dangerously-skip-permissions": "use --bypass-permissions",
		"--resume":                       "the Factory sets --resume itself",
		"-p":                             "the Factory sets -p itself",
		"--allowed-tools=Bash":           "the Factory sets --allowed-tools itself",
		"--system-prompt-file":           "the Factory sets --system-prompt-file itself",
		"--no-session-persistence":       "the Factory sets --no-session-persistence itself",
		"--bare":                         "the Factory sets --bare itself",
		"--":                             "ends the flags itself",
	} {
		err := claude.CheckArgs([]string{"--chrome", arg})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want a refusal saying %q", arg, err, want)
		}
	}
}

func TestCheckArgsTakesFlagsAndValuesItDoesntSet(t *testing.T) {
	t.Parallel()
	if err := claude.CheckArgs([]string{"--chrome", "--add-dir", "a", "b", "--append-dir=x"}); err != nil {
		t.Error(err)
	}
}

func verify(t *testing.T, tt *turnTest, args ...string) (warnings, error) {
	t.Helper()
	h := claude.New(claude.Options{Path: tt.h.Claude(), Environ: tt.h.Environ(), Args: args})
	var w warnings
	err := h.VerifyArgs(tt.h.Repo, w.add)
	return w, err
}

func TestVerifyArgsRefusesAnArgClaudeNamesUnknown(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	tt.h.ClaudeRejects("--chrom")
	_, err := verify(t, tt, "--chrom")
	if err == nil || !strings.Contains(err.Error(), "claude doesn't accept the Claude Args: error: unknown option '--chrom'") {
		t.Errorf("got %v, want claude refused for the unknown option", err)
	}
}

func TestVerifyArgsWarnsAndCarriesOnWhenClaudesAnswerIsNotUnderstood(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	tt.h.ClaudeSays("something new")
	w, err := verify(t, tt, "--chrome", "--add-dir", "my dir")
	if err != nil || !w.has("can't tell whether claude accepts the Claude Args") {
		t.Errorf("got %v, warned %q; want no error and a warning", err, w)
	}
}

func TestVerifyArgsAcceptsArgsClaudeTakesWithoutWarning(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	if w, err := verify(t, tt, "--chrome"); err != nil || len(w) != 0 {
		t.Errorf("got %v, warned %q; want neither", err, w)
	}
}

func TestVerifyArgsChecksNothingWithNoArgs(t *testing.T) {
	t.Parallel()
	tt := newTurnTest(t, options{})
	tt.h.ClaudeRejects("--chrom")
	tt.h.ClaudeSays("would warn")
	if w, err := verify(t, tt); err != nil || len(w) != 0 {
		t.Errorf("got %v, warned %q; want neither", err, w)
	}
}
