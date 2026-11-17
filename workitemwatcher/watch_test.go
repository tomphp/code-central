package workitemwatcher_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/ciwatcher/ciwatchertest"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/gh"
	"github.com/tomoram/code-central/internal/fakeazdo"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/watchers"
	"github.com/tomoram/code-central/workitemwatcher"
)

func TestMain(m *testing.M) { ciwatchertest.Main(m) }

var human = core.WithActor(context.Background(), core.Human)

// queryID is the saved query Azure Boards runs, and pat the PAT in
// ADO_PAT.
const (
	queryID = "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"
	pat     = "s3cr3t-pat-value"
)

// watching is a real Code Central with Azure Boards on for acme, running
// the saved query queryID on the fake Azure DevOps, which matches nothing
// yet, and the Work Item Watcher's loop running against both, as the
// integration Actor, until the test ends or its stdin closes.
type watching struct {
	t    *testing.T
	fake *fakeazdo.Server
	srv  *codecentraltest.Server
	// environ is the Work Item Watcher's environment, with ADO_PAT set.
	environ []string
	// stdin is the Work Item Watcher's stdin; closing it stops the loop.
	stdin *io.PipeWriter
	done  chan error
}

func newWatching(t *testing.T) *watching {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	if _, err := srv.Core.CreateProject(human, core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	w := &watching{t: t, fake: fakeazdo.New(t), srv: srv, environ: []string{"ADO_PAT=" + pat}}
	w.fake.SavedQuery(queryID)
	w.setBoards(core.SetAzureBoardsInput{On: true, QueryURL: w.queryURL(queryID)})
	return w
}

// queryURL is the URL of the fake project's saved query id.
func (w *watching) queryURL(id string) string { return w.fake.ProjectURL() + "/_queries/query/" + id }

// setBoards is the human's set_azure_boards on acme.
func (w *watching) setBoards(in core.SetAzureBoardsInput) {
	w.t.Helper()
	if _, err := w.srv.Core.SetAzureBoards(human, "acme", in); err != nil {
		w.t.Fatal(err)
	}
}

// start runs the loop.
func (w *watching) start() {
	w.t.Helper()
	r, pw := io.Pipe()
	w.stdin, w.done = pw, make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	w.t.Cleanup(func() {
		cancel()
		pw.Close()
	})
	c := client.New(w.srv.URL)
	c.Actor = core.Integration
	go func() {
		w.done <- workitemwatcher.Watch(ctx, workitemwatcher.Config{
			Client: c, Project: "acme", Clock: w.srv.Clock, Stdin: r, Log: io.Discard, Env: gh.Env{Environ: w.environ},
		})
	}()
}

// settings is acme's Azure Boards settings, as the HTTP API reads them.
func (w *watching) settings() core.AzureBoardsView {
	w.t.Helper()
	p, err := w.srv.Client.GetProject(context.Background(), "acme")
	if err != nil {
		w.t.Fatal(err)
	}
	return p.AzureBoards
}

// status waits for the Work Item Watcher's status to be state, and
// returns it.
func (w *watching) status(state core.WatcherState) core.WatcherView {
	w.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(w.t, func() bool {
		got = w.settings().Watcher
		return got.State == state
	}, "the Work Item Watcher's status %+v, want %s", &got, state)
	return got
}

// nextPoll waits for the loop to wait out its interval, then ends it.
func (w *watching) nextPoll() {
	w.t.Helper()
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(workitemwatcher.Interval)
}

// polled waits out the interval, then waits for the poll it starts to
// end.
func (w *watching) polled() {
	w.t.Helper()
	w.nextPoll()
	w.srv.Clock.BlockUntil(1)
}

// errorCount is how many times the Work Item Watcher went into an error,
// each one Notification.
func (w *watching) errorCount() int {
	for _, a := range w.srv.Core.WatcherAttention(context.Background()) {
		if a.Kind == core.WatcherWorkItems && a.Project == "acme" {
			return a.Errors
		}
	}
	return 0
}

// queries is how many times the fake was asked to run a query.
func (w *watching) queries() int {
	n := 0
	for _, r := range w.fake.Requests() {
		if strings.Contains(r.Path, "/_apis/wit/wiql") {
			n++
		}
	}
	return n
}

func TestTheWorkItemWatcherReportsWatchingAndRunsTheQueryEvery2Minutes(t *testing.T) {
	w := newWatching(t)
	w.start()
	if got := w.status(core.WatcherWatching); got.Message != "" || got.Fix != "" || !got.RateLimitedUntil.IsZero() {
		t.Fatalf("status %+v, want watching with no message", got)
	}
	if r := w.fake.Requests()[0]; r.Path != fakeazdo.ProjectPath+"/_apis/wit/wiql/"+queryID || r.Authorization == "" {
		t.Fatalf("first request %+v, want the saved query run by its ID, with the PAT", r)
	}
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(workitemwatcher.Interval - time.Second)
	if got := w.queries(); got != 1 {
		t.Fatalf("%d queries before 2 minutes, want 1", got)
	}
	w.srv.Clock.Advance(time.Second)
	codecentraltest.Eventually(t, func() bool { return w.queries() == 2 }, "the Work Item Watcher didn't poll again after 2 minutes")
}

// failing scripts Azure DevOps answering the next three requests on
// method and path with status, then answering as its board does.
func (w *watching) failing(method, path string, status int, message string) {
	a := fakeazdo.JSON(method, path, status, map[string]string{"message": message})
	a.Times = 3
	w.fake.Script(a)
}

// Each error has its own fix, and the Work Item Watcher retries each
// poll, still in error, so it sends one Notification, and goes back to
// Watching by itself once the cause is fixed.
func TestEachErrorIsReportedWithItsFixUntilItIsFixed(t *testing.T) {
	savedQuery := fakeazdo.ProjectPath + "/_apis/wit/wiql/" + queryID
	for _, c := range []struct {
		name        string
		cause       func(w *watching)
		problem     core.WatcherProblem
		message, to string
	}{
		{"rejected PAT", func(w *watching) { w.failing("GET", savedQuery, http.StatusUnauthorized, "") },
			core.ProblemPATRejected, "rejected the PAT in ADO_PAT", "Work Items: Read & write"},
		{"missing scope", func(w *watching) { w.failing("GET", savedQuery, http.StatusForbidden, "") },
			core.ProblemPATScope, "can't read work items", "Work Items: Read & write"},
		{"query not found", func(w *watching) {
			w.failing("GET", savedQuery, http.StatusNotFound, "TF401243: The query does not exist.")
		}, core.ProblemQueryUnusable, "can't find the saved query", "saved query's URL"},
		{"tree query", func(w *watching) {
			a := fakeazdo.JSON("GET", savedQuery, 200, map[string]any{"queryType": "tree", "workItemRelations": []any{}})
			a.Times = 3
			w.fake.Script(a)
		}, core.ProblemQueryUnusable, "a tree query", "flat list"},
		{"Azure DevOps failing", func(w *watching) { w.failing("GET", savedQuery, http.StatusInternalServerError, "Something broke") },
			core.ProblemCheckFailed, "Something broke", "status"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWatching(t)
			c.cause(w)
			w.start()
			got := w.status(core.WatcherError)
			if got.Problem != c.problem || !strings.Contains(got.Message, c.message) || !strings.Contains(got.Fix, c.to) {
				t.Fatalf("status %+v, want %s, %q, with the fix %q", got, c.problem, c.message, c.to)
			}
			if strings.Contains(got.Message+got.Fix, pat) {
				t.Fatalf("status %+v shows the PAT", got)
			}
			w.nextPoll()
			w.nextPoll()
			w.status(core.WatcherError)
			if n := w.errorCount(); n != 1 {
				t.Fatalf("went into an error %d times, want once, for one Notification", n)
			}
			w.nextPoll()
			w.status(core.WatcherWatching)
		})
	}
}

