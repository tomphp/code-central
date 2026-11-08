package notify

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tomoram/code-central/home"
)

// AppID is the AUMID Windows toasts show as. service install writes
// ShortcutName into the Start menu carrying it, which is what makes
// Windows show the toasts as Code Central.
const (
	AppID        = "CodeCentral.Service"
	ShortcutName = "Code Central.lnk"
)

// powerShellAppID is Windows PowerShell's own AUMID: toasts fall back to
// it, shown as Windows PowerShell, when the shortcut isn't there.
const powerShellAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// OS returns the Sender for this machine. On macOS it uses
// terminal-notifier when it is on PATH (a click opens the URL), else
// osascript (text only). On Linux it uses notify-send, over D-Bus, with
// --action where libnotify supports it (0.7.9+) so a click opens the URL
// through xdg-open, else text only. On Windows it shows a toast through
// Windows PowerShell; a click opens the URL.
func OS() Sender {
	return osSender{goos: runtime.GOOS, tools: systemTools{}}
}

// tools is what the OS sender asks of the machine; tests replace it.
type tools interface {
	has(name string) bool
	// help is a command's --help output, or "" if it fails.
	help(name string) string
	// hasShortcut reports whether ShortcutName is in the Start menu.
	hasShortcut() bool
}

type systemTools struct{}

func (systemTools) hasShortcut() bool {
	p, err := home.Lookup{Getenv: os.Getenv, UserHomeDir: os.UserHomeDir}.StartMenu(ShortcutName)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

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
	case "windows":
		if !t.has("powershell") {
			return command{}, errors.New("powershell is not on PATH")
		}
		appID := powerShellAppID
		if t.hasShortcut() {
			appID = AppID
		}
		return command{argv: []string{"powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedCommand(toastScript(appID, n))}}, nil
	}
	return command{}, fmt.Errorf("notifications are not supported on %s", goos)
}

// toastScript shows n as a WinRT toast from appID, through Windows
// PowerShell's built-in Windows.UI.Notifications; it needs no module.
// With a URL, a click opens it (protocol activation), so nothing waits.
// The XML is in a single-quoted string, which toastText leaves no quote
// of any kind to end.
func toastScript(appID string, n Notification) string {
	toast := "<toast>"
	if n.URL != "" {
		toast = `<toast activationType="protocol" launch="` + toastText(n.URL) + `">`
	}
	xml := toast + `<visual><binding template="ToastGeneric"><text>` + toastText(n.Title) + "</text><text>" +
		toastText(n.Body) + "</text></binding></visual></toast>"
	return strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null",
		"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null",
		"$xml = New-Object Windows.Data.Xml.Dom.XmlDocument",
		"$xml.LoadXml('" + xml + "')",
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('" + appID + "').Show([Windows.UI.Notifications.ToastNotification]::new($xml))",
	}, "\n")
}

// toastText escapes s as XML text or an attribute value, leaving only
// printable ASCII: markup, quotes, line breaks and every other rune
// become references, and a rune XML can't hold becomes U+FFFD.
func toastText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\'':
			b.WriteString("&apos;")
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		default:
			if !xmlChar(r) {
				r = utf8.RuneError
			}
			b.WriteString("&#" + strconv.Itoa(int(r)) + ";")
		}
	}
	return b.String()
}

// xmlChar reports whether XML 1.0 can hold r.
func xmlChar(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xd7ff || r >= 0xe000 && r <= 0xfffd || r >= 0x10000 && r <= 0x10ffff
}

// encodedCommand is script as PowerShell's -EncodedCommand takes it:
// base64 of UTF-16LE. The script never touches the command line's
// quoting.
func encodedCommand(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
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
