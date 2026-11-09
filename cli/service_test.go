package cli_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/tomoram/code-central/cli"
)

// fakeHost is a pretend machine: a temporary HOME, a working directory
// holding a linked binary, and a recording runner in place of launchctl,
// systemctl and schtasks. Nothing real is ever installed or restarted.
type fakeHost struct {
	t        *testing.T
	goos     string
	home     string
	dir      string
	program  string
	bin      string // the binary's absolute path, as the unit names it
	calls    []string
	failCall string
}

// newHost makes a host whose code-central is this test binary, linked
// into bin under its own name. It is never run: service install only
// records its path.
func newHost(t *testing.T, goos string) *fakeHost {
	t.Helper()
	h := &fakeHost{t: t, goos: goos, home: t.TempDir(), dir: t.TempDir()}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	real := filepath.Join(h.dir, "real-code-central"+exe)
	linkOrCopy(t, testBinary(t), real)
	os.MkdirAll(filepath.Join(h.dir, "bin"), 0o755)
	h.bin = filepath.Join(h.dir, "bin", "code-central"+exe)
	if err := os.Symlink(real, h.bin); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		// Symlinks need Developer Mode on Windows; a hard link will do.
		linkOrCopy(t, real, h.bin)
	}
	// As invoked, relative to Dir; on Windows without its .exe, as
	// typed at a prompt.
	h.program = filepath.Join("bin", "code-central")
	return h
}

// taskXML is where service install writes the scheduled task on Windows.
func (h *fakeHost) taskXML() string {
	return filepath.Join(h.home, ".local", "state", "code-central", "code-central-task.xml")
}

