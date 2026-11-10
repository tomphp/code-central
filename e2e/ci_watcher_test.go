//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/tomoram/code-central/codecentraltest"
)

// started is serve's log line for a CI Watcher it started, with its pid.
var started = regexp.MustCompile(`started the CI Watcher for acme \(pid (\d+)\)`)

// The service runs a CI Watcher for a Project with CI Tracking on, as
// its own executable, stops it on turn-off and when it stops, and leaves
// none behind when it is killed: the CI Watcher exits when its stdin, a
// pipe from the service, closes, as a stopped Windows scheduled task
// doesn't kill the service's children.
func TestTheServiceSupervisesItsCIWatcher(t *testing.T) {
	store := t.TempDir()
	first, msg := startServe(t, store, "127.0.0.1:0")
	if first == nil {
		t.Fatalf("serve: %s", msg)
	}
	svc := &service{url: "http://" + first.addr}
	svc.human(t, "/api/projects/create_project", `{"key":"acme"}`)
	// The fake gh, answering yes to everything, through the gh path Option.
	gh, _ := json.Marshal(map[string]string{"gh_path": fakeGHBin})
	svc.human(t, "/api/set_options", string(gh))
	tracking := func(on bool) {
		svc.human(t, "/api/p/acme/set_ci_tracking", `{"tracking":`+strconv.FormatBool(on)+`,"repository":"acme/web"}`)
	}

	// Turning CI Tracking on starts it, and it reports Watching.
	tracking(true)
	pid := watcherPID(t, first, 1)
	svc.watcherState(t, "watching")

	// Turning it off stops it.
	tracking(false)
	codecentraltest.Eventually(t, func() bool { return !alive(pid) }, "the CI Watcher kept running after CI Tracking was turned off")

	// Stopping the service stops it.
	tracking(true)
	pid = watcherPID(t, first, 2)
	first.stop(t)
	if alive(pid) {
		t.Fatal("the CI Watcher outlived the service")
	}

	// The service starts it again with itself.
	second, msg := startServe(t, store, first.addr)
	if second == nil {
		t.Fatalf("restart: %s", msg)
	}
	pid = watcherPID(t, second, 1)
	svc.watcherState(t, "watching")

	// Killed outright, as a stopped Windows scheduled task is, the
	// service leaves no CI Watcher behind.
	if err := second.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-second.exited
	codecentraltest.Eventually(t, func() bool { return !alive(pid) }, "the CI Watcher outlived the killed service")
}

// watcherPID waits for p's log to say it started acme's n-th CI Watcher,
// and returns its pid.
func watcherPID(t *testing.T, p *process, n int) int {
	t.Helper()
	var pid int
	codecentraltest.Eventually(t, func() bool {
		m := started.FindAllStringSubmatch(p.stderr.String(), -1)
		if len(m) < n {
			return false
		}
		pid, _ = strconv.Atoi(m[n-1][1])
		return true
	}, "serve didn't start a CI Watcher")
	if !alive(pid) {
		t.Fatalf("the CI Watcher %d isn't running:\n%s", pid, p.stderr)
	}
	return pid
}

// watcherState waits for acme's CI Watcher status to be state.
func (s *service) watcherState(t *testing.T, state string) {
	t.Helper()
	codecentraltest.Eventually(t, func() bool {
		resp, err := http.Get(s.url + "/api/projects/acme")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var p struct {
			CI struct {
				Watcher struct {
					State string `json:"state"`
				} `json:"watcher"`
			} `json:"ci"`
		}
		json.NewDecoder(resp.Body).Decode(&p)
		return p.CI.Watcher.State == state
	}, "the CI Watcher's status isn't %s", state)
}