// The PAT is read from the environment the service gave the Work Item
// Watcher, so an unset variable needs a restart to fix.
func TestAMissingPATIsAnErrorNamingItsVariable(t *testing.T) {
	w := newWatching(t)
	w.environ = []string{"OTHER=x"}
	w.start()
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemPATMissing || !strings.Contains(got.Message, "ADO_PAT") || !strings.Contains(got.Fix, "Work Items: Read & write") || !strings.Contains(got.Fix, "restart") {
		t.Fatalf("status %+v, want pat_missing naming ADO_PAT, the scope and a restart", got)
	}
	if n := len(w.fake.Requests()); n != 0 {
		t.Fatalf("%d requests to Azure DevOps without a PAT, want none", n)
	}
}

// A WIQL filter's errors are Azure DevOps' own, and fixing the clause in
// Code Central brings the Work Item Watcher back to Watching.
func TestAWIQLFilterAzureDevOpsRefusesIsAnErrorWithItsMessage(t *testing.T) {
	w := newWatching(t)
	w.setBoards(core.SetAzureBoardsInput{On: true, Mode: "wiql", ProjectURL: w.fake.ProjectURL(), Where: "[Nope] = 1"})
	w.start()
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemQueryUnusable || !strings.Contains(got.Message, "TF51005: The query references a field that does not exist.") || !strings.Contains(got.Fix, "WHERE clause") {
		t.Fatalf("status %+v, want query_unusable with Azure DevOps' message", got)
	}
	w.fake.Filter("[System.State] = 'Approved'")
	w.setBoards(core.SetAzureBoardsInput{On: true, Mode: "wiql", ProjectURL: w.fake.ProjectURL(), Where: "[System.State] = 'Approved'"})
	w.nextPoll()
	w.status(core.WatcherWatching)
}

