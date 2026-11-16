// The fakes the test binary runs as: the fake claude following its
// Script, the fake git, and HoldSession's process.

package factorytest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tomoram/code-central/cli"
)

func runFake(data string, args []string) int {
	switch name := strings.TrimSuffix(filepath.Base(args[0]), exeSuffix); name {
	case "claude":
		return runFakeClaude(data, args[1:])
	case "git":
		return runFakeGit(data, args[1:])
	case holdName:
		// Long enough for any test; HoldSession's release kills it sooner.
		time.Sleep(time.Minute)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "factorytest: %s=%s but run as %q, not a fake\n", fakeEnv, data, name)
		return 70
	}
}

var (
	ticketLine = regexp.MustCompile(`(?m)^Ticket: (\S+)$`)
	claimLine  = regexp.MustCompile(`(?m)^Claim: (\S+)$`)
)

// runFakeGit is the fake git: it reports the version FakeGit set and runs
// the real git for everything else.
func runFakeGit(data string, args []string) int {
	var g fakeGit
	b, err := os.ReadFile(filepath.Join(data, "git.json"))
	if err == nil {
		err = json.Unmarshal(b, &g)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake git:", err)
		return 70
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println("git version " + g.Version)
		return 0
	}
	cmd := exec.Command(g.Git, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "fake git:", err)
		return 70
	}
	return 0
}

// startupCheck answers the Factory's check of its Claude Args, a -p run
// with no session flag and no prompt, as real claude does with empty
// stdin: an unknown option for one a test named with ClaudeRejects, else
// what ClaudeSays set, else that the input must be provided. It isn't a
// Call. ok is false when args are a turn's.
func startupCheck(data string, args []string) (code int, ok bool) {
	if len(args) == 0 || args[0] != "-p" {
		return 0, false
	}
	for _, a := range args {
		if a == "--session-id" || a == "--resume" {
			return 0, false
		}
	}
	if b, err := os.ReadFile(filepath.Join(data, "rejects.txt")); err == nil {
		for _, bad := range strings.Fields(string(b)) {
			if slices.Contains(args, bad) {
				fmt.Fprintf(os.Stderr, "error: unknown option '%s'\n(Did you mean --chrome?)\n", bad)
				return 1, true
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(data, "says.txt")); err == nil {
		os.Stdout.Write(b)
		return 0, true
	}
	fmt.Fprintln(os.Stderr, "Error: Input must be provided either through stdin or as a prompt argument when using --print")
	return 1, true
}

// runFakeClaude is the fake claude: claude [flags] <prompt>, or claude
// --help, which prints the help ClaudeHelp set, else DefaultHelp, and
// isn't a Call.
func runFakeClaude(data string, args []string) int {
	if slices.Equal(args, []string{"--help"}) {
		help, err := os.ReadFile(filepath.Join(data, "help.txt"))
		if os.IsNotExist(err) {
			help, err = []byte(DefaultHelp), nil
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "fake claude:", err)
			return 70
		}
		os.Stdout.Write(help)
		return 0
	}
	if code, ok := startupCheck(data, args); ok {
		return code
	}
	wd, _ := os.Getwd()
	c := Call{Args: args, Dir: wd}
	if len(args) > 0 {
		c.Prompt = args[len(args)-1]
	}
	c.SystemPrompt, _ = c.Flag("--append-system-prompt")
	for _, text := range []string{c.SystemPrompt, c.Prompt} {
		if m := ticketLine.FindStringSubmatch(text); m != nil {
			c.Ticket = m[1]
		}
		if m := claimLine.FindStringSubmatch(text); m != nil {
			c.Claim = m[1]
		}
	}
	if s, ok := c.Flag("--session-id"); ok {
		c.Session = s
	} else if s, ok := c.Flag("--resume"); ok {
		c.Session = s
	}
	// A Chat turn's prompts name no Ticket, so its runs go by Chat.
	key, steps := c.Ticket, DefaultScript
	if key == "" {
		key, steps = Chat, DefaultChatScript
	}
	if b, err := os.ReadFile(filepath.Join(data, "scripts", key+".json")); err == nil {
		steps = nil
		if err := json.Unmarshal(b, &steps); err != nil {
			fmt.Fprintln(os.Stderr, "fake claude:", err)
			return 70
		}
	}
	// A test stops a waiting run as soon as it sees the call, so take the
	// stop signals before recording it: none is lost before Wait starts.
	sigs := make(chan os.Signal, 2)
	if slices.ContainsFunc(steps, func(s Step) bool { return s.Wait }) {
		signal.Notify(sigs, waitSignals...)
		defer signal.Stop(sigs)
	}
	b, _ := json.Marshal(c)
	name := fmt.Sprintf("%020d-%d.json", time.Now().UnixNano(), os.Getpid())
	// Write beside calls/ and rename in, so Calls never reads half a file.
	tmp := filepath.Join(data, name)
	err := os.WriteFile(tmp, b, 0o644)
	if err == nil {
		err = os.Rename(tmp, filepath.Join(data, "calls", name))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake claude:", err)
		return 70
	}

	fill := strings.NewReplacer("{{ticket}}", c.Ticket, "{{claim}}", c.Claim, "{{session}}", c.Session).Replace
	fillAll := func(args []string) []string {
		var filled []string
		for _, a := range args {
			filled = append(filled, fill(a))
		}
		return filled
	}
	fmt.Println(`{"type":"system","subtype":"init","session_id":"` + c.Session + `"}`)
	noted := 0 // the stop signals the run has noted
	for _, s := range steps {
		dir, err := stepDir(s, c)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fake claude:", err)
			return 70
		}
		switch {
		case s.Exit != nil:
			return *s.Exit
		case s.Wait:
			if code, stop := wait(data, key, sigs, &noted); stop {
				return code
			}
		case s.Stdout != "":
			fmt.Println(fill(s.Stdout))
		case s.Write != nil:
			if err := os.WriteFile(filepath.Join(dir, s.Write.Name), []byte(fill(s.Write.Content)), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "fake claude:", err)
				return 70
			}
		case s.Git != nil:
			filled := fillAll(s.Git)
			cmd := exec.Command("git", filled...)
			cmd.Dir = dir
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "fake claude: git %s: %v\n", strings.Join(filled, " "), err)
				return 70
			}
		case s.Need != "":
			if _, err := os.Stat(filepath.Join(dir, s.Need)); err != nil {
				fmt.Fprintln(os.Stderr, "fake claude: need", s.Need+":", err)
				return 70
			}
		case s.CLI != nil:
			filled := fillAll(s.CLI)
			code := cli.Run(context.Background(), filled, cli.Env{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: os.Stderr, Getenv: os.Getenv})
			if code != 0 {
				fmt.Fprintf(os.Stderr, "fake claude: code-central %s exited %d\n", strings.Join(filled, " "), code)
				return 70
			}
		}
	}
	return 0
}

