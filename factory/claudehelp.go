// The Reported Models (Spec code-central-301): at startup the Factory
// reads from claude --help the effort levels and model aliases its claude
// accepts, checks its own --effort against the levels, and reports both
// to Code Central with its flags.

package factory

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/core"
)

// helpTimeout bounds claude --help.
const helpTimeout = 30 * time.Second

// claudeAccepts is what claude --help says claude accepts; either is
// empty when the help didn't say.
type claudeAccepts struct {
	levels, aliases []string
}

// readClaudeHelp runs claude --help and reads it, warning of what it
// can't find. A claude that can't print its help is warned of too:
// nothing here is fatal.
func (f *factory) readClaudeHelp() claudeAccepts {
	ctx, cancel := context.WithTimeout(context.Background(), helpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.claude, "--help")
	cmd.Dir, cmd.Env = f.repo, f.e.Environ
	out, err := cmd.Output()
	if err != nil {
		f.printf("warning: can't run claude --help (%v), so there are no effort levels or model aliases to report, and --effort isn't checked", err)
		return claudeAccepts{}
	}
	a := parseHelp(string(out))
	if len(a.levels) == 0 {
		f.printf("warning: can't read the effort levels from claude --help, so none are reported, and --effort isn't checked")
	}
	if len(a.aliases) == 0 {
		f.printf("warning: can't read the model aliases from claude --help, so none are reported")
	}
	return a
}

// checkEffortFlag refuses an --effort outside the levels claude accepts.
// Without levels it can't say, so it takes any.
func (f *factory) checkEffortFlag() error {
	levels := f.accepts.levels
	if f.o.effort == "" || len(levels) == 0 || slices.Contains(levels, f.o.effort) {
		return nil
	}
	return fmt.Errorf("--effort %q isn't an effort level claude accepts: use one of %s", f.o.effort, strings.Join(levels, ", "))
}

// reportModels reports what claude accepts with the Factory's flags. It
// reports whether Code Central took it; if not, the next poll tries again.
func (f *factory) reportModels(ctx context.Context) bool {
	in := core.ReportModelsInput{EffortLevels: f.accepts.levels, Aliases: f.accepts.aliases, ModelFlag: f.o.model, EffortFlag: f.o.effort}
	if _, err := f.client.ReportModels(ctx, f.project, in); err != nil {
		if ctx.Err() == nil {
			f.printf("warning: can't report what claude accepts to Code Central: %v", err)
		}
		return false
	}
	return true
}

var (
	// optionLine starts an option's entry in the help, such as
	// "-p, --print" or "--model <model>".
	optionLine = regexp.MustCompile(`^-{1,2}[A-Za-z]`)
	// parenthesised is a parenthesised list in an option's text.
	parenthesised = regexp.MustCompile(`\(([^()]*)\)`)
	// quoted is a name in single or double quotes.
	quoted = regexp.MustCompile(`'([^'\s]+)'|"([^"\s]+)"|‘([^’\s]+)’`)
	// name is an effort level or a model alias.
	name = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\[\]-]*$`)
)

// parseHelp reads the effort levels from the --effort option's
// parenthesised list and the model aliases from the quoted names in the
// --model option's text, wherever the options are and however their text
// wraps.
func parseHelp(help string) claudeAccepts {
	var a claudeAccepts
	for _, m := range parenthesised.FindAllStringSubmatch(optionText(help, "--effort"), -1) {
		if levels := listed(m[1]); len(levels) > 1 {
			a.levels = levels
			break
		}
	}
	for _, m := range quoted.FindAllStringSubmatch(optionText(help, "--model"), -1) {
		alias := m[1] + m[2] + m[3]
		if name.MatchString(alias) && !slices.Contains(a.aliases, alias) {
			a.aliases = append(a.aliases, alias)
		}
	}
	return a
}

// optionText is the text of option's entry in the help, its wrapped lines
// joined: from the line naming it to the next option, blank line or
// heading. It is "" when the help has no such option.
func optionText(help, option string) string {
	var text []string
	for line := range strings.Lines(help) {
		t := strings.TrimSpace(line)
		if text == nil {
			if names(t, option) {
				text = append(text, t)
			}
			continue
		}
		if t == "" || optionLine.MatchString(t) || !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		text = append(text, t)
	}
	return strings.Join(text, " ")
}

// names reports whether the option line t names option, as in
// "--effort <level>" or "-m, --model <model>".
func names(t, option string) bool {
	if !optionLine.MatchString(t) {
		return false
	}
	flags, _, _ := strings.Cut(t, "  ")
	for f := range strings.FieldsFuncSeq(flags, func(r rune) bool { return r == ',' || r == ' ' || r == '=' }) {
		if f == option {
			return true
		}
	}
	return false
}

// listed is the names in a comma-separated list such as "low, medium, or
// high", or nil when any item isn't a name.
func listed(list string) []string {
	var out []string
	for item := range strings.SplitSeq(list, ",") {
		item = strings.TrimSpace(item)
		item = strings.TrimPrefix(strings.TrimPrefix(item, "or "), "and ")
		if !name.MatchString(item) {
			return nil
		}
		out = append(out, item)
	}
	return out
}
