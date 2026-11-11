//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/internal/fakegh"
)

// started is serve's log line for a Watcher of kind it started for acme,
// with its pid.
func started(kind string) *regexp.Regexp {
	return regexp.MustCompile(`started the ` + kind + ` for acme \(pid (\d+)\)`)
}

// The service runs a CI Watcher for a Project with CI Tracking on, and an
// Issue Watcher for one with GitHub Issues on, each as its own
// executable, stops each on its turn-off and both when it stops, and
// leaves none behind when it is killed: a Watcher exits when its stdin, a
// pipe from the service, closes, as a stopped Windows scheduled task
// doesn't kill the service's children.
func TestTheServiceSupervisesItsWatchers(t *testing.T) {
	// The fake gh, through the gh path Option, answering yes to
	// everything, and that its login can triage acme/web's issues.
	ghData := t.TempDir()
	t.Setenv("FAKE_GH_DIR", ghData)
	if err := fakegh.Script(ghData, fakegh.Answer{Args: []string{"api", "repos/acme/web"},
		Stdout: `{"full_name":"acme/web","has_issues":true,"permissions":{"triage":true,"pull":true}}`}); err != nil {
		t.Fatal(err)
	}
	store := t.TempDir()
	first, msg := startServe(t, store, "127.0.0.1:0")
	if first == nil {
		t.Fatalf("serve: %s", msg)
	}
	svc := &service{url: "http://" + first.addr}
	svc.human(t, "/api/projects/create_project", `{"key":"acme"}`)
	gh, _ := json.Marshal(map[string]string{"gh_path": fakeGHBin})
	svc.human(t, "/api/set_options", string(gh))
	tracking := func(on bool) {
		svc.human(t, "/api/p/acme/set_ci_tracking", `{"tracking":`+strconv.FormatBool(on)+`,"repository":"acme/web"}`)
	}
	issues := func(on bool) {
		svc.human(t, "/api/p/acme/set_github_issues", `{"on":`+strconv.FormatBool(on)+`,"repository":"acme/web"}`)
	}

	// Turning each on starts its Watcher, which reports Watching.
	tracking(true)
	ci := watcherPID(t, first, "CI Watcher", 1)
	svc.watcherState(t, "ci", "watching")
	issues(true)
	issue := watcherPID(t, first, "Issue Watcher", 1)
	svc.watcherState(t, "github_issues", "watching")

	// Turning each off stops its own.
	tracking(false)
	codecentraltest.Eventually(t, func() bool { return !alive(ci) }, "the CI Watcher kept running after CI Tracking was turned off")
	if !alive(issue) {
		t.Fatal("turning CI Tracking off stopped the Issue Watcher")
	}
	issues(false)
	codecentraltest.Eventually(t, func() bool { return !alive(issue) }, "the Issue Watcher kept running after GitHub Issues was turned off")

	// Stopping the service stops both.
	tracking(true)
	issues(true)
	ci = watcherPID(t, first, "CI Watcher", 2)
	issue = watcherPID(t, first, "Issue Watcher", 2)
	first.stop(t)
	if alive(ci) || alive(issue) {
		t.Fatal("a Watcher outlived the service")
	}

	// The service starts them again with itself.
	second, msg := startServe(t, store, first.addr)
	if second == nil {
		t.Fatalf("restart: %s", msg)
	}
	ci = watcherPID(t, second, "CI Watcher", 1)
	issue = watcherPID(t, second, "Issue Watcher", 1)
	svc.watcherState(t, "ci", "watching")
	svc.watcherState(t, "github_issues", "watching")

	// Killed outright, as a stopped Windows scheduled task is, the
	// service leaves no Watcher behind.
	if err := second.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-second.exited
	codecentraltest.Eventually(t, func() bool { return !alive(ci) }, "the CI Watcher outlived the killed service")
	codecentraltest.Eventually(t, func() bool { return !alive(issue) }, "the Issue Watcher outlived the killed service")
}

// watcherPID waits for p's log to say it started acme's n-th Watcher of
// kind, such as CI Watcher, and returns its pid.
func watcherPID(t *testing.T, p *process, kind string, n int) int {
	t.Helper()
	var pid int
	codecentraltest.Eventually(t, func() bool {
		m := started(kind).FindAllStringSubmatch(p.stderr.String(), -1)
		if len(m) < n {
			return false
		}
		pid, _ = strconv.Atoi(m[n-1][1])
		return true
	}, "serve didn't start a %s", kind)
	if !alive(pid) {
		t.Fatalf("the %s %d isn't running:\n%s", kind, pid, p.stderr)
	}
	return pid
}

// watcherState waits for the status of acme's Watcher in its settings,
// ci or github_issues, to be state.
func (s *service) watcherState(t *testing.T, settings, state string) {
	t.Helper()
	codecentraltest.Eventually(t, func() bool {
		resp, err := http.Get(s.url + "/api/projects/acme")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var p map[string]struct {
			Watcher struct {
				State string `json:"state"`
			} `json:"watcher"`
		}
		json.NewDecoder(resp.Body).Decode(&p)
		return p[settings].Watcher.State == state
	}, "the %s Watcher's status isn't %s", settings, state)
}

// The service runs a Pull Request Watcher for a Project under Pull Request,
// whether or not a Factory runs, stops it when the Project switches away
// or is archived, and restarts it after a crash.
func TestTheServiceSupervisesThePullRequestWatcher(t *testing.T) {
	ghData := t.TempDir()
	t.Setenv("FAKE_GH_DIR", ghData)
	if err := fakegh.Script(ghData, fakegh.Answer{Args: []string{"api", "repos/acme/web"},
		Stdout: `{"full_name":"acme/web","permissions":{"push":true,"pull":true}}`}); err != nil {
		t.Fatal(err)
	}
	p, msg := startServe(t, t.TempDir(), "127.0.0.1:0")
	if p == nil {
		t.Fatalf("serve: %s", msg)
	}
	svc := &service{url: "http://" + p.addr}
	svc.human(t, "/api/projects/create_project", `{"key":"acme"}`)
	gh, _ := json.Marshal(map[string]string{"gh_path": fakeGHBin})
	svc.human(t, "/api/set_options", string(gh))
	svc.human(t, "/api/p/acme/report_push_remote", `{"url":"git@github.com:acme/web.git","base":"main"}`)
	method := func(m string) {
		svc.human(t, "/api/p/acme/set_landing_method", `{"method":"`+m+`","push":false,"remote":"origin"}`)
	}

	// Turning Pull Request on runs the check, then starts the watcher,
	// which reports Watching with no Factory running.
	method("pull_request")
	first := watcherPID(t, p, "Pull Request Watcher", 1)
	svc.watcherState(t, "landing", "watching")

	// Switching away stops it.
	method("branch")
	codecentraltest.Eventually(t, func() bool { return !alive(first) }, "the Pull Request Watcher kept running after switching away")

	// A crash is restarted.
	method("pull_request")
	second := watcherPID(t, p, "Pull Request Watcher", 2)
	proc, err := os.FindProcess(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	third := watcherPID(t, p, "Pull Request Watcher", 3)
	// The service's crash report holds for the streak, as for the others.
	svc.watcherState(t, "landing", "error")

	// Archiving the Project stops it.
	svc.human(t, "/api/p/acme/archive_project", `{}`)
	codecentraltest.Eventually(t, func() bool { return !alive(third) }, "the Pull Request Watcher kept running after the Project was archived")
}