// shortcut is where service install writes the toasts' Start-menu
// shortcut on Windows.
func (h *fakeHost) shortcut() string {
	return filepath.Join(h.home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Code Central.lnk")
}

// wantTaskStarted checks install's schtasks calls: register (replacing
// any task of the same name), end it (allowed to fail), run it.
func (h *fakeHost) wantTaskStarted() {
	h.t.Helper()
	wantCalls(h.t, h.calls,
		"schtasks /Create /TN Code Central /XML "+h.taskXML()+" /F",
		"schtasks /End /TN Code Central",
		"schtasks /Run /TN Code Central")
}

// testBinary is this test binary's path: the fake code-central.
func testBinary(t *testing.T) string {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// linkOrCopy hard-links src to dst, or copies it across volumes.
func linkOrCopy(t *testing.T, src, dst string) {
	t.Helper()
	if os.Link(src, dst) == nil {
		return
	}
	b, err := os.ReadFile(src)
	if err == nil {
		err = os.WriteFile(dst, b, 0o755)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (h *fakeHost) run(env map[string]string, args ...string) result {
	h.t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	if _, ok := env["HOME"]; !ok {
		env["HOME"] = h.home
	}
	if _, ok := env["USERNAME"]; !ok && h.goos == "windows" {
		env["USERNAME"], env["USERDOMAIN"] = "ada", "LOVELACE-PC"
	}
	if _, ok := env["APPDATA"]; !ok && h.goos == "windows" {
		env["APPDATA"] = filepath.Join(h.home, "AppData", "Roaming")
	}
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
		Getenv:      func(k string) string { return env[k] },
		UserHomeDir: userHomeDir(env),
		Dir:         h.dir,
		Program:     h.program,
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

// read reads a unit, decoding a scheduled task's UTF-16 into LF lines.
func (h *fakeHost) read(path string) string {
	h.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatalf("unit not written: %v", err)
	}
	if !bytes.HasPrefix(b, []byte{0xff, 0xfe}) {
		return string(b)
	}
	u := make([]uint16, (len(b)-2)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2+2*i:])
	}
	return strings.ReplaceAll(string(utf16.Decode(u)), "\r\n", "\n")
}

func wantCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestServiceInstallWritesALaunchdAgentOnMacOS(t *testing.T) {
	t.Parallel()
	h := newHost(t, "darwin")
	r := h.run(nil, "service", "install")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	plist := h.read(filepath.Join(h.home, "Library", "LaunchAgents", "com.github.tomoram.code-central.plist"))
	bin := h.bin // the symlink as invoked, not its target
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
	t.Parallel()
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
	t.Parallel()
	h := newHost(t, "linux")
	store := filepath.Join(h.dir, "my store")
	if r := h.run(nil, "service", "install", "--store", store); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	unit := h.read(filepath.Join(h.home, ".config", "systemd", "user", "code-central.service"))
	for _, want := range []string{
		`ExecStart="` + systemdEscaped(h.bin) + `" serve --store "` + systemdEscaped(store) + `"`,
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

// systemdEscaped is s as a systemd unit writes it inside quotes: the
// backslashes of a Windows path doubled.
func systemdEscaped(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }

func TestSystemdUnitHonoursXDGConfigHome(t *testing.T) {
	t.Parallel()
	h := newHost(t, "linux")
	cfg := t.TempDir()
	if r := h.run(map[string]string{"XDG_CONFIG_HOME": cfg}, "service", "install"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	h.read(filepath.Join(cfg, "systemd", "user", "code-central.service"))
}

func TestWithNoHOMEServiceInstallUsesTheProfileFolder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ goos, unit string }{
		{"darwin", filepath.Join("Library", "LaunchAgents", "com.github.tomoram.code-central.plist")},
		{"linux", filepath.Join(".config", "systemd", "user", "code-central.service")},
		{"windows", filepath.Join(".local", "state", "code-central", "code-central-task.xml")},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			h := newHost(t, tc.goos)
			profile := t.TempDir()
			if r := h.run(map[string]string{"HOME": "", "USERPROFILE": profile}, "service", "install"); r.code != 0 {
				t.Fatalf("exit %d: %s", r.code, r.stderr)
			}
			// The store's default is under the profile folder too.
			want := filepath.Join(profile, ".local", "share", "code-central")
			if tc.goos == "linux" {
				want = systemdEscaped(want)
			}
			if unit := h.read(filepath.Join(profile, tc.unit)); !strings.Contains(unit, want) {
				t.Fatalf("unit doesn't name the profile folder's store:\n%s", unit)
			}
		})
	}
}

func TestWithNoHomeAtAllServiceInstallNamesEveryOption(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	for _, tc := range []struct{ goos, want string }{
		{"darwin", fmt.Sprintf("launchctl kickstart -k gui/%d/com.github.tomoram.code-central", os.Getuid())},
		{"linux", "systemctl --user restart code-central.service"},
		{"windows", "schtasks /End /TN Code Central\nschtasks /Run /TN Code Central"},
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
			wantCalls(t, h.calls, strings.Split(tc.want, "\n")...)
		})
	}
}

func TestServiceRestartWithNothingInstalledSaysHowToInstall(t *testing.T) {
	t.Parallel()
	h := newHost(t, "darwin")
	r := h.run(nil, "service", "restart")
	if r.code != 1 || !strings.Contains(r.stderr, "code-central service install") || len(h.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.stderr, h.calls)
	}
}