func TestAWIQLFiltersProjectNotFoundIsAnError(t *testing.T) {
	w := newWatching(t)
	w.setBoards(core.SetAzureBoardsInput{On: true, Mode: "wiql", ProjectURL: w.fake.ProjectURL(), Where: "[System.State] = 'Approved'"})
	w.failing("POST", fakeazdo.ProjectPath+"/_apis/wit/wiql", http.StatusNotFound, "TF200016: The project does not exist.")
	w.start()
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemProjectUnreadable || !strings.Contains(got.Message, w.fake.ProjectURL()) || !strings.Contains(got.Fix, "project URL") {
		t.Fatalf("status %+v, want project_unreadable naming the project", got)
	}
}

// More than 1,000 matches is an error that tells the human to narrow the
// query, and nothing arrives or leaves on that poll.
func TestAQueryMatchingMoreThan1000WorkItemsIsAnErrorThatChangesNothing(t *testing.T) {
	w := newWatching(t)
	w.arrived(1)
	many := make([]int, 0, 1001)
	for id := 2; id <= 1002; id++ {
		many = append(many, id)
	}
	w.fake.PutWorkItems(item(2))
	w.fake.SavedQuery(queryID, many...)
	from := len(w.fake.Requests())
	w.nextPoll()
	got := w.status(core.WatcherError)
	if got.Problem != core.ProblemTooManyWorkItems || !strings.Contains(got.Message, "1001") || !strings.Contains(got.Fix, "Narrow the query") {
		t.Fatalf("status %+v, want too_many_work_items naming 1001 and to narrow the query", got)
	}
	w.srv.Clock.BlockUntil(1)
	// Work item 1, which no longer matches, isn't out of scope, and
	// work item 2, which does, hasn't arrived.
	wantIncoming(t, w, 1)
	wantExternal(t, w, 1, store.ExternalOpen)
	if got := w.read(from); len(got) != 0 {
		t.Fatalf("read work items %v on a poll matching too many, want none", got)
	}

	w.fake.SavedQuery(queryID, 1, 2)
	w.nextPoll()
	w.status(core.WatcherWatching)
	w.srv.Clock.BlockUntil(1)
	wantIncoming(t, w, 1, 2)
}

// rateLimitedUntil waits for the Work Item Watcher to report Watching,
// rate-limited until until.
func (w *watching) rateLimitedUntil(until time.Time) {
	w.t.Helper()
	var got core.WatcherView
	codecentraltest.Eventually(w.t, func() bool {
		got = w.settings().Watcher
		return got.State == core.WatcherWatching && got.RateLimitedUntil.Equal(until)
	}, "the Work Item Watcher's status %+v, want Watching, rate-limited until %s", &got, until)
}

// Rate-limited, the Work Item Watcher is still Watching, with when it
// calls Azure DevOps again, makes no calls until then, and sends no
// Notification.
func TestARateLimitedWorkItemWatcherStaysWatchingAndBacksOff(t *testing.T) {
	w := newWatching(t)
	limit := fakeazdo.JSON("GET", fakeazdo.ProjectPath+"/_apis/wit/wiql/", http.StatusTooManyRequests, map[string]string{"message": "slow down"}).WithHeader("Retry-After", "600")
	limit.Times = 1
	w.fake.Script(limit)
	w.start()
	until := w.srv.Clock.Now().Add(10 * time.Minute)
	w.rateLimitedUntil(until)

	requests := len(w.fake.Requests())
	w.srv.Clock.BlockUntil(1)
	w.srv.Clock.Advance(10*time.Minute - time.Second)
	if got := len(w.fake.Requests()); got != requests {
		t.Fatalf("%d requests before Retry-After ran out, want none after the rate limit's %d", got, requests)
	}
	w.srv.Clock.Advance(time.Second)
	w.rateLimitedUntil(time.Time{})
	if n := w.errorCount(); n != 0 {
		t.Fatalf("went into an error %d times, want none: a rate limit sends no Notification", n)
	}
}

