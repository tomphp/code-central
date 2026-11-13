// Claude Args (Spec code-central-516): what follows -- on the Factory's
// command line is claude flags the Factory passes, as given, to every
// start and resume. The Factory refuses the ones it sets itself and
// checks at startup that claude accepts the rest.

package factory

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

// checkClaudeArgs refuses a Claude Arg the Factory sets itself, in its
// --flag or --flag=value form, and a bare --.
func checkClaudeArgs(args []string) error {
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

// joinArgs joins args with spaces, quoting any that holds a space.
func joinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a
		if strings.ContainsAny(a, " \t") {
			parts[i] = fmt.Sprintf("%q", a)
		}
	}
	return strings.Join(parts, " ")
}

// checkClaudeArgsAccepted runs `claude -p <Claude Args>` with empty
// stdin and no prompt, which makes no model call and leaves no session:
// claude names an unknown option, or says the input must be provided.
// Anything else only warns.
func (f *factory) checkClaudeArgsAccepted() error {
	if len(f.o.claudeArgs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), helpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.claude, append([]string{"-p"}, f.o.claudeArgs...)...)
	cmd.Dir, cmd.Env = f.repo, f.e.Environ
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
	f.printf("warning: can't tell whether claude accepts the Claude Args (claude -p %s: %v, said %q); carrying on", joinArgs(f.o.claudeArgs), err, text)
	return nil
}
