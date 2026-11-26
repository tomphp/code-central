// What claude accepts (Spec code-central-301): the effort levels and
// model aliases its --help lists.

package claude

import (
	"context"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/factory/harness"
)

// helpTimeout bounds claude --help. It is on real time, not the Factory's
// clock: it bounds a process, and no test moves the clock while one
// hangs.
const helpTimeout = 30 * time.Second

// Discover runs claude --help in dir and reads the effort levels and
// model aliases from it, warning of what it can't find. A claude that
// can't print its help is warned of too: nothing here is fatal.
func (h *Harness) Discover(dir string, warnf func(string, ...any)) harness.Accepts {
	ctx, cancel := context.WithTimeout(context.Background(), helpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.o.Path, "--help")
	cmd.Dir, cmd.Env = dir, h.o.Environ
	out, err := cmd.Output()
	if err != nil {
		warnf("warning: can't run claude --help (%v), so there are no effort levels or model aliases to report, and --effort isn't checked", err)
		return harness.Accepts{}
	}
	a := parseHelp(string(out))
	if len(a.Efforts) == 0 {
		warnf("warning: can't read the effort levels from claude --help, so none are reported, and --effort isn't checked")
	}
	if len(a.Aliases) == 0 {
		warnf("warning: can't read the model aliases from claude --help, so none are reported")
	}
	h.efforts = a.Efforts
	return a
}

var (
	// optionLine starts an option's entry in the help, such as
	// "-p, --print" or "--model <model>".
	optionLine = regexp.MustCompile(`^-{1,2}[A-Za-z]`)
	// parenthesised is a parenthesised list in an option's text.
	parenthesised = regexp.MustCompile(`\(([^()]*)\)`)
	// quoted is a name in straight or curly, single or double quotes.
	quoted = regexp.MustCompile(`['"‘“]([^'"’”\s]+)['"’”]`)
	// listedName is an effort level or a model alias.
	listedName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\[\]-]*$`)
	// listLabel leads a list, as in "(choices: …)" or "(one of: …)".
	listLabel = regexp.MustCompile(`^[^,|:]*:\s*`)
)

// parseHelp reads the effort levels from the --effort option's
// parenthesised list and the model aliases from the quoted names in the
// --model option's text, wherever the options are and however their text
// wraps.
func parseHelp(help string) harness.Accepts {
	var a harness.Accepts
	for _, m := range parenthesised.FindAllStringSubmatch(optionText(help, "--effort"), -1) {
		if levels := listed(m[1]); len(levels) > 1 {
			a.Efforts = levels
			break
		}
	}
	for _, m := range quoted.FindAllStringSubmatch(optionText(help, "--model"), -1) {
		alias := m[1]
		if listedName.MatchString(alias) && !slices.Contains(a.Aliases, alias) {
			a.Aliases = append(a.Aliases, alias)
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

// listed is the names in a list separated by commas or bars, such as
// "low, medium, or high", "choices: "low", "high"" or "low|high", or nil
// when any item isn't a name.
func listed(list string) []string {
	var out []string
	list = listLabel.ReplaceAllString(strings.TrimSpace(list), "")
	for item := range strings.FieldsFuncSeq(list, func(r rune) bool { return r == ',' || r == '|' }) {
		item = strings.TrimSpace(item)
		item = strings.TrimPrefix(strings.TrimPrefix(item, "or "), "and ")
		item = strings.Trim(item, `'"‘’“”`)
		if !listedName.MatchString(item) {
			return nil
		}
		out = append(out, item)
	}
	return out
}