// A 503 with Retry-After is a rate limit too; with no time given, the
// interval doubles each poll, up to 10 minutes.
func TestA503WithRetryAfterIsARateLimitToo(t *testing.T) {
	w := newWatching(t)
	limit := fakeazdo.JSON("GET", fakeazdo.ProjectPath+"/_apis/wit/wiql/", http.StatusServiceUnavailable, nil).WithHeader("Retry-After", "soon")
	limit.Times = 3
	w.fake.Script(limit)
	w.start()
	for _, wait := range []time.Duration{4 * time.Minute, 8 * time.Minute, watchers.MaxRateLimitInterval} {
		w.rateLimitedUntil(w.srv.Clock.Now().Add(wait))
		w.srv.Clock.BlockUntil(1)
		w.srv.Clock.Advance(wait)
	}
	w.rateLimitedUntil(time.Time{})
	if n := w.errorCount(); n != 0 {
		t.Fatalf("went into an error %d times, want none: a rate limit sends no Notification", n)
	}
}

// Turned off, Azure Boards has no Work Item Watcher status to report:
// the loop reports nothing and calls Azure DevOps for nothing.
func TestTheWorkItemWatcherReportsNothingWithAzureBoardsOff(t *testing.T) {
	w := newWatching(t)
	w.setBoards(core.SetAzureBoardsInput{On: false, QueryURL: w.queryURL(queryID)})
	w.start()
	w.srv.Clock.BlockUntil(1)
	if reqs := w.fake.Requests(); len(reqs) != 0 {
		t.Fatalf("requests %+v with Azure Boards off, want none", reqs)
	}
	if got := w.settings().Watcher; got.State != core.WatcherNotReported {
		t.Fatalf("status %+v, want not reported", got)
	}
}

func TestTheWorkItemWatcherExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	w.start()
	w.status(core.WatcherWatching)
	w.stdin.Close()
	select {
	case err := <-w.done:
		if err != nil {
			t.Fatalf("Watch = %v, want nil on its stdin closing", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the Work Item Watcher kept running after its stdin closed")
	}
}

// The service runs the Work Item Watcher as its own executable's hidden
// subcommand, with a pipe for stdin and the PAT in its environment; it
// writes as the integration Actor, whose status report alone is
// accepted.
func TestTheWorkItemWatcherSubcommandWatchesAndExitsWhenItsStdinCloses(t *testing.T) {
	w := newWatching(t)
	h := ciwatchertest.New(t)
	cmd := exec.Command(h.CodeCentral, workitemwatcher.WatchArgs("acme")...)
	cmd.Env = append(h.Environ(), "CODE_CENTRAL_URL="+w.srv.URL, "ADO_PAT="+pat)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var exit error
	done := make(chan struct{})
	go func() {
		exit = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		cmd.Process.Kill()
		<-done
	})
	w.status(core.WatcherWatching)
	stdin.Close()
	select {
	case <-done:
		if exit != nil {
			t.Fatalf("the Work Item Watcher exited with %v, want 0", exit)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the Work Item Watcher kept running after its stdin closed")
	}
}

// The service supervises a Work Item Watcher for each Project with Azure
// Boards on, whether or not the Factory runs.
func TestTheServiceRunsAWorkItemWatcherWhileAzureBoardsIsOn(t *testing.T) {
	w := newWatching(t)
	if got := w.srv.Core.WatchedProjects(context.Background(), workitemwatcher.Kind.Kind); !slices.Equal(got, []string{"acme"}) {
		t.Fatalf("watched projects %v, want acme", got)
	}
	if args := workitemwatcher.Kind.Args("acme"); !slices.Equal(args, []string{"work-item-watcher", "--project", "acme"}) {
		t.Fatalf("args %v", args)
	}
}
