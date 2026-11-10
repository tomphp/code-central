// Package ciwatchertest is the test harness for the CI Watcher, modelled
// on factorytest's fake claude: the test binary, linked or copied into
// the harness's folders as gh, runs as a fake gh (internal/fakegh) when
// the harness's environment names its data folder. It follows a Script
// of API answers and records its calls; nothing is built and there is no
// shell, so it runs the same on every OS. The test binary also runs as
// code-central, the CLI, so a test can run the CI Watcher subcommand as
// the service does, in its own process, and as a CI Watcher that crashes,
// so a test can watch the service restart it. The Issue Watcher's tests
// use it too, as both kinds reach GitHub through the same gh.
//
// A test package that uses it calls Main from its TestMain:
//
//	func TestMain(m *testing.M) { ciwatchertest.Main(m) }
package ciwatchertest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/internal/fakegh"
)

// fakeEnv names the harness's data folder to a test binary run as a fake.
const fakeEnv = "CIWATCHERTEST_FAKE"

// Main runs the fake the test binary was started as, named by the program
// name it was run under (gh or code-central), and the tests otherwise.
func Main(m *testing.M) {
	if data := os.Getenv(fakeEnv); data != "" {
		os.Exit(runFake(data, os.Args))
	}
	os.Exit(m.Run())
}

func runFake(data string, args []string) int {
	switch name := strings.TrimSuffix(filepath.Base(args[0]), exeSuffix); name {
	case "gh":
		return fakegh.Run(data, args[0], args[1:], os.Stdout, os.Stderr)
	case "code-central":
		return cli.Run(context.Background(), args[1:], cli.Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv})
	case crashingName:
		return crash(data)
	default:
		fmt.Fprintf(os.Stderr, "ciwatchertest: %s=%s but run as %q, not a fake\n", fakeEnv, data, name)
		return 70
	}
}

// crashingName is the program name the test binary crashes under.
const crashingName = "crashing-code-central"

// The crashing CI Watcher's files in the data folder: a line in runs for
// each run, and while hold exists, it waits before crashing.
const (
	crashRuns = "crash-runs"
	crashHold = "crash-hold"
)

// crash is a CI Watcher that crashes: it counts its run, waits while
// held, then writes two lines to stderr and exits 2, as a panic does.
func crash(data string) int {
	f, err := os.OpenFile(filepath.Join(data, crashRuns), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		_, err = f.WriteString("run\n")
		f.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "crashing CI Watcher:", err)
		return 70
	}
	for {
		if _, err := os.Stat(filepath.Join(data, crashHold)); err != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Fprint(os.Stderr, "starting\npanic: the CI Watcher broke on purpose\n")
	return 2
}

// exeSuffix ends the name of a program: .exe on Windows.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// Harness is one fake gh's world.
type Harness struct {
	t    testing.TB
	data string
	// Bin is a folder holding the fake gh, for PATH.
	Bin string
	// CodeCentral is the test binary as code-central, in a folder of its
	// own, as the service's executable.
	CodeCentral string
	// Crashing is a CI Watcher that crashes on every run, as the
	// service's executable.
	Crashing string
}

// New sets up a fake gh in Bin, with no Script: every call exits 0.
func New(t testing.TB) *Harness {
	t.Helper()
	root := t.TempDir()
	h := &Harness{t: t, data: filepath.Join(root, "data"), Bin: filepath.Join(root, "bin")}
	for _, d := range []string{h.data, h.Bin, filepath.Join(root, "program")} {
		must(t, os.MkdirAll(d, 0o755))
	}
	h.GH(h.Bin)
	h.CodeCentral = h.install(filepath.Join(root, "program"), "code-central")
	h.Crashing = h.install(filepath.Join(root, "program"), crashingName)
	return h
}

// CrashRuns is how many times the crashing CI Watcher has started.
func (h *Harness) CrashRuns() int {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.data, crashRuns))
	if os.IsNotExist(err) {
		return 0
	}
	must(h.t, err)
	return strings.Count(string(b), "\n")
}

// HoldCrashes makes the crashing CI Watcher run until ReleaseCrashes
// before it crashes.
func (h *Harness) HoldCrashes() {
	h.t.Helper()
	must(h.t, os.WriteFile(filepath.Join(h.data, crashHold), nil, 0o644))
}

// ReleaseCrashes lets a held crashing CI Watcher crash.
func (h *Harness) ReleaseCrashes() {
	h.t.Helper()
	must(h.t, os.Remove(filepath.Join(h.data, crashHold)))
}

// GH puts a fake gh in dir, as gh's installer would, and returns its
// path.
func (h *Harness) GH(dir string) string {
	h.t.Helper()
	must(h.t, os.MkdirAll(dir, 0o755))
	return h.install(dir, "gh")
}

// install puts the test binary in dir as name: a hard link, or a copy
// where the link can't be made.
func (h *Harness) install(dir, name string) string {
	self, err := os.Executable()
	must(h.t, err)
	dst := filepath.Join(dir, name+exeSuffix)
	if os.Link(self, dst) == nil {
		return dst
	}
	src, err := os.Open(self)
	must(h.t, err)
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	must(h.t, err)
	_, err = io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	must(h.t, err)
	return dst
}

// Environ is the environment with PATH set to path, the folders joined,
// and the fakes' data folder named.
func (h *Harness) Environ(path ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "PATH") && k != fakeEnv {
			env = append(env, kv)
		}
	}
	return append(env, "PATH="+strings.Join(path, string(os.PathListSeparator)), fakeEnv+"="+h.data)
}

// Answer adds answers to the fake gh's Script; the latest that matches a
// call is given.
func (h *Harness) Answer(answers ...fakegh.Answer) {
	h.t.Helper()
	must(h.t, fakegh.Script(h.data, answers...))
}

// LoggedOut makes gh auth status fail as a gh with no login does.
func (h *Harness) LoggedOut() {
	h.Answer(fakegh.Answer{Args: []string{"auth", "status"}, Exit: 1,
		Stderr: "You are not logged into any GitHub hosts. To log in, run: gh auth login\n"})
}

// Unreadable makes reading repository fail as GitHub answers for a
// repository the login can't see.
func (h *Harness) Unreadable(repository string) {
	h.Answer(fakegh.Answer{Args: []string{"api", "repos/" + repository}, Exit: 1,
		Stderr: "gh: Not Found (HTTP 404)\n"})
}

// Calls is every call of the fake gh so far, oldest first.
func (h *Harness) Calls() []fakegh.Call {
	h.t.Helper()
	calls, err := fakegh.Calls(h.data)
	must(h.t, err)
	return calls
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
