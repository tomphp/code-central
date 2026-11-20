package core_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/events"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
)

// Update functions are safe to retry (Spec code-central-1032): a Store
// shared by several services runs an Update's function again when its
// transaction conflicts, so every attempt but the last is thrown away
// after its write. Each operation must then do what one attempt does:
// one comment, one notice, one Watcher reset.

// attempts is how a retryingStore runs an Update.
type attempts int

const (
	// once runs it once, as the File Store does.
	once attempts = iota
	// twice fails the first attempt after its write, and commits the
	// second.
	twice
	// never fails every attempt after its write, until they run out.
	never
)

// retryingStore is a Store that retries an Update on conflict, as one
// shared by several services does, over the in-memory fake.
type retryingStore struct {
	store.Store
	attempts attempts
	// announced are the Changes committed Updates announced.
	announced []store.Change
}

func (s *retryingStore) Update(fn func(store.Writer) error) error {
	if s.attempts != once {
		if err := s.Store.Update(func(w store.Writer) error { return fn(&abandonedWriter{Writer: w}) }); err != nil {
			return err
		}
		if s.attempts == never {
			return store.ErrConflict
		}
	}
	return s.Store.Update(func(w store.Writer) error { return fn(announcingWriter{w, s}) })
}

// abandonedWriter is an attempt that fails after its write: it keeps the
// one-write rule, and nothing it writes or announces is committed.
type abandonedWriter struct {
	store.Writer
	wrote bool
}

func (w *abandonedWriter) write() error {
	if w.wrote {
		return store.ErrSecondWrite
	}
	w.wrote = true
	return nil
}

func (w *abandonedWriter) PutProject(domain.Project) error                      { return w.write() }
func (w *abandonedWriter) PutSpec(string, domain.Spec) error                    { return w.write() }
func (w *abandonedWriter) PutTicket(string, domain.Ticket, ...store.Part) error { return w.write() }
func (w *abandonedWriter) PutImport(string, store.Import) error                 { return w.write() }
func (w *abandonedWriter) PutOptions(domain.Options) error                      { return w.write() }
func (w *abandonedWriter) PutCIRuns(string, []domain.CIRun) error               { return w.write() }
func (w *abandonedWriter) Announce(store.Change)                                {}

// announcingWriter is the attempt that commits, recording what it
// announces.
type announcingWriter struct {
	store.Writer
	s *retryingStore
}

func (w announcingWriter) Announce(ch store.Change) { w.s.announced = append(w.s.announced, ch) }

// retryFixture is a fixture over a retryingStore, running each Update
// once until the test says otherwise.
func retryFixture() (fixture, *retryingStore) {
	mem := store.NewMemory()
	rs := &retryingStore{Store: mem}
	clk := clock.NewFake(t0)
	return fixture{core: core.New(rs, clk, &ids.Sequence{}), clock: clk, store: mem}, rs
}

// retryOutcome is what an operation leaves: its result, the notices it
// sent, the Changes it announced, and the Project and acme-2 after it.
type retryOutcome struct {
	Result    string
	Notices   []events.Event
	Announced []store.Change
	Project   string
	Ticket    string
}

// runAttempts sets up a fresh retryFixture with setup, each Update once, and then
// runs op with its Updates run as a.
func runAttempts(t *testing.T, a attempts, setup func(t *testing.T, f fixture), op func(f fixture) (any, error)) (retryOutcome, error) {
	t.Helper()
	f, rs := retryFixture()
	setup(t, f)
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	rs.announced, rs.attempts = nil, a
	res, err := op(f)
	rs.attempts = once
	out := retryOutcome{Result: asText(t, res), Announced: rs.announced}
	for len(notices) > 0 {
		out.Notices = append(out.Notices, <-notices)
	}
	p, perr := f.core.GetProject(human, "acme")
	if perr != nil {
		t.Fatal(perr)
	}
	out.Project = asText(t, p)
	if tk, terr := f.core.GetTicket(ctx, "acme", api.GetTicketInput{Ticket: "2"}); terr == nil {
		out.Ticket = asText(t, tk)
	}
	return out, err
}

func asText(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// retryCase is an audited operation: setup builds the world, op runs it.
type retryCase struct {
	name  string
	setup func(t *testing.T, f fixture)
	op    func(f fixture) (any, error)
}

// prWatching is acme-2 Accepted under Pull Request with an open pull
// request, and the Pull Request Watcher Watching.
func prWatching(t *testing.T, f fixture) {
	f.pullRequestAccepted(t)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PROpen})
	if _, err := f.core.ReportPullRequestWatcherStatus(ctx, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
}

