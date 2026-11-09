//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClaudeEnv names, to a run of this test binary, the code-central
// binary it calls as the fake claude. It is in the Factory's environment,
// so its turns get it; the tests themselves never run with it.
const fakeClaudeEnv = "E2E_FAKE_CLAUDE_CODE_CENTRAL"

// 3. A Requirement to Done: the real service and the real Factory, with a
// fake claude working every Stage and the one implementation Ticket.
func TestARequirementGoesThroughEveryStageAndItsTicketLandsOnTheBase(t *testing.T) {
	svc := serve(t)
	// git reports real paths, so resolve macOS's /var -> /private/var.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, home, shims := filepath.Join(root, "work", "acme"), filepath.Join(root, "home"), filepath.Join(root, "bin")
	for _, d := range []string{repo, home, shims} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git := gitBinary(t)
	installSelf(t, filepath.Join(shims, "claude"+exeSuffix))
	env := []string{
		"PATH=" + shims + string(os.PathListSeparator) + filepath.Dir(git),
		"HOME=" + home,
		"TMPDIR=" + os.TempDir(), "TEMP=" + os.TempDir(), "TMP=" + os.TempDir(),
		fakeClaudeEnv + "=" + bin,
		"CODE_CENTRAL_URL=" + svc.url, // never the default service
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=E2E", "GIT_AUTHOR_EMAIL=e2e@example.com",
		"GIT_COMMITTER_NAME=E2E", "GIT_COMMITTER_EMAIL=e2e@example.com",
	}
	inRepo := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir, cmd.Env = repo, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	// The repository is Project acme, with init's files committed on main.
	inRepo("init", "-q", "-b", "main")
	if code, out := svc.cli(t, repo, "\n", "init", "--key", "acme"); code != 0 {
		t.Fatalf("init: exit %d: %s", code, out)
	}
	inRepo("add", "-A")
	inRepo("commit", "-q", "-m", "Set up Code Central")
	base := inRepo("rev-parse", "main")

	// 1. The human submits a Requirement: its Spec and three Stage Tickets.
	code, out := svc.cli(t, repo, "", "submit-requirement", "--text", "Greet the user\n\nSay hello on start.")
	if code != 0 {
		t.Fatalf("submit-requirement: exit %d: %s", code, out)
	}
	var submitted struct {
		Spec struct {
			ID string `json:"id"`
		} `json:"spec"`
	}
	json.Unmarshal([]byte(out), &submitted)
	if submitted.Spec.ID != "acme-1" {
		t.Fatalf("submit-requirement: %s", out)
	}

	// The Factory runs as its own process in the repository.
	factory := exec.Command(factoryBin, "--poll", "5s", "--parallel", "1")
	factory.Dir, factory.Env = repo, env
	startInOwnGroup(factory)
	var factoryOut syncBuffer
	factory.Stdout, factory.Stderr = &factoryOut, &factoryOut
	if err := factory.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- factory.Wait() }()
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			factory.Process.Kill()
			<-exited
		}
		if t.Failed() {
			t.Logf("Factory output:\n%s", factoryOut.String())
		}
	})

	ticket := func(id string) ticketView {
		t.Helper()
		code, out := svc.cli(t, repo, "", "get-ticket", id, "--part", "moves")
		var tk ticketView
		if code != 0 || json.Unmarshal([]byte(out), &tk) != nil {
			t.Fatalf("get-ticket %s: exit %d: %s", id, code, out)
		}
		return tk
	}
	waitFor := func(id, state string) ticketView {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		for {
			tk := ticket(id)
			if tk.State == state {
				return tk
			}
			select {
			case err := <-exited:
				t.Fatalf("the Factory exited (%v) with %s %s", err, id, tk.State)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s is %s, not %s", id, tk.State, state)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	accept := func(id string) {
		t.Helper()
		n := strings.TrimPrefix(id, "acme-")
		svc.human(t, "/api/p/acme/tickets/"+n+"/accept", `{}`)
	}

	// 2. The Stages, in order: the Factory claims each once the one before
	// is Done, the fake finishes it, and the human accepts it.
	for _, stage := range []struct{ id, label string }{
		{"acme-2", "stage:grilling"}, {"acme-3", "stage:spec"}, {"acme-4", "stage:tickets"},
	} {
		tk := waitFor(stage.id, "In Review")
		if !contains(tk.Labels, stage.label) {
			t.Fatalf("%s has Labels %v, want %s", stage.id, tk.Labels, stage.label)
		}
		accept(stage.id)
		waitFor(stage.id, "Done")
	}
	code, out = svc.cli(t, repo, "", "get-spec", "acme-1")
	if code != 0 || !strings.Contains(out, "## Problem Statement") {
		t.Fatalf("the spec Stage didn't write the Spec: %s", out)
	}

	// The tickets Stage made one implementation Ticket.
	impl := ticket("acme-5")
	if impl.Spec.ID != "acme-1" || len(impl.Labels) != 0 {
		t.Fatalf("implementation Ticket: %+v", impl)
	}

	// 3. The fake implements it with a commit on its branch and finishes
	// with branch and commit.
	impl = waitFor("acme-5", "In Review")
	if impl.Branch != "acme-5" || impl.Commit == "" {
		t.Fatalf("finish named branch %q, commit %q", impl.Branch, impl.Commit)
	}
	if got := inRepo("rev-parse", "acme-5"); got != impl.Commit {
		t.Fatalf("branch acme-5 is at %s, finish named %s", got, impl.Commit)
	}

	// 4. Accepting it lands it: the base has the merge, and it is Done.
	accept("acme-5")
	done := waitFor("acme-5", "Done")
	merge := inRepo("rev-parse", "main")
	if got := inRepo("log", "-1", "--format=%P", merge); got != base+" "+impl.Commit {
		t.Errorf("main's tip has parents %q, want %q then %q", got, base, impl.Commit)
	}
	if got := inRepo("log", "-1", "--format=%s", merge); got != "Merge acme-5: Say hello" {
		t.Errorf("merge commit subject %q", got)
	}
	if b, err := os.ReadFile(filepath.Join(repo, "hello.txt")); err != nil || string(b) != "hello\n" {
		t.Errorf("hello.txt on main: %q, %v", b, err)
	}
	if m := done.Moves[len(done.Moves)-1]; m.Commit != merge {
		t.Errorf("the land Move records commit %q, want the merge %s", m.Commit, merge)
	}

	// 5. Ctrl-C drains the idle Factory, which exits cleanly.
	if err := interrupt(factory.Process); err != nil {
		t.Fatalf("interrupting the Factory: %v", err)
	}
	select {
	case err := <-exited:
		stopped = true
		if err != nil {
			t.Fatalf("the Factory did not stop cleanly: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the Factory did not stop")
	}
	// factory/run.go's drain prints this when no turn is running.
	if out := factoryOut.String(); !strings.Contains(out, "drained: no turns running") {
		t.Errorf("the Factory did not drain:\n%s", out)
	}
}

// installSelf copies this test binary to dst, as the fake claude. Not a
// hard link: Windows refuses to delete any name of a running program, so
// a link to this binary would fail the temporary folder's cleanup.
func installSelf(t *testing.T, dst string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
}

type ticketView struct {
	ID     string   `json:"id"`
	State  string   `json:"state"`
	Labels []string `json:"labels"`
	Spec   struct {
		ID string `json:"id"`
	} `json:"spec"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Moves  []struct {
		Commit string `json:"commit"`
	} `json:"moves"`
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// gitBinary is the git binary itself. On macOS /usr/bin/git is an xcrun
// shim that is slow under a fresh HOME; the git in git's exec path isn't.
func gitBinary(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(git, "--exec-path").Output(); err == nil {
		if p, err := filepath.EvalSymlinks(filepath.Join(strings.TrimSpace(string(out)), "git")); err == nil {
			return p
		}
	}
	return git
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

var (
	promptTicket = regexp.MustCompile(`(?m)^Ticket: (\S+)$`)
	promptClaim  = regexp.MustCompile(`(?m)^Claim: (\S+)$`)
)

// fakeClaude is the fake claude the Factory runs: claude [flags] <prompt>.
// It works the prompt's Ticket under its Claim with the real code-central
// binary, as a session would, by the Ticket's Stage: grilling comments a
// resolution, spec writes the Spec, tickets makes one implementation
// Ticket, and implementation commits on the Ticket's branch. Each
// finishes the Ticket and ends the turn with a success result.
func fakeClaude(codeCentral string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "fake claude: no prompt")
		return 64
	}
	prompt := args[len(args)-1]
	tm, cm := promptTicket.FindStringSubmatch(prompt), promptClaim.FindStringSubmatch(prompt)
	if tm == nil || cm == nil {
		fmt.Fprintf(os.Stderr, "fake claude: no Ticket or Claim in the prompt %q\n", prompt)
		return 64
	}
	id, claim := tm[1], cm[1]
	run := func(name string, args ...string) (string, error) {
		var out, errOut bytes.Buffer
		cmd := exec.Command(name, args...)
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("%s %s: %v: %s", filepath.Base(name), strings.Join(args, " "), err, errOut.String())
		}
		return strings.TrimSpace(out.String()), nil
	}
	work := func() error {
		out, err := run(codeCentral, "get-ticket", id)
		if err != nil {
			return err
		}
		var tk ticketView
		if err := json.Unmarshal([]byte(out), &tk); err != nil {
			return err
		}
		finish := []string{"finish", id, "--claim", claim}
		switch {
		case contains(tk.Labels, "stage:grilling"):
			if _, err := run(codeCentral, "comment", id, "--body", "Resolution: the Requirement is clear. No open points."); err != nil {
				return err
			}
			finish = append(finish, "--summary", "Clear as written; see the resolution comment.")
		case contains(tk.Labels, "stage:spec"):
			body := "## Problem Statement\n\nThe user isn't greeted.\n\n## Solution\n\nSay hello on start.\n"
			if _, err := run(codeCentral, "edit-spec", tk.Spec.ID, "--body", body); err != nil {
				return err
			}
			finish = append(finish, "--summary", "Wrote the Spec.")
		case contains(tk.Labels, "stage:tickets"):
			if _, err := run(codeCentral, "create-ticket", "--spec", tk.Spec.ID, "--title", "Say hello",
				"--state", "Ready", "--blocked-by", id, "--description", "Write hello.txt."); err != nil {
				return err
			}
			finish = append(finish, "--summary", "1. Say hello (blocked by "+id+").")
		default:
			if err := os.WriteFile("hello.txt", []byte("hello\n"), 0o644); err != nil {
				return err
			}
			if _, err := run("git", "add", "hello.txt"); err != nil {
				return err
			}
			if _, err := run("git", "commit", "-q", "-m", "Say hello"); err != nil {
				return err
			}
			commit, err := run("git", "rev-parse", "HEAD")
			if err != nil {
				return err
			}
			finish = append(finish, "--summary", "Wrote hello.txt.", "--branch", id, "--commit", commit)
		}
		_, err = run(codeCentral, finish...)
		return err
	}
	if err := work(); err != nil {
		fmt.Fprintln(os.Stderr, "fake claude:", err)
		return 70
	}
	fmt.Println(`{"type":"result","subtype":"success","is_error":false,"result":"Finished ` + id + `."}`)
	return 0
}
