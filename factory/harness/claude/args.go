// Claude Args (Spec code-central-516): what follows -- on the Factory's
// command line is claude flags the Factory passes, as given, to every
// start and resume. The Harness refuses the ones the Factory sets itself
// and checks at startup that claude accepts the rest.

package claude

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tomoram/code-central/factory/harness"
)

// reservedFlags maps each flag the Factory sets itself to what to say
// about it; "" is "the Factory sets <flag> itself".
var reservedFlags = map[string]string{
	"--model":                        "use the Factory's --model",
	"--effort":                       "use the Factory's --effort",
	"--permission-mode":              "use --bypass-permissions",
	"--permission-prompts":           "use --bypass-permissions",
	"--dangerously-skip-permissions": "use --bypass-permissions",
	"-p":                             "",
	"--print":                        "",
	"--session-id":                   "",
	"-r":                             "",
	"--resume":                       "",
	"-c":                             "",
	"--continue":                     "",
	"--output-format":                "",
	"--input-format":                 "",
	"--allowedTools":                 "",
	"--allowed-tools":                "",
	"--append-system-prompt":         "",
	"--system-prompt":                "",
	"--system-prompt-file":           "",
	"--no-session-persistence":       "",
	"--bare":                         "",
}

// CheckArgs refuses a Claude Arg the Factory sets itself, in its
// --flag or --flag=value form, and a bare --.
func CheckArgs(args []string) error {
	for _, a := range args {
		if a == "--" {
			return errors.New("a -- among the Claude Args: the Factory ends the flags itself")
		}
		name, _, _ := strings.Cut(a, "=")
		hint, reserved := reservedFlags[name]
		if !reserved {
			continue
		}
		if hint == "" {
			hint = "the Factory sets " + name + " itself"
		}
		return fmt.Errorf("%s can't be a Claude Arg: %s", a, hint)
	}
	return nil
}

// VerifyArgs runs `claude -p <Claude Args>` with empty stdin and no
// prompt, which makes no model call and leaves no session: claude names
// an unknown option, or says the input must be provided. Anything else
// only warns.
func (h *Harness) VerifyArgs(dir string, warnf func(string, ...any)) error {
	if len(h.o.Args) == 0 {
		return nil
	}
	cmd, cancel := h.probe(dir, append([]string{"-p"}, h.o.Args...)...)
	defer cancel()
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	for line := range strings.Lines(text) {
		if strings.Contains(line, "unknown option") {
			return fmt.Errorf("claude doesn't accept the Claude Args: %s", strings.TrimSpace(line))
		}
	}
	if strings.Contains(text, "Input must be provided") {
		return nil
	}
	if err == nil {
		err = errors.New("exit 0")
	}
	warnf("warning: can't tell whether claude accepts the Claude Args (claude -p %s: %v, said %q); carrying on", harness.JoinArgs(h.o.Args), err, text)
	return nil
}
