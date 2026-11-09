package notify

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

// fakeTools is a machine with the given commands on PATH, the given
// --help outputs and, on Windows, maybe the Start-menu shortcut. Nothing
// is ever run.
type fakeTools struct {
	on       []string
	helps    map[string]string
	shortcut bool
}

func (f fakeTools) has(name string) bool    { return slices.Contains(f.on, name) }
func (f fakeTools) help(name string) string { return f.helps[name] }
func (f fakeTools) hasShortcut() bool       { return f.shortcut }

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
		t.Error("windows without powershell: want an error")
	}
	if _, err := commandFor("plan9", fakeTools{}, single); err == nil {
		t.Error("plan9: want an error")
	}
}

// decodedScript is the PowerShell script a Windows command runs, decoded
// from its -EncodedCommand.
func decodedScript(t *testing.T, c command) string {
	t.Helper()
	if len(c.argv) != 5 || !slices.Equal(c.argv[:4], []string{"powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand"}) || c.waitForClick {
		t.Fatalf("argv %q, waitForClick %v; want powershell -EncodedCommand <script>, not waiting", c.argv, c.waitForClick)
	}
	b, err := base64.StdEncoding.DecodeString(c.argv[4])
	if err != nil || len(b)%2 != 0 {
		t.Fatalf("not base64 UTF-16: %v", err)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// toastLines are the script's lines that load the toast XML and show it.
func toastLines(t *testing.T, c command) (load, show string) {
	t.Helper()
	for _, l := range strings.Split(decodedScript(t, c), "\n") {
		switch {
		case strings.HasPrefix(l, "$xml.LoadXml("):
			load = l
		case strings.Contains(l, "CreateToastNotifier("):
			show = l
		}
	}
	return load, show
}

func TestTheWindowsToast(t *testing.T) {
	text := Notification{Title: "acme-2 has a new Round", Body: "Task A"}
	cases := []struct {
		name     string
		n        Notification
		shortcut bool
		load     string
		appID    string
	}{
		{"with the Start-menu shortcut it shows as Code Central", text, true,
			`$xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text>acme-2 has a new Round</text><text>Task A</text></binding></visual></toast>')`,
			"CodeCentral.Service"},
		{"without it, as PowerShell", text, false,
			`$xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text>acme-2 has a new Round</text><text>Task A</text></binding></visual></toast>')`,
			`{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`},
		{"with a URL a click opens it", single, true,
			`$xml.LoadXml('<toast activationType="protocol" launch="http://localhost:7733/?view=inbox&amp;ticket=acme-2"><visual><binding template="ToastGeneric">` +
				`<text>acme-2 has a new Round</text><text>Fix &quot;login&quot; \ logout</text></binding></visual></toast>')`,
			"CodeCentral.Service"},
		// No quote of any kind reaches PowerShell's string: XML references
		// stand for quotes, markup, line breaks and everything beyond ASCII.
		{"XML and quote characters are escaped", Notification{Title: `<b>'it's' & "so"</b>`, Body: "it’s ‘quoted’\nnext\x01line"}, false,
			`$xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text>&lt;b&gt;&apos;it&apos;s&apos; &amp; &quot;so&quot;&lt;/b&gt;</text>` +
				`<text>it&#8217;s &#8216;quoted&#8217;&#10;next&#65533;line</text></binding></visual></toast>')`,
			`{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := commandFor("windows", fakeTools{on: []string{"powershell"}, shortcut: c.shortcut}, c.n)
			if err != nil {
				t.Fatal(err)
			}
			load, show := toastLines(t, got)
			if load != c.load {
				t.Errorf("toast XML\n%s\nwant\n%s", load, c.load)
			}
			if want := "::CreateToastNotifier('" + c.appID + "')"; !strings.Contains(show, want) {
				t.Errorf("show line %q lacks %q", show, want)
			}
		})
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
	case "darwin", "windows":
	default:
		t.Skipf("no notification daemon on %s", runtime.GOOS)
	}
	if err := OS().Send(Notification{Title: "Code Central", Body: "Contract test: click to open Needs Me", URL: "http://localhost:7733/?view=inbox"}); err != nil {
		t.Fatal(err)
	}
}
