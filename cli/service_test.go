package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
)

// unixOnly skips t on Windows, which lacks the Unix-only tool or signal
// it needs: ps, /bin/sh or os.Interrupt.
func unixOnly(t *testing.T, needs string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skipf("needs %s; code-central-124 ports this", needs)
	}
}

// fakeHost is a pretend machine: a temporary HOME, a working directory
// holding a symlinked binary, and a recording runner in place of
// launchctl and systemctl. Nothing real is ever installed or restarted.
type fakeHost struct {
	t        *testing.T
	goos     string
	home     string
	dir      string
	calls    []string
	failCall string
}

func newHost(t *testing.T, goos string) *fakeHost {
	t.Helper()
	unixOnly(t, "/bin/sh for the fake code-central binary")
	h := &fakeHost{t: t, goos: goos, home: t.TempDir(), dir: t.TempDir()}
	real := filepath.Join(h.dir, "real-code-central")
	os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755)
	os.MkdirAll(filepath.Join(h.dir, "bin"), 0o755)
	if err := os.Symlink(real, filepath.Join(h.dir, "bin", "code-central")); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *fakeHost) run(env map[string]string, args ...string) result {
	h.t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	if _, ok := env["HOME"]; !ok {
		env["HOME"] = h.home
	}
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
		Getenv:      func(k string) string { return env[k] },
		UserHomeDir: userHomeDir(env),
		Dir:         h.dir,
		Program:     "bin/code-central", // as invoked, relative to Dir
		GOOS:        h.goos,
		RunService: func(_ context.Context, name string, args ...string) error {
			call := strings.Join(append([]string{name}, args...), " ")
			h.calls = append(h.calls, call)
			if h.failCall != "" && strings.HasPrefix(call, h.failCall) {
				return errors.New("exit status 5")
			}
			return nil
		},
	})
	return result{code, out.String(), errOut.String()}
}

func (h *fakeHost) read(path string) string {
	h.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatalf("unit not written: %v", err)
	}
	return string(b)
}

func wantCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestServiceInstallWritesALaunchdAgentOnMacOS(t *testing.T) {
	h := newHost(t, "darwin")
	r := h.run(nil, "service", "install")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	plist := h.read(filepath.Join(h.home, "Library", "LaunchAgents", "com.github.tomoram.code-central.plist"))
	bin := filepath.Join(h.dir, "bin", "code-central") // the symlink as invoked, not its target
	for _, want := range []string{
		"<key>Label</key>\n\t<string>com.github.tomoram.code-central</string>",
		"<string>" + bin + "</string>\n\t\t<string>serve</string>\n\t\t<string>--store</string>\n\t\t<string>" +
			filepath.Join(h.home, ".local", "share", "code-central") + "</string>",
		"<key>RunAtLoad</key>\n\t<true/>",                                             // starts at login
		"<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>", // restarts after a crash
		"<string>" + filepath.Join(h.home, "Library", "Logs", "code-central.log") + "</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist lacks %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "real-code-central") {
		t.Fatalf("the symlink was resolved:\n%s", plist)
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	wantCalls(t, h.calls,
		"launchctl bootout "+domain+"/com.github.tomoram.code-central",
		"launchctl bootstrap "+domain+" "+filepath.Join(h.home, "Library", "LaunchAgents", "com.github.tomoram.code-central.plist"))
	if !strings.Contains(r.stdout, bin) {
		t.Fatalf("stdout %q doesn't name the binary", r.stdout)
	}
}

func TestServiceInstallAgainRewritesTheUnitAndRestarts(t *testing.T) {
	h := newHost(t, "darwin")
	plist := filepath.Join(h.home, "Library", "LaunchAgents", "com.github.tomoram.code-central.plist")
	os.MkdirAll(filepath.Dir(plist), 0o755)
	os.WriteFile(plist, []byte("an older unit"), 0o644)
	// A bootout of a service that isn't loaded fails; install carries on.
	h.failCall = "launchctl bootout"
	if r := h.run(nil, "service", "install", "--bind", "172.17.0.1"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	got := h.read(plist)
	if strings.Contains(got, "an older unit") || !strings.Contains(got, "<string>--bind</string>\n\t\t<string>172.17.0.1</string>") {
		t.Fatalf("plist not rewritten with --bind:\n%s", got)
	}
	if len(h.calls) != 2 || !strings.HasPrefix(h.calls[1], "launchctl bootstrap") {
		t.Fatalf("calls %v", h.calls)
	}
}

func TestServiceInstallWritesASystemdUserUnitOnLinux(t *testing.T) {
	h := newHost(t, "linux")
	store := filepath.Join(h.dir, "my store")
	if r := h.run(nil, "service", "install", "--store", store); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	unit := h.read(filepath.Join(h.home, ".config", "systemd", "user", "code-central.service"))
	bin := filepath.Join(h.dir, "bin", "code-central")
	for _, want := range []string{
		`ExecStart="` + bin + `" serve --store "` + store + `"`,
		"Restart=on-failure",      // restarts after a crash
		"WantedBy=default.target", // starts at login
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit lacks %q:\n%s", want, unit)
		}
	}
	wantCalls(t, h.calls,
		"systemctl --user daemon-reload",
		"systemctl --user enable code-central.service",
		"systemctl --user restart code-central.service")
}

func TestSystemdUnitHonoursXDGConfigHome(t *testing.T) {
	h := newHost(t, "linux")
	cfg := t.TempDir()
	if r := h.run(map[string]string{"XDG_CONFIG_HOME": cfg}, "service", "install"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	h.read(filepath.Join(cfg, "systemd", "user", "code-central.service"))
}

func TestWithNoHOMEServiceInstallUsesTheProfileFolder(t *testing.T) {
	for _, tc := range []struct{ goos, unit string }{
		{"darwin", filepath.Join("Library", "LaunchAgents", "com.github.tomoram.code-central.plist")},
		{"linux", filepath.Join(".config", "systemd", "user", "code-central.service")},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			h := newHost(t, tc.goos)
			profile := t.TempDir()
			if r := h.run(map[string]string{"HOME": "", "USERPROFILE": profile}, "service", "install"); r.code != 0 {
				t.Fatalf("exit %d: %s", r.code, r.stderr)
			}
			// The store's default is under the profile folder too.
			if unit := h.read(filepath.Join(profile, tc.unit)); !strings.Contains(unit, filepath.Join(profile, ".local", "share", "code-central")) {
				t.Fatalf("unit doesn't name the profile folder's store:\n%s", unit)
			}
		})
	}
}

