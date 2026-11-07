package notify

import (
	"os"
	"os/exec"
	"runtime"
	"slices"
	"testing"
)

// fakeTools is a machine with the given commands on PATH and the given
// --help outputs. Nothing is ever run.
type fakeTools struct {
	on    []string
	helps map[string]string
}

func (f fakeTools) has(name string) bool    { return slices.Contains(f.on, name) }
func (f fakeTools) help(name string) string { return f.helps[name] }

var single = Notification{Title: "acme-2 has a new Round", Body: `Fix "login" \ logout`, URL: "http://localhost:7733/?view=inbox&ticket=acme-2"}

func TestTheOSCommandForEachPlatform(t *testing.T) {
	withAction := map[string]string{"notify-send": "Usage:\n  -A, --action=[NAME=]Text...  Specifies the actions to display"}
	cases := []struct {
		name  string
		goos  string
		tools fakeTools
		want  command
	}{
		{"macOS with terminal-notifier opens the URL on a click", "darwin", fakeTools{on: []string{"terminal-notifier", "osascript"}},
			command{argv: []string{"terminal-notifier", "-title", single.Title, "-message", single.Body, "-open", single.URL}}},
		{"macOS without it is text only", "darwin", fakeTools{on: []string{"osascript"}},
			command{argv: []string{"osascript", "-e", `display notification "Fix \"login\" \\ logout" with title "acme-2 has a new Round"`}}},
		{"Linux with notify-send --action waits for a click", "linux", fakeTools{on: []string{"notify-send", "xdg-open"}, helps: withAction},
			command{argv: []string{"notify-send", "--app-name=Code Central", "--action=default=Open", "--wait", single.Title, single.Body},
				waitForClick: true, opener: []string{"xdg-open", single.URL}}},
		{"Linux with an older libnotify is text only", "linux", fakeTools{on: []string{"notify-send", "xdg-open"}, helps: map[string]string{"notify-send": "Usage: -u, --urgency"}},
			command{argv: []string{"notify-send", "--app-name=Code Central", single.Title, single.Body}}},
		{"Linux without xdg-open is text only", "linux", fakeTools{on: []string{"notify-send"}, helps: withAction},
			command{argv: []string{"notify-send", "--app-name=Code Central", single.Title, single.Body}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := commandFor(c.goos, c.tools, single)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.argv, c.want.argv) || got.waitForClick != c.want.waitForClick || !slices.Equal(got.opener, c.want.opener) {
				t.Fatalf("got %#v\nwant %#v", got, c.want)
			}
		})
	}
}

func TestTheOSCommandFailsWhereNothingCanShowANotification(t *testing.T) {
	if _, err := commandFor("linux", fakeTools{}, single); err == nil {
		t.Error("linux without notify-send: want an error")
	}
	if _, err := commandFor("windows", fakeTools{}, single); err == nil {
		t.Error("windows: want an error")
	}
}

// The contract test for the real sender: it shows one real Notification,
// so it runs only when asked (CODE_CENTRAL_NOTIFY_CONTRACT=1) and skips
// when no notification daemon is present, as in CI.
func TestTheOSSenderShowsANotification(t *testing.T) {
	if os.Getenv("CODE_CENTRAL_NOTIFY_CONTRACT") != "1" {
		t.Skip("set CODE_CENTRAL_NOTIFY_CONTRACT=1 to show a real OS Notification")
	}
	switch runtime.GOOS {
	case "linux":
		if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
			t.Skip("no D-Bus session bus, so no notification daemon")
		}
		if _, err := exec.LookPath("notify-send"); err != nil {
			t.Skip("notify-send is not on PATH")
		}
	case "darwin":
	default:
		t.Skipf("no notification daemon on %s", runtime.GOOS)
	}
	if err := OS().Send(Notification{Title: "Code Central", Body: "Contract test: click to open Needs Me", URL: "http://localhost:7733/?view=inbox"}); err != nil {
		t.Fatal(err)
	}
}
