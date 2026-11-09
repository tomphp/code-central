package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/store"
)

func init() {
	commands["service"] = command{
		usage: "service install [--store <dir>] [--bind <ip>] | service restart  run serve as a login item " +
			"(launchd on macOS, systemd user unit on Linux, a scheduled task on Windows); install again after an upgrade",
		run: runService,
	}
}

// The launchd label, the systemd unit name and the scheduled task's name.
const (
	launchdLabel  = "com.github.tomoram.code-central"
	systemdUnit   = "code-central.service"
	scheduledTask = "Code Central"
)

func runService(ctx context.Context, e Env, args []string) error {
	if len(args) == 0 {
		return usagef("expected install or restart")
	}
	m, err := serviceManagerFor(e)
	if err != nil {
		return err
	}
	switch args[0] {
	case "install":
		return serviceInstall(ctx, e, m, args[1:])
	case "restart":
		if len(args) > 1 {
			return usagef("unexpected argument %q", args[1])
		}
		if _, err := os.Stat(m.unitPath); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("Code Central isn't installed as a service (no %s): run `code-central service install`", m.unitPath)
		}
		if err := runServiceCmds(ctx, e, m.restart); err != nil {
			return err
		}
		fmt.Fprintln(e.Stdout, "Restarted the Code Central service.")
		return nil
	default:
		return usagef("unknown service command %q: expected install or restart", args[0])
	}
}

func serviceInstall(ctx context.Context, e Env, m serviceManager, args []string) error {
	fs := flagSet("service install")
	storeFlag := fs.String("store", "", "store directory")
	bind := fs.String("bind", "", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	bin, err := programPath(e)
	if err != nil {
		return err
	}
	dir, err := store.Dir(*storeFlag, e.home())
	if err != nil {
		return err
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(e.Dir, dir)
	}
	serveArgs := []string{bin, "serve", "--store", dir}
	if isSet(fs, "bind") {
		if _, _, err := parseBind(*bind, Port); err != nil {
			return err
		}
		serveArgs = append(serveArgs, "--bind", *bind)
	}
	if err := os.MkdirAll(filepath.Dir(m.unitPath), 0o755); err != nil {
		return err
	}
	unit, err := m.unit(serveArgs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.unitPath, unit, 0o644); err != nil {
		return err
	}
	if m.shortcutPath != "" {
		if err := os.MkdirAll(filepath.Dir(m.shortcutPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(m.shortcutPath, shortcutFile(bin, notify.AppID), 0o644); err != nil {
			return err
		}
	}
	if err := runServiceCmds(ctx, e, m.install); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "Installed %s and (re)started the service: %s\n", m.unitPath, strings.Join(serveArgs, " "))
	return nil
}

// serviceCmd is one launchctl, systemctl or schtasks call.
type serviceCmd struct {
	args    []string
	mayFail bool // e.g. booting out a service that isn't loaded
}

type serviceManager struct {
	unitPath string
	// shortcutPath, on Windows, is the Start-menu shortcut that makes
	// toasts show as Code Central.
	shortcutPath     string
	unit             func(serveArgs []string) ([]byte, error)
	install, restart []serviceCmd
}

func serviceManagerFor(e Env) (serviceManager, error) {
	goos := e.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	switch goos {
	case "darwin":
		dir, err := e.home().Home()
		if err != nil {
			return serviceManager{}, fmt.Errorf("cannot find the LaunchAgents folder: %w", err)
		}
		domain := fmt.Sprintf("gui/%d", os.Getuid())
		plist := filepath.Join(dir, "Library", "LaunchAgents", launchdLabel+".plist")
		log := filepath.Join(dir, "Library", "Logs", "code-central.log")
		return serviceManager{
			unitPath: plist,
			unit:     func(a []string) ([]byte, error) { return launchdPlist(a, log), nil },
			install: []serviceCmd{
				{args: []string{"launchctl", "bootout", domain + "/" + launchdLabel}, mayFail: true},
				{args: []string{"launchctl", "bootstrap", domain, plist}},
			},
			restart: []serviceCmd{{args: []string{"launchctl", "kickstart", "-k", domain + "/" + launchdLabel}}},
		}, nil
	case "linux":
		unit, err := e.home().Config("systemd", "user", systemdUnit)
		if err != nil {
			return serviceManager{}, fmt.Errorf("cannot find the systemd user folder: %w", err)
		}
		return serviceManager{
			unitPath: unit,
			unit:     func(a []string) ([]byte, error) { return systemdUnitFile(a), nil },
			install: []serviceCmd{
				{args: []string{"systemctl", "--user", "daemon-reload"}},
				{args: []string{"systemctl", "--user", "enable", systemdUnit}},
				{args: []string{"systemctl", "--user", "restart", systemdUnit}},
			},
			restart: []serviceCmd{{args: []string{"systemctl", "--user", "restart", systemdUnit}}},
		}, nil
	case "windows":
		// The task's XML sits next to its log (ADR 0011).
		dir, err := e.home().State("code-central")
		if err != nil {
			return serviceManager{}, fmt.Errorf("cannot find the service's folder: %w", err)
		}
		user := e.Getenv("USERNAME")
		if user == "" {
			return serviceManager{}, errors.New("cannot find who to run the scheduled task as: USERNAME isn't set")
		}
		if d := e.Getenv("USERDOMAIN"); d != "" {
			user = d + `\` + user
		}
		shortcut, err := notify.ShortcutPath(e.home())
		if err != nil {
			return serviceManager{}, err
		}
		task := filepath.Join(dir, "code-central-task.xml")
		log := filepath.Join(dir, "code-central.log")
		end := serviceCmd{args: []string{"schtasks", "/End", "/TN", scheduledTask}, mayFail: true} // it may not be running
		run := serviceCmd{args: []string{"schtasks", "/Run", "/TN", scheduledTask}}
		return serviceManager{
			unitPath:     task,
			shortcutPath: shortcut,
			unit:         func(a []string) ([]byte, error) { return scheduledTaskXML(a, log, user) },
			install: []serviceCmd{
				{args: []string{"schtasks", "/Create", "/TN", scheduledTask, "/XML", task, "/F"}},
				end, run,
			},
			restart: []serviceCmd{end, run},
		}, nil
	default:
		return serviceManager{}, fmt.Errorf("service install supports macOS (launchd), Linux (systemd) and Windows (Task Scheduler), not %s; run `code-central serve` instead", goos)
	}
}

// runServiceCmds runs cs in order, carrying on past a failure that may fail.
func runServiceCmds(ctx context.Context, e Env, cs []serviceCmd) error {
	for _, c := range cs {
		if err := runServiceCmd(ctx, e, c); err != nil && !c.mayFail {
			return err
		}
	}
	return nil
}

func runServiceCmd(ctx context.Context, e Env, c serviceCmd) error {
	run := e.RunService
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdout, cmd.Stderr = e.Stderr, e.Stderr
			return cmd.Run()
		}
	}
	if err := run(ctx, c.args[0], c.args[1:]...); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(c.args, " "), err)
	}
	return nil
}

// programPath is the binary's absolute path as invoked, without resolving
// symlinks, so a symlinked install keeps working after an upgrade. On
// Windows the binary may have been invoked without its .exe.
func programPath(e Env) (string, error) {
	p := e.Program
	if p == "" {
		return os.Executable()
	}
	if runtime.GOOS == "windows" && filepath.Ext(p) == "" {
		p += ".exe"
	}
	if filepath.Base(p) == p {
		found, err := lookPath(p, e.Getenv("PATH"))
		if err != nil {
			return "", err
		}
		p = found
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(e.Dir, p)
	}
	return filepath.Clean(p), nil
}

func lookPath(name, path string) (string, error) {
	for _, d := range filepath.SplitList(path) {
		if d == "" {
			d = "."
		}
		c := filepath.Join(d, name)
		// Windows has no execute bit; the .exe is what makes it a program.
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() && (runtime.GOOS == "windows" || fi.Mode()&0o111 != 0) {
			return c, nil
		}
	}
	return "", fmt.Errorf("cannot find %s on PATH; run service install with the binary's path", name)
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func launchdPlist(args []string, log string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchdLabel + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range args {
		b.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>` + xmlEscape(log) + `</string>
	<key>StandardErrorPath</key>
	<string>` + xmlEscape(log) + `</string>
</dict>
</plist>
`)
	return []byte(b.String())
}

func systemdUnitFile(args []string) []byte {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'\\$%;") || i == 0 || a == "" || filepath.IsAbs(a) {
			r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `$$`, `%`, `%%`)
			quoted[i] = `"` + r.Replace(a) + `"`
		} else {
			quoted[i] = a
		}
	}
	return []byte(`[Unit]
Description=Code Central
After=network.target

[Service]
ExecStart=` + strings.Join(quoted, " ") + `
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`)
}