func TestServiceReportsAFailedRunner(t *testing.T) {
	t.Parallel()
	h := newHost(t, "linux")
	h.failCall = "systemctl --user restart"
	r := h.run(nil, "service", "install")
	if r.code != 1 || !strings.Contains(r.stderr, "systemctl --user restart code-central.service") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestServiceRefusesOtherSystemsAndBadArguments(t *testing.T) {
	t.Parallel()
	h := newHost(t, "freebsd")
	if r := h.run(nil, "service", "install"); r.code != 1 || !strings.Contains(r.stderr, "Windows (Task Scheduler), not freebsd") {
		t.Fatalf("freebsd: exit %d, stderr %q", r.code, r.stderr)
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

func TestServiceInstallRegistersAScheduledTaskOnWindows(t *testing.T) {
	t.Parallel()
	h := newHost(t, "windows")
	store := filepath.Join(h.dir, "my store")
	r := h.run(nil, "service", "install", "--store", store, "--bind", "172.17.0.1")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	task := h.read(h.taskXML()) // next to the log
	log := filepath.Join(filepath.Dir(h.taskXML()), "code-central.log")
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-16"?>`,
		"<LogonTrigger>\n      <Enabled>true</Enabled>\n      <UserId>LOVELACE-PC\\ada</UserId>", // at this user's logon
		"<UserId>LOVELACE-PC\\ada</UserId>\n      <LogonType>InteractiveToken</LogonType>",       // as them, in their session
		"<RestartOnFailure>\n      <Interval>PT1M</Interval>\n      <Count>999</Count>",
		"<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>",            // no time limit
		"<MultipleInstancesPolicy>Queue</MultipleInstancesPolicy>", // /Run waits for an /End still in progress
		// The binary itself, so /End ends serve, with launchd's and
		// systemd's serve arguments and its output in the log.
		"<Command>" + h.bin + "</Command>\n      <Arguments>serve --store &#34;" + store +
			"&#34; --bind 172.17.0.1 --log " + windowsQuoted(log) + "</Arguments>",
	} {
		if !strings.Contains(task, want) {
			t.Fatalf("task lacks %q:\n%s", want, task)
		}
	}
	h.wantTaskStarted()
	if target, appID := readShortcut(t, h.shortcut()); target != h.bin || appID != "CodeCentral.Service" {
		t.Fatalf("shortcut to %q as %q; want to %q as CodeCentral.Service", target, appID, h.bin)
	}
	if !strings.Contains(r.stdout, h.bin) {
		t.Fatalf("stdout %q doesn't name the binary", r.stdout)
	}
}

// windowsQuoted is p as the task's arguments hold it, XML-escaped: quoted
// only when it has a space.
func windowsQuoted(p string) string {
	if strings.Contains(p, " ") {
		return "&#34;" + p + "&#34;"
	}
	return p
}

func TestServiceInstallAgainReplacesTheScheduledTask(t *testing.T) {
	t.Parallel()
	h := newHost(t, "windows")
	os.MkdirAll(filepath.Dir(h.taskXML()), 0o755)
	os.WriteFile(h.taskXML(), []byte("an older task"), 0o644)
	os.MkdirAll(filepath.Dir(h.shortcut()), 0o755)
	os.WriteFile(h.shortcut(), []byte("an older shortcut"), 0o644)
	// Ending a task that isn't running fails; install carries on.
	h.failCall = "schtasks /End"
	if r := h.run(nil, "service", "install"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if got := h.read(h.taskXML()); strings.Contains(got, "an older task") {
		t.Fatalf("task not rewritten:\n%s", got)
	}
	if _, appID := readShortcut(t, h.shortcut()); appID != "CodeCentral.Service" {
		t.Fatalf("shortcut not rewritten: AUMID %q", appID)
	}
	h.wantTaskStarted()

	h.calls = nil
	if r := h.run(nil, "service", "restart"); r.code != 0 {
		t.Fatalf("restart: exit %d: %s", r.code, r.stderr)
	}
	wantCalls(t, h.calls, "schtasks /End /TN Code Central", "schtasks /Run /TN Code Central")
}

func TestScheduledTaskRefusesAPathTaskSchedulerWouldExpand(t *testing.T) {
	t.Parallel()
	h := newHost(t, "windows")
	r := h.run(nil, "service", "install", "--store", filepath.Join(h.dir, "100%PATH%"))
	if r.code == 0 || !strings.Contains(r.stderr, "Task Scheduler would expand its %") || len(h.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.stderr, h.calls)
	}
}

func TestScheduledTaskWithNoUSERNAMESaysSo(t *testing.T) {
	t.Parallel()
	h := newHost(t, "windows")
	r := h.run(map[string]string{"USERNAME": ""}, "service", "install")
	if r.code == 0 || !strings.Contains(r.stderr, "USERNAME isn't set") || len(h.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.stderr, h.calls)
	}
}

func TestWindowsArgRoundTripsThroughTheCommandLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{`C:\Users\ada\bin\code-central.exe`, `C:\Users\ada\bin\code-central.exe`},
		{`C:\my store`, `"C:\my store"`},
		{`C:\my store\`, `"C:\my store\\"`}, // the closing quote isn't escaped
		{`a"b`, `"a\"b"`},
		{`a\"b`, `"a\\\"b"`},
		{``, `""`},
	} {
		if got := cli.WindowsArg(tc.in); got != tc.want {
			t.Errorf("WindowsArg(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// readShortcut reads a Shell Link (.lnk) file as Windows does
// ([MS-SHLLINK]): the target's path from its LinkInfo, and the AUMID
// (System.AppUserModel.ID) from its property store.
func readShortcut(t *testing.T, path string) (target, appID string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("shortcut not written: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("malformed shortcut: %v", r)
		}
	}()
	u16 := func(i int) int { return int(binary.LittleEndian.Uint16(b[i:])) }
	u32 := func(i int) int { return int(binary.LittleEndian.Uint32(b[i:])) }
	str := func(i, n int) string { // n UTF-16 code units at i
		u := make([]uint16, n)
		for k := range u {
			u[k] = uint16(u16(i + 2*k))
		}
		return string(utf16.Decode(u))
	}
	strz := func(i int) string { // a NUL-terminated UTF-16 string at i
		n := 0
		for u16(i+2*n) != 0 {
			n++
		}
		return str(i, n)
	}
	shellLink := []byte{0x01, 0x14, 0x02, 0, 0, 0, 0, 0, 0xc0, 0, 0, 0, 0, 0, 0, 0x46}
	if u32(0) != 0x4c || !bytes.Equal(b[4:20], shellLink) {
		t.Fatalf("not a Shell Link header: % x", b[:20])
	}
	flags := u32(20)
	off := 0x4c
	if flags&0x1 != 0 { // HasLinkTargetIDList
		off += 2 + u16(off)
	}
	if flags&0x2 != 0 { // HasLinkInfo, with its Unicode path (header 0x24)
		if u32(off+4) >= 0x24 {
			target = strz(off + u32(off+28))
		}
		off += u32(off)
	}
	if flags&0x80 == 0 {
		t.Fatal("not IsUnicode")
	}
	for _, f := range []int{0x4, 0x8, 0x10, 0x20, 0x40} { // the StringData
		if flags&f != 0 {
			off += 2 + 2*u16(off)
		}
	}
	appModel := []byte{0x55, 0x28, 0x4c, 0x9f, 0x79, 0x9f, 0x39, 0x4b, 0xa8, 0xd0, 0xe1, 0xd4, 0x2d, 0xe1, 0xd5, 0xf3}
	for u32(off) >= 4 { // the ExtraData blocks, up to the terminal block
		if u32(off+4) == 0xa0000009 { // a PropertyStoreDataBlock
			for p := off + 8; u32(p) != 0; p += u32(p) {
				if u32(p+4) != 0x53505331 {
					t.Fatalf("property storage version %x", u32(p+4))
				}
				for q := p + 24; u32(q) != 0; q += u32(q) {
					if bytes.Equal(b[p+8:p+24], appModel) && u32(q+4) == 5 && u16(q+9) == 0x1f { // VT_LPWSTR
						appID = str(q+17, u32(q+13)-1)
					}
				}
			}
		}
		off += u32(off)
	}
	if off+4 != len(b) {
		t.Fatalf("%d bytes after the terminal block", len(b)-off-4)
	}
	return target, appID
}
