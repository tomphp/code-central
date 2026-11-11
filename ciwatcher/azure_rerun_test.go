// Azure Pipelines Re-run requests and throttling: the CI Watcher retries a
// failed build through the fake Azure DevOps, a new attempt clears the
// request, a refusal lapses it, and throttling is Watching, not an error.

package ciwatcher_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/internal/fakeazdo"
)

// retries are the requests that retried build 11.
func (a *azureWatching) retries() []fakeazdo.Request {
	var out []fakeazdo.Request
	for _, r := range a.azdo.Requests() {
		if r.Method == http.MethodPatch && strings.HasSuffix(r.Path, "/builds/11") {
			out = append(out, r)
		}
	}
	return out
}

// failedAndRerun pushes a commit whose build 11 failed, and asks for a
// Re-run.
func (a *azureWatching) failedAndRerun() string {
	a.t.Helper()
	id := a.pushed("abc123")
	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "failed", changed: "2026-01-05T09:05:00Z", finished: "2026-01-05T09:05:00Z"})
	a.timeline(11, azRecord{"Job", "test", "failed", 1})
	a.startAzure()
	a.ciResult(id, core.CIFailed)
	human := core.WithActor(context.Background(), core.Human)
	must(a.t, func() error {
		_, err := a.srv.Core.RerunCI(human, "acme", core.TicketInput{Ticket: core.Ref(id)})
		return err
	})
	return id
}

func TestTheCIWatcherRetriesAnAzureBuildAndTheNewAttemptClearsTheRequest(t *testing.T) {
	a := newAzureWatching(t)
	id := a.failedAndRerun()
	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "inProgress", changed: "2026-01-05T09:07:00Z"})
	a.timeline(11, azRecord{"Job", "test", "", 2})
	a.poll()
	a.ciResult(id, core.CIRunning)

	var tk core.TicketView
	a.waitFor("attempt 2 reported", func() bool {
		var err error
		if tk, err = a.srv.Client.GetTicket(context.Background(), "acme", core.Ref(id)); err != nil {
			t.Fatal(err)
		}
		for _, r := range tk.CIRuns {
			if r.ID == 11 && r.Attempt == 2 {
				return true
			}
		}
		return false
	})
	if rs := a.retries(); len(rs) != 1 || !strings.Contains(rs[0].Body, `"retry":true`) || !strings.HasPrefix(rs[0].Authorization, "Basic ") {
		t.Fatalf("retries = %+v, want one PATCH with retry, as Basic auth", rs)
	}
	watch, err := a.srv.Client.GetCIWatch(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(watch.Reruns) != 0 {
		t.Fatalf("reruns = %+v, want the request cleared by attempt 2", watch.Reruns)
	}

	// Asked once, not again while the new attempt runs.
	a.poll()
	if n := len(a.retries()); n != 1 {
		t.Fatalf("%d retries, want the request carried out once", n)
	}
	a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "succeeded", changed: "2026-01-05T09:09:00Z", finished: "2026-01-05T09:09:00Z"})
	a.timeline(11, azRecord{"Job", "test", "succeeded", 2})
	a.poll()
	a.ciResult(id, core.CIPassed)
}

func TestAnAzureRetryTheServiceRefusesIsTriedAgainUntilTheRequestLapses(t *testing.T) {
	a := newAzureWatching(t)
	a.azdo.Script(fakeazdo.Answer{Method: "PATCH", Path: buildsPath + "/11", Status: http.StatusForbidden, Body: `{"message":"needs execute"}`})
	id := a.failedAndRerun()
	a.poll()
	a.waitFor("the first refused retry", func() bool { return len(a.retries()) == 1 })
	a.ciResult(id, core.CIRunning)

	a.poll()
	a.waitFor("a retry on the next poll", func() bool { return len(a.retries()) == 2 })
	a.ciResult(id, core.CIRunning)

	a.srv.Clock.Advance(11 * time.Minute)
	a.ciResult(id, core.CIFailed)
	if p, err := a.srv.Client.GetProject(context.Background(), "acme"); err != nil || p.CI.Watcher.State != core.WatcherWatching {
		t.Fatalf("watcher = %+v, %v, want Watching: a refused retry isn't a watcher error", p.CI.Watcher, err)
	}
	for _, r := range a.azdo.Requests() {
		if strings.Contains(r.Body, secretPAT) {
			t.Fatalf("request %+v carries the PAT in its body", r)
		}
	}
}

func TestAThrottledAzureWatcherCallsNothingUntilRetryAfterThenResumes(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			a := newAzureWatching(t)
			id := a.pushed("abc123")
			a.azdo.Script(fakeazdo.Answer{Method: "GET", Path: buildsPath, Status: status, Body: `{}`}.WithHeader("Retry-After", "90"))
			start := a.srv.Clock.Now()
			a.startAzure()
			a.rateLimitedUntil(start.Add(90 * time.Second))
			a.noErrors()

			calls := len(a.azdo.Requests())
			a.srv.Clock.BlockUntil(1)
			a.srv.Clock.Advance(89 * time.Second)
			if got := len(a.azdo.Requests()); got != calls {
				t.Fatalf("%d calls before Retry-After, want none after the throttled %d", got, calls)
			}

			a.builds(azBuild{id: 11, pipeline: "CI", commit: "abc123", status: "completed", result: "succeeded", changed: "2026-01-05T09:05:00Z", finished: "2026-01-05T09:05:00Z"})
			a.timeline(11, azRecord{"Job", "test", "succeeded", 1})
			a.srv.Clock.Advance(time.Second)
			a.ciResult(id, core.CIPassed)
			a.rateLimitedUntil(time.Time{})
			a.noErrors()
		})
	}
}