// ciWatching is itemWorld with acme-2 in it, and the CI Watcher Watching.
func ciWatching(t *testing.T, f fixture) {
	itemWorldOn(t, f)
	if _, err := f.core.ReportCIWatcherStatus(ctx, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
}

// issuesWatching is work in acme, with GitHub Issues on and the Issue
// Watcher Watching.
func issuesWatching(t *testing.T, f fixture) {
	f.work(t, domain.Ready)
	if _, err := f.core.SetGitHubIssues(human, "acme", api.SetGitHubIssuesInput{On: true, Repositories: []string{"acme/web"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.ReportIssueWatcherStatus(integration, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
}

// boardsWatching is work in acme, with Azure Boards on and the Work Item
// Watcher Watching.
func boardsWatching(t *testing.T, f fixture) {
	f.work(t, domain.Ready)
	if _, err := f.core.SetAzureBoards(human, "acme", api.SetAzureBoardsInput{On: true, QueryURL: azureBoardsQuery}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.ReportWorkItemWatcherStatus(integration, "acme", api.ReportWatcherStatusInput{State: api.WatcherWatching}); err != nil {
		t.Fatal(err)
	}
}

var retryCases = []retryCase{
	{"landing method change", prWatching, func(f fixture) (any, error) {
		return f.core.SetLandingMethod(human, "acme", api.SetLandingMethodInput{Method: domain.LandingMerge, Remote: "origin"})
	}},
	{"CI Run report", ciWatching, func(f fixture) (any, error) {
		return f.core.ReportCIRuns(ctx, "acme", api.ReportCIRunsInput{Runs: []api.CIRunView{
			{ID: 1, Attempt: 1, Workflow: "ci.yml", Name: "ci.yml", Branch: "main", Commit: "abc123", Status: domain.CICompleted,
				Conclusion: domain.CIFailure, URL: "https://github.com/acme/web/actions/runs/1", Ticket: "acme-5"},
		}, Full: true})
	}},
	{"CI response", func(t *testing.T, f fixture) {
		ciWatching(t, f)
		f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)
	}, func(f fixture) (any, error) {
		return f.core.RerunCI(human, "acme", api.CIResponseInput{Ticket: "acme-5"})
	}},
	{"pushed-commit report", func(t *testing.T, f fixture) {
		ciWatching(t, f)
		f.moveTo(t, 2, domain.Accepted)
	}, func(f fixture) (any, error) {
		return f.core.ReportPushedCommit(ctx, "acme", api.ReportPushedCommitInput{Ticket: "2", Commit: "abc123", Branch: "main"})
	}},
	{"pull request report", func(t *testing.T, f fixture) { f.pullRequestAccepted(t) }, func(f fixture) (any, error) {
		return f.core.ReportPullRequest(ctx, "acme", api.ReportPullRequestInput{Branch: "acme-2", State: domain.PROpen, Number: 7, Link: "https://github.com/acme/web/pull/7"})
	}},
	{"setting CI", ciWatching, func(f fixture) (any, error) {
		return f.core.SetCITracking(human, "acme", api.SetCITrackingInput{Tracking: false})
	}},
	{"reporting the push remote", ciWatching, func(f fixture) (any, error) {
		return f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{})
	}},
	{"CI rule edit", func(t *testing.T, f fixture) {
		ciWatching(t, f)
		if _, err := f.core.AddCIRule(human, "acme", api.CIRuleInput{Workflow: "ci.yml", Branch: "main", State: domain.Ready}); err != nil {
			t.Fatal(err)
		}
	}, func(f fixture) (any, error) {
		return f.core.EditCIRule(human, "acme", api.CIRuleInput{Spec: "acme-9", Workflow: "ci.yml", Branch: "release"})
	}},
	{"turning GitHub Issues on", func(t *testing.T, f fixture) { f.work(t, domain.Ready) }, func(f fixture) (any, error) {
		return f.core.SetGitHubIssues(human, "acme", api.SetGitHubIssuesInput{On: true, Repositories: []string{"acme/web"}})
	}},
	{"setting GitHub Issues", issuesWatching, func(f fixture) (any, error) {
		return f.core.SetGitHubIssues(human, "acme", api.SetGitHubIssuesInput{On: false, Repositories: []string{"acme/web"}})
	}},
	{"setting Azure Boards", boardsWatching, func(f fixture) (any, error) {
		return f.core.SetAzureBoards(human, "acme", api.SetAzureBoardsInput{On: false, QueryURL: azureBoardsQuery})
	}},
	{"claim", func(t *testing.T, f fixture) { f.work(t, domain.Ready) }, func(f fixture) (any, error) {
		return f.core.Claim(ctx, "acme", api.ClaimInput{Ticket: "2", SessionID: "sess-1"})
	}},
}

const azureBoardsQuery = "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10"

// Each audited operation, retried, leaves what one attempt does: the
// same result, notices, announced Changes, Project (its Watchers'
// statuses with it) and Ticket.
func TestARetriedUpdateDoesWhatOneAttemptDoes(t *testing.T) {
	for _, c := range retryCases {
		t.Run(c.name, func(t *testing.T) {
			want, err := runAttempts(t, once, c.setup, c.op)
			if err != nil {
				t.Fatal(err)
			}
			if len(want.Notices) == 0 || len(want.Announced) == 0 {
				t.Fatalf("one attempt sent notices %v and announced %v, want some of each", want.Notices, want.Announced)
			}
			got, err := runAttempts(t, twice, c.setup, c.op)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("retried:\n%+v\nwant, as one attempt:\n%+v", got, want)
			}
		})
	}
}

// An operation whose attempts all conflict fails with ErrConflict and
// leaves nothing: no notice, no Change, no Watcher reset.
func TestAnUpdateThatKeepsConflictingLeavesNothing(t *testing.T) {
	for _, c := range retryCases {
		t.Run(c.name, func(t *testing.T) {
			want, err := runAttempts(t, once, c.setup, func(fixture) (any, error) { return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			got, err := runAttempts(t, never, c.setup, c.op)
			if !errors.Is(err, store.ErrConflict) {
				t.Fatalf("error %v, want store.ErrConflict", err)
			}
			if len(got.Notices) != 0 || len(got.Announced) != 0 {
				t.Fatalf("sent notices %v and announced %v, want none", got.Notices, got.Announced)
			}
			if got.Project != want.Project || got.Ticket != want.Ticket {
				t.Fatalf("left:\n%s\n%s\nwant as before:\n%s\n%s", got.Project, got.Ticket, want.Project, want.Ticket)
			}
		})
	}
}

// The Landing Method change away from Pull Request, retried: one comment
// on the Ticket whose pull request is left open, one notice for it per
// write, and the Pull Request Watcher reset once the change commits.
func TestARetriedLandingMethodChangeCommentsOnceAndResetsTheWatcher(t *testing.T) {
	f, rs := retryFixture()
	prWatching(t, f)
	comments := len(f.get(t, "2").Comments)
	notices, stop := f.core.Events().Subscribe()
	defer stop()
	rs.attempts = twice
	f.switchTo(t, domain.LandingMerge)
	rs.attempts = once
	tk := f.get(t, "2")
	if got := len(tk.Comments) - comments; got != 1 || !strings.Contains(tk.Comments[len(tk.Comments)-1].Body, "left open") {
		t.Fatalf("%d new comments, want 1 saying the pull request is left open", got)
	}
	seen := map[events.Event]int{}
	for len(notices) > 0 {
		seen[<-notices]++
	}
	// The comment's write, and the method's bearing on its Needs Me reason.
	if got := seen[events.Event{Type: events.TicketChanged, Project: "acme", ID: "acme-2"}]; got != 3 {
		t.Fatalf("%d notices for acme-2, want 3: the comment, and one each for being Accepted and leaving a pull request open", got)
	}
	if got := f.switchTo(t, domain.LandingPullRequest).Landing.Watcher.State; got != api.WatcherNotReported {
		t.Fatalf("Pull Request Watcher %q, want not reported: what it said was about the old method", got)
	}
}

// A setting's write that resets its Watcher's status returns the Project
// with the status reset, though the reset waits for the write to commit.
func TestASettingsResultShowsItsWatcherReset(t *testing.T) {
	for _, c := range []struct {
		name    string
		setup   func(t *testing.T, f fixture)
		op      func(f fixture) (api.Written[api.ProjectView], error)
		watcher func(p api.ProjectView) api.WatcherView
	}{
		{"setting CI", ciWatching, func(f fixture) (api.Written[api.ProjectView], error) {
			return f.core.SetCITracking(human, "acme", api.SetCITrackingInput{Tracking: false, Full: true})
		}, func(p api.ProjectView) api.WatcherView { return p.CI.Watcher }},
		{"reporting the push remote", ciWatching, func(f fixture) (api.Written[api.ProjectView], error) {
			return f.core.ReportPushRemote(ctx, "acme", api.ReportPushRemoteInput{Full: true})
		}, func(p api.ProjectView) api.WatcherView { return p.CI.Watcher }},
		{"setting GitHub Issues", issuesWatching, func(f fixture) (api.Written[api.ProjectView], error) {
			return f.core.SetGitHubIssues(human, "acme", api.SetGitHubIssuesInput{On: true, Repositories: []string{"acme/site"}, Full: true})
		}, func(p api.ProjectView) api.WatcherView { return p.GitHubIssues.Watcher }},
		{"setting Azure Boards", boardsWatching, func(f fixture) (api.Written[api.ProjectView], error) {
			return f.core.SetAzureBoards(human, "acme", api.SetAzureBoardsInput{On: false, QueryURL: azureBoardsQuery, Full: true})
		}, func(p api.ProjectView) api.WatcherView { return p.AzureBoards.Watcher }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, rs := retryFixture()
			c.setup(t, f)
			rs.attempts = twice
			v, err := c.op(f)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.watcher(*v.Record).State; got != api.WatcherNotReported {
				t.Fatalf("the result's Watcher is %q, want not reported", got)
			}
		})
	}
}
