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
	"sync/atomic"
	"time"

	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/httpapi"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/server"
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
	store   store.Store
	clock   *clock.Fake
	handler http.Handler

	mu       sync.Mutex
	requests map[*request]struct{}
	dropped  bool
	// others are the other services on the world's Store, by host.
	others map[string]*core.Core
	// The answers the stand-in checks give; the zero one passes.
	ciCheck, issuesCheck, azureBoardsCheck, pullRequestCheck core.WatcherCheckResult
}

// request is one request being served, for ending it.
type request struct {
	cancel context.CancelFunc
	// stream is the httpapi stream pattern it follows, if any.
	stream string
	// unread counts the bytes its handler has written, or is writing,
	// that its response body's reader has yet to read.
	unread atomic.Int64
}

// streams tells which requests follow a stream: the change-notice feed and
// Transcript streams.
var streams = func() *http.ServeMux {
	mux := http.NewServeMux()
	for _, pattern := range []string{httpapi.EventsPath, httpapi.TranscriptStreamPath, httpapi.ChatStreamPath} {
		mux.Handle(pattern, http.NotFoundHandler())
	}
	return mux
}()

// Host is the machine the world's own service runs on.
const Host = "laptop"

// New is a fresh world whose fake clock reads start.
func New(start time.Time) *World {
	clk := clock.NewFake(start)
	s := store.NewMemory()
	c := core.New(s, clk, &ids.Sequence{})
	c.SetUsage(usage.NewMemory(clk, time.UTC, nil))
	c.SetService(core.Service{ID: Host, Host: Host, Version: Version})
	c.Register(context.Background()) // a fresh memory Store refuses no Registration
	w := &World{
		core:     c,
		store:    s,
		clock:    clk,
		handler:  server.Versioned(server.Handler(c, nil, nil), Version),
		requests: map[*request]struct{}{},
		others:   map[string]*core.Core{},
	}
	c.SetCICheck(func(context.Context, core.CICheckInput) core.WatcherCheckResult { return w.answer(&w.ciCheck) })
	c.SetIssuesCheck(func(context.Context, core.IssuesCheckInput) core.WatcherCheckResult { return w.answer(&w.issuesCheck) })
	c.SetAzureBoardsCheck(func(context.Context, core.AzureBoardsCheckInput) core.WatcherCheckResult {
		return w.answer(&w.azureBoardsCheck)
	})
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

// SetAzureBoardsCheck makes the Azure Boards check, which calls Azure
// DevOps in serve, answer res, as SetCICheck does the CI Watcher's.
func (w *World) SetAzureBoardsCheck(res core.WatcherCheckResult) {
	w.setAnswer(&w.azureBoardsCheck, res)
}

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

// HoldWatcherLease has the service on host hold key's Watcher Lease, as
// serve's Watcher supervisor does on each renewal: the world's own service
// on Host, or another service on the same Store, which takes it if it is
// free. The world's service then renews it or sees where it is held, as
// its supervisor would.
func (w *World) HoldWatcherLease(key, host string) {
	ctx := context.Background()
	if host != Host {
		w.service(host).HoldWatcherLease(ctx, key)
	}
	w.core.HoldWatcherLease(ctx, key)
}

// PostSessionEventsOn posts a session's Session Events to the service on
// host, as a Factory reporting to it does: the world's own service on
// Host, or another service on the same Store, which then holds the
// Transcript, so the world's own names its host.
func (w *World) PostSessionEventsOn(host, sessionID string, events []core.SessionEvent) {
	c := w.core
	if host != Host {
		c = w.service(host)
	}
	c.PostSessionEvents(context.Background(), sessionID, events)
}

// service is the other service on host, on the world's Store: made the
// first time, and registered, or its Registration renewed, each time.
func (w *World) service(host string) *core.Core {
	w.mu.Lock()
	other := w.others[host]
	if other == nil {
		other = core.New(w.store, w.clock, &ids.Sequence{})
		other.SetService(core.Service{ID: host, Host: host, Version: Version})
		w.others[host] = other
	}
	w.mu.Unlock()
	other.Register(context.Background())
	return other
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

// Subscribed counts the feeds a client has subscribed, of each stream
// kind: the change-notice feed, Transcript streams and Chat streams.
type Subscribed struct {
	Events, Transcripts, Chats int
}

// Busy is what keeps a world from being settled (ADR 0044); its zero
// value, an empty report, is a settled world.
type Busy struct {
	// Requests counts the plain requests, those not streams, in flight:
	// each from RoundTrip until its handler has returned, which is only
	// once its body has been read whole.
	Requests int `json:"requests,omitempty"`
	// Events, Transcripts and Chats are each kind's open streams against
	// its subscribed feeds, where the two differ. A stream is open from
	// RoundTrip until its handler has returned, so one still closing
	// counts.
	Events      *Streams `json:"events,omitempty"`
	Transcripts *Streams `json:"transcripts,omitempty"`
	Chats       *Streams `json:"chats,omitempty"`
	// UnreadWrites counts the open streams with a write their reader has
	// yet to read.
	UnreadWrites int `json:"unreadWrites,omitempty"`
}

// Streams is one stream kind's open streams against its subscribed feeds.
type Streams struct {
	Open       int `json:"open"`
	Subscribed int `json:"subscribed"`
}

// Settled reports what keeps the world from being settled, given the
// feeds its client has subscribed, or an empty report when it is. It reads
// the world as it stands: under js/wasm Go hands JavaScript control only
// once every goroutine is blocked, so a stream with a notice queued has
// already written it and waits on its reader, and no feed needs a count
// of its own.
func (w *World) Settled(subscribed Subscribed) Busy {
	w.mu.Lock()
	defer w.mu.Unlock()
	var busy Busy
	open := map[string]int{}
	for r := range w.requests {
		if r.stream == "" {
			busy.Requests++
			continue
		}
		open[r.stream]++
		if r.unread.Load() > 0 {
			busy.UnreadWrites++
		}
	}
	busy.Events = busyStreams(open[httpapi.EventsPath], subscribed.Events)
	busy.Transcripts = busyStreams(open[httpapi.TranscriptStreamPath], subscribed.Transcripts)
	busy.Chats = busyStreams(open[httpapi.ChatStreamPath], subscribed.Chats)
	return busy
}

// busyStreams is one kind's open streams against its subscribed feeds
// when the two differ, and nil when they match.
func busyStreams(open, subscribed int) *Streams {
	if open == subscribed {
		return nil
	}
	return &Streams{Open: open, Subscribed: subscribed}
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
