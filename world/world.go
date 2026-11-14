// Package world builds one WASM Core world (Spec code-central-580): a
// fresh Core over the in-memory store, a fake clock and in-memory Usage,
// behind the HTTP API and the MCP front door as serve mounts them, with
// the controls that stand for what lies outside Core. It builds on every
// platform, so it is tested with go test; cmd/corewasm only binds it to
// syscall/js.
package world

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/frontdoor"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

// Version is the service version a world reports, an unstamped CLI's.
const Version = "dev"

// ErrDropped is RoundTrip's error once the world has been dropped.
var ErrDropped = errors.New("the world has been dropped")

// World is one Core and its controls. It is an http.RoundTripper (see
// RoundTrip): each request is served by the world's handler, its response
// streaming as the handler writes it. Its controls stand only for what
// lies outside Core, each calling the entry point serve's own code calls;
// none seeds or writes state.
type World struct {
	core    *core.Core
	clock   *clock.Fake
	handler http.Handler

	mu       sync.Mutex
	requests map[*request]struct{}
	dropped  bool
	// The answers the stand-in gh checks give; the zero one passes.
	ciCheck, issuesCheck, pullRequestCheck core.WatcherCheckResult
}

// request is one request being served, for ending it.
type request struct {
	cancel context.CancelFunc
	// stream is the httpapi stream pattern it follows, if any.
	stream string
}

// streams tells which requests follow a stream: the change-notice feed and
// Transcript streams.
var streams = func() *http.ServeMux {
	mux := http.NewServeMux()
	for _, pattern := range []string{httpapi.EventsPath, httpapi.TranscriptStreamPath} {
		mux.Handle(pattern, http.NotFoundHandler())
	}
	return mux
}()

// New is a fresh world whose fake clock reads start.
func New(start time.Time) *World {
	clk := clock.NewFake(start)
	c := core.New(store.NewMemory(), clk, &ids.Sequence{})
	c.SetUsage(usage.NewMemory(clk, time.UTC, nil))
	w := &World{
		core:     c,
		clock:    clk,
		handler:  frontdoor.Versioned(frontdoor.Handler(c, nil, nil), Version),
		requests: map[*request]struct{}{},
	}
	c.SetCICheck(func(context.Context, core.CICheckInput) core.WatcherCheckResult { return w.answer(&w.ciCheck) })
	c.SetIssuesCheck(func(context.Context, core.IssuesCheckInput) core.WatcherCheckResult { return w.answer(&w.issuesCheck) })
	c.SetPullRequestCheck(func(context.Context, core.PullRequestCheckInput) core.WatcherCheckResult {
		return w.answer(&w.pullRequestCheck)
	})
	return w
}

// Now is the time on the world's clock.
func (w *World) Now() time.Time { return w.clock.Now() }

// Advance moves the world's clock forward by d.
func (w *World) Advance(d time.Duration) { w.clock.Advance(d) }

// SetCICheck makes the CI Watcher's check mode, which runs gh in serve,
// answer res: a Problem, whose reason and fix Core refuses with on the
// field the Problem picks, or none to pass.
func (w *World) SetCICheck(res core.WatcherCheckResult) { w.setAnswer(&w.ciCheck, res) }

// SetIssuesCheck makes the Issue Watcher's check mode answer res, as
// SetCICheck does the CI Watcher's.
func (w *World) SetIssuesCheck(res core.WatcherCheckResult) { w.setAnswer(&w.issuesCheck, res) }

// SetPullRequestCheck makes the Pull Request Watcher's check mode answer
// res, as SetCICheck does the CI Watcher's.
func (w *World) SetPullRequestCheck(res core.WatcherCheckResult) {
	w.setAnswer(&w.pullRequestCheck, res)
}

// setAnswer and answer set and read one stand-in check's answer, which a
// request's check reads while a test may set it.
func (w *World) setAnswer(check *core.WatcherCheckResult, res core.WatcherCheckResult) {
	w.mu.Lock()
	defer w.mu.Unlock()
	*check = res
}

func (w *World) answer(check *core.WatcherCheckResult) core.WatcherCheckResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	return *check
}

// ReportWatcherCrash reports that key's Watcher of kind crashed, as serve's
// Watcher supervisor does.
func (w *World) ReportWatcherCrash(kind core.WatcherKind, key string, crash core.WatcherCrash) {
	w.core.ReportWatcherCrash(context.Background(), kind, key, crash)
}

// DropStreams ends every open stream, change notices and Transcript
// streams alike, as a dropped connection would; the world serves on.
func (w *World) DropStreams() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for r := range w.requests {
		if r.stream != "" {
			r.cancel()
		}
	}
}

// TranscriptStreams counts the Transcript streams open now.
func (w *World) TranscriptStreams() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for r := range w.requests {
		if r.stream == httpapi.TranscriptStreamPath {
			n++
		}
	}
	return n
}

// Drop ends the world: every request it is serving ends, and it serves no
// more, so nothing holds it once its owner lets it go.
func (w *World) Drop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dropped = true
	for r := range w.requests {
		r.cancel()
	}
}
