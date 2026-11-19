package factory_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/factory/factorytest"
)

// reported is the Project's Reported Models, read as the human.
func reported(t *testing.T, h *factorytest.Harness) api.ReportedModelsView {
	t.Helper()
	p, err := h.Human().GetProject(context.Background(), h.Project)
	if err != nil {
		t.Fatal(err)
	}
	return p.ReportedModels
}

// wantReport fails the test unless the Factory has made its report_models
// (a test settles the Factory first), and returns it.
func wantReport(t *testing.T, h *factorytest.Harness) api.ReportedModelsView {
	t.Helper()
	got := reported(t, h)
	if got.Reported == nil {
		t.Fatal("the Factory hasn't reported what its claude accepts")
	}
	return got
}

// wantAccepts checks the Reported Models hold levels, aliases and flags.
func wantAccepts(t *testing.T, got api.ReportedModelsView, levels, aliases []string, modelFlag, effortFlag string) {
	t.Helper()
	if !slices.Equal(got.EffortLevels, levels) || !slices.Equal(got.Aliases, aliases) || got.ModelFlag != modelFlag || got.EffortFlag != effortFlag {
		t.Errorf("reported %+v, want levels %v, aliases %v, --model %q, --effort %q", got, levels, aliases, modelFlag, effortFlag)
	}
}

var (
	claudeLevels  = []string{"low", "medium", "high", "xhigh", "max"}
	claudeAliases = []string{"fable", "opus", "sonnet"}
)

func TestAtStartupTheFactoryReportsWhatItsClaudeAcceptsWithItsFlags(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start("--model", "opus", "--effort", "high")
	wantAccepts(t, wantReport(t, h), claudeLevels, claudeAliases, "opus", "high")
}

func TestTheFactoryReadsReWrappedAndReorderedHelp(t *testing.T) {
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
			h := factorytest.New(t)
			h.ClaudeHelp(help)
			h.Start()
			wantAccepts(t, wantReport(t, h), claudeLevels, claudeAliases, "", "")
		})
	}
}

func TestHelpTheFactoryCantReadWarnsAndReportsNoLevelsOrAliases(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	// What an earlier Factory found is kept.
	if _, err := h.Server.Client.ReportModels(context.Background(), h.Project, api.ReportModelsInput{EffortLevels: []string{"low", "high"}, Aliases: []string{"opus"}}); err != nil {
		t.Fatal(err)
	}
	h.ClaudeHelp("Usage: claude [options] [prompt]\n\nOptions:\n  -p, --print   Print response and exit\n")

	// With nothing to check it against, any --effort is taken.
	r := h.Start("--model", "sonnet", "--effort", "extreme")
	r.WantOutput("warning: can't read the effort levels from claude --help")
	r.WantOutput("warning: can't read the model aliases from claude --help")
	wantAccepts(t, wantReport(t, h), []string{"low", "high"}, []string{"opus"}, "sonnet", "extreme")
}

func TestTheFactoryRefusesToStartWithAnEffortItsClaudeDoesntAccept(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	refuses(t, h, h.Repo, `--effort "extreme" isn't an effort level claude accepts: use one of low, medium, high, xhigh, max`, "--effort", "extreme")
	if r := reported(t, h); r.Reported != nil {
		t.Errorf("a refused Factory reported %+v", r)
	}
}

func TestAProjectUnreadableAtStartupIsReportedOnTheFirstPollThatCanReadIt(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.SetFailing("/api/projects/" + h.Project)
	r := h.Start("--model", "opus")
	r.Poll()
	if got := reported(t, h); got.Reported != nil {
		t.Fatalf("reported %+v while the Project couldn't be read", got)
	}

	h.SetFailing("")
	r.Poll()
	wantAccepts(t, wantReport(t, h), claudeLevels, claudeAliases, "opus", "")
	if n := strings.Count(r.Stdout(), "warning: can't report"); n != 0 {
		t.Errorf("warned of a failed report: %s", r.Stdout())
	}
}