// stepDir is where s runs in the run c: the worktree of the Repository
// s names, the one of c's --add-dir folders at <Name>.worktrees/<ticket
// or chat>, else "", the run's working directory.
func stepDir(s Step, c Call) (string, error) {
	if s.In == "" {
		return "", nil
	}
	for _, d := range c.Values("--add-dir") {
		if filepath.Base(filepath.Dir(d)) == s.In+".worktrees" {
			return d, nil
		}
	}
	return "", fmt.Errorf("no --add-dir is Repository %s's worktree: %q", s.In, c.Values("--add-dir"))
}

// wait is the fake's Wait step: it returns when the test continues the
// run, or with stop set when a signal on sigs ends it, counting each
// signal it notes in noted. While it waits it keeps the marker
// waiting/<key> (Settle's), holding noted, and removes it before it
// leaves: before it takes the continue file, so the marker is gone once
// Continue returns.
func wait(data, key string, sigs <-chan os.Signal, noted *int) (code int, stop bool) {
	note := func(s string) {
		f, err := os.OpenFile(filepath.Join(data, "signals", key), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, s)
			f.Close()
		}
	}
	marker := filepath.Join(data, "waiting", key)
	mark := func() {
		whileInUse(func() error { return os.WriteFile(marker, []byte(strconv.Itoa(*noted)), 0o644) })
	}
	unmark := func() {
		whileInUse(func() error {
			if err := os.Remove(marker); !os.IsNotExist(err) {
				return err
			}
			return nil
		})
	}
	cont := filepath.Join(data, "continue", key)
	mark()
	for {
		select {
		case s := <-sigs:
			name, ends := signalName(s)
			note(name)
			*noted++
			if ends {
				unmark()
				return 143, true
			}
			mark()
		case <-time.After(5 * time.Millisecond):
			if _, err := os.Stat(cont); err != nil {
				continue
			}
			unmark()
			if os.Remove(cont) == nil {
				return 0, false
			}
			mark()
		}
	}
}

// whileInUse runs do again for up to 2s while it fails on a file another
// process holds open, as the harness reading a marker does on Windows.
func whileInUse(do func() error) {
	deadline := time.Now().Add(2 * time.Second)
	for err := do(); inUse(err) && time.Now().Before(deadline); err = do() {
		time.Sleep(10 * time.Millisecond)
	}
}