// scheduledTaskXML is the Task Scheduler task that runs serve at the
// user's logon, in their session, restarting it when it fails. It runs
// the binary itself, not through cmd.exe, so /End ends serve and a crash
// fails the task; serve's --log stands in for redirecting its output.
// The file is UTF-16 with a byte-order mark, as Task Scheduler exports
// it, so schtasks reads it on every Windows version.
func scheduledTaskXML(args []string, log, user string) ([]byte, error) {
	args = append(append([]string{}, args...), "--log", log)
	// Task Scheduler expands %NAME% in a task's command and arguments.
	for _, a := range args {
		if strings.Contains(a, "%") {
			return nil, fmt.Errorf("cannot run %q from a scheduled task: Task Scheduler would expand its %%", a)
		}
	}
	quoted := make([]string, len(args)-1)
	for i, a := range args[1:] {
		quoted[i] = windowsArg(a)
	}
	doc := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Runs code-central serve at logon. Written by code-central service install.</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>` + xmlEscape(user) + `</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + xmlEscape(user) + `</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
    <MultipleInstancesPolicy>Queue</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + xmlEscape(args[0]) + `</Command>
      <Arguments>` + xmlEscape(strings.Join(quoted, " ")) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
	u := utf16.Encode([]rune("\uFEFF" + strings.ReplaceAll(doc, "\n", "\r\n")))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b, nil
}

// windowsArg quotes a for a Windows command line, as CommandLineToArgvW
// and Go's os.Args read it back: quotes around anything with a space,
// and a backslash doubled where it comes before a quote.
func windowsArg(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, c := range a {
		switch c {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1)) // n slashes become 2n+1
			slashes = 0
		default:
			slashes = 0
		}
		b.WriteRune(c)
	}
	b.WriteString(strings.Repeat(`\`, slashes)) // to 2n before the closing quote
	b.WriteByte('"')
	return b.String()
}
