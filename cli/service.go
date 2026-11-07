package cli

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tomoram/code-central/store"
)

func init() {
	commands["service"] = command{
		usage: "service install [--store <dir>] [--bind <ip>] | service restart  run serve as a login item " +
			"(launchd on macOS, systemd user unit on Linux); install again after an upgrade",
		run: runService,
	}
}

// The launchd label and the systemd unit name.
const (
	launchdLabel = "com.github.tomoram.code-central"
	systemdUnit  = "code-central.service"
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
		for _, c := range m.restart {
			if err := runServiceCmd(ctx, e, c); err != nil {
				return err
			}
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
	dir, err := store.Dir(*storeFlag, e.Getenv)
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
	if err := os.WriteFile(m.unitPath, m.unit(serveArgs), 0o644); err != nil {
		return err
	}
	for _, c := range m.install {
		if err := runServiceCmd(ctx, e, c); err != nil && !c.mayFail {
			return err
		}
	}
	fmt.Fprintf(e.Stdout, "Installed %s and (re)started the service: %s\n", m.unitPath, strings.Join(serveArgs, " "))
	return nil
}

// serviceCmd is one launchctl or systemctl call.
type serviceCmd struct {
	args    []string
	mayFail bool // e.g. booting out a service that isn't loaded
}

type serviceManager struct {
	unitPath         string
	unit             func(serveArgs []string) []byte
	install, restart []serviceCmd
}

func serviceManagerFor(e Env) (serviceManager, error) {
	goos := e.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	home := e.Getenv("HOME")
	switch goos {
	case "darwin":
		if home == "" {
			return serviceManager{}, errors.New("cannot find the LaunchAgents folder: HOME is not set")
		}
		domain := fmt.Sprintf("gui/%d", os.Getuid())
		plist := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
		log := filepath.Join(home, "Library", "Logs", "code-central.log")
		return serviceManager{
			unitPath: plist,
			unit:     func(a []string) []byte { return launchdPlist(a, log) },
			install: []serviceCmd{
				{args: []string{"launchctl", "bootout", domain + "/" + launchdLabel}, mayFail: true},
				{args: []string{"launchctl", "bootstrap", domain, plist}},
			},
			restart: []serviceCmd{{args: []string{"launchctl", "kickstart", "-k", domain + "/" + launchdLabel}}},
		}, nil
	case "linux":
		cfg := e.Getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			if home == "" {
				return serviceManager{}, errors.New("cannot find the systemd user folder: set HOME or XDG_CONFIG_HOME")
			}
			cfg = filepath.Join(home, ".config")
		}
		return serviceManager{
			unitPath: filepath.Join(cfg, "systemd", "user", systemdUnit),
			unit:     systemdUnitFile,
			install: []serviceCmd{
				{args: []string{"systemctl", "--user", "daemon-reload"}},
				{args: []string{"systemctl", "--user", "enable", systemdUnit}},
				{args: []string{"systemctl", "--user", "restart", systemdUnit}},
			},
			restart: []serviceCmd{{args: []string{"systemctl", "--user", "restart", systemdUnit}}},
		}, nil
	default:
		return serviceManager{}, fmt.Errorf("service install supports macOS (launchd) and Linux (systemd), not %s; run `code-central serve` instead", goos)
	}
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
// symlinks, so a symlinked install keeps working after an upgrade.
func programPath(e Env) (string, error) {
	p := e.Program
	if p == "" {
		return os.Executable()
	}
	if !strings.ContainsRune(p, filepath.Separator) {
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
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return c, nil
		}
	}
	return "", fmt.Errorf("cannot find %s on PATH; run service install with the binary's path", name)
}

func launchdPlist(args []string, log string) []byte {
	esc := func(s string) string {
		var b bytes.Buffer
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
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
		b.WriteString("\t\t<string>" + esc(a) + "</string>\n")
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
	<string>` + esc(log) + `</string>
	<key>StandardErrorPath</key>
	<string>` + esc(log) + `</string>
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
