package notify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OS returns the Sender for this machine. On macOS it uses
// terminal-notifier when it is on PATH (a click opens the URL), else
// osascript (text only). On Linux it uses notify-send, over D-Bus, with
// --action where libnotify supports it (0.7.9+) so a click opens the URL
// through xdg-open, else text only.
func OS() Sender {
	return osSender{goos: runtime.GOOS, tools: systemTools{}}
}

// tools is what the OS sender asks of the machine; tests replace it.
type tools interface {
	has(name string) bool
	// help is a command's --help output, or "" if it fails.
	help(name string) string
}

type systemTools struct{}

func (systemTools) has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (systemTools) help(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, "--help").CombinedOutput()
	return string(out)
}

// command is how to show one Notification: run argv; when waitForClick,
// argv waits for the Notification to close and prints "default" if it was
// clicked, and then opener opens the URL.
type command struct {
	argv         []string
	waitForClick bool
	opener       []string
}

// commandFor chooses the command for n on goos.
func commandFor(goos string, t tools, n Notification) (command, error) {
	switch goos {
	case "darwin":
		if t.has("terminal-notifier") {
			argv := []string{"terminal-notifier", "-title", n.Title, "-message", n.Body}
			if n.URL != "" {
				argv = append(argv, "-open", n.URL)
			}
			return command{argv: argv}, nil
		}
		script := fmt.Sprintf("display notification %s with title %s", appleString(n.Body), appleString(n.Title))
		return command{argv: []string{"osascript", "-e", script}}, nil
	case "linux":
		if !t.has("notify-send") {
			return command{}, errors.New("notify-send is not on PATH (install libnotify)")
		}
		argv := []string{"notify-send", "--app-name=Code Central"}
		if n.URL != "" && t.has("xdg-open") && strings.Contains(t.help("notify-send"), "--action") {
			argv = append(argv, "--action=default=Open", "--wait", n.Title, n.Body)
			return command{argv: argv, waitForClick: true, opener: []string{"xdg-open", n.URL}}, nil
		}
		return command{argv: append(argv, n.Title, n.Body)}, nil
	}
	return command{}, fmt.Errorf("notifications are not supported on %s", goos)
}

// appleString quotes s as an AppleScript string literal.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

type osSender struct {
	goos  string
	tools tools
}

func (s osSender) Send(n Notification) error {
	c, err := commandFor(s.goos, s.tools, n)
	if err != nil {
		return err
	}
	if c.waitForClick {
		cmd := exec.Command(c.argv[0], c.argv[1:]...)
		var out strings.Builder
		cmd.Stdout = &out
		if err := cmd.Start(); err != nil {
			return err
		}
		// Waiting for the human must not hold notify up.
		go func() {
			if cmd.Wait() == nil && strings.TrimSpace(out.String()) == "default" {
				exec.Command(c.opener[0], c.opener[1:]...).Run()
			}
		}()
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, c.argv[0], c.argv[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v: %s", c.argv[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