func TestWithNoHomeAtAllServiceInstallNamesEveryOption(t *testing.T) {
	h := newHost(t, "linux")
	r := h.run(map[string]string{"HOME": ""}, "service", "install")
	if want := "set XDG_CONFIG_HOME or HOME, or give this account a profile folder (%USERPROFILE% on Windows)"; r.code != 1 || !strings.Contains(r.stderr, want) {
		t.Fatalf("exit %d, stderr %q; want %q", r.code, r.stderr, want)
	}
	h = newHost(t, "linux")
	r = h.run(map[string]string{"HOME": "", "XDG_CONFIG_HOME": t.TempDir()}, "service", "install")
	if want := "cannot find the store: set CODE_CENTRAL_STORE, XDG_DATA_HOME or HOME, or give this account a profile folder (%USERPROFILE% on Windows)"; r.code != 1 || !strings.Contains(r.stderr, want) {
		t.Fatalf("exit %d, stderr %q; want %q", r.code, r.stderr, want)
	}
}

func TestServiceRestartRestartsTheInstalledService(t *testing.T) {
	for _, tc := range []struct{ goos, want string }{
		{"darwin", fmt.Sprintf("launchctl kickstart -k gui/%d/com.github.tomoram.code-central", os.Getuid())},
		{"linux", "systemctl --user restart code-central.service"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			h := newHost(t, tc.goos)
			if r := h.run(nil, "service", "install"); r.code != 0 {
				t.Fatalf("install: exit %d: %s", r.code, r.stderr)
			}
			h.calls = nil
			if r := h.run(nil, "service", "restart"); r.code != 0 {
				t.Fatalf("restart: exit %d: %s", r.code, r.stderr)
			}
			wantCalls(t, h.calls, tc.want)
		})
	}
}

func TestServiceRestartWithNothingInstalledSaysHowToInstall(t *testing.T) {
	h := newHost(t, "darwin")
	r := h.run(nil, "service", "restart")
	if r.code != 1 || !strings.Contains(r.stderr, "code-central service install") || len(h.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.stderr, h.calls)
	}
}

func TestServiceReportsAFailedRunner(t *testing.T) {
	h := newHost(t, "linux")
	h.failCall = "systemctl --user restart"
	r := h.run(nil, "service", "install")
	if r.code != 1 || !strings.Contains(r.stderr, "systemctl --user restart code-central.service") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestServiceRefusesOtherSystemsAndBadArguments(t *testing.T) {
	h := newHost(t, "windows")
	if r := h.run(nil, "service", "install"); r.code != 1 || !strings.Contains(r.stderr, "launchd") {
		t.Fatalf("windows: exit %d, stderr %q", r.code, r.stderr)
	}
	h = newHost(t, "linux")
	for _, args := range [][]string{{"service"}, {"service", "start"}, {"service", "install", "--bind", "0.0.0.0"}} {
		if r := h.run(nil, args...); r.code != 1 {
			t.Fatalf("%v: exit %d", args, r.code)
		}
	}
	if len(h.calls) != 0 {
		t.Fatalf("calls %v", h.calls)
	}
}
