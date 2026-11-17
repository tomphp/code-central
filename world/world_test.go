package world_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/frontdoor"
	"github.com/tomoram/code-central/world"
)

var start = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

// newWorld is a fresh world, dropped when the test ends.
func newWorld(t *testing.T) *world.World {
	t.Helper()
	w := world.New(start)
	t.Cleanup(w.Drop)
	return w
}

// send makes one request to w, as actor unless actor is "", and returns
// its status and body.
func send(t *testing.T, w *world.World, actor, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if actor != "" {
		req.Header.Set(frontdoor.ActorHeader, actor)
	}
	res, err := w.RoundTrip(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the body: %v", method, path, err)
	}
	return res.StatusCode, string(b)
}

// ok sends a request that must answer 200 and returns its body.
func ok(t *testing.T, w *world.World, actor, method, path, body string) string {
	t.Helper()
	status, b := send(t, w, actor, method, path, body)
	if status != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, path, status, b)
	}
	return b
}

func TestARequestWithNoActorIsTheHumans(t *testing.T) {
	w := newWorld(t)
	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)

	ok(t, w, "", "POST", "/api/p/acme/rename_project", `{"display_name":"Acme"}`)
	if status, b := send(t, w, "agent", "POST", "/api/p/acme/rename_project", `{"display_name":"Acme"}`); status == http.StatusOK || !strings.Contains(b, "made as agent") {
		t.Errorf("rename_project as agent: %d %s, want it refused as a human action", status, b)
	}
	if b := ok(t, w, "", "GET", "/api/projects/acme", ""); !strings.Contains(b, `"display_name":"Acme"`) {
		t.Errorf("get_project: %s, want the new display name", b)
	}
}

// mcpCall calls tool on key's Project through the MCP front door, as an
// agent does, and returns the tool's text.
func mcpCall(t *testing.T, w *world.World, key, tool, args string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", "http://localhost/p/"+key+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+tool+`","arguments":`+args+`}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := w.RoundTrip(req)
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	defer res.Body.Close()
	var out struct {
		Result struct {
			Content []struct{ Text string }
			IsError bool
		}
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || len(out.Result.Content) == 0 || out.Result.IsError {
		t.Fatalf("%s: %d %+v %v", tool, res.StatusCode, out, err)
	}
	return out.Result.Content[0].Text
}

func TestAnAgentsMCPCallIsServedAndRecordedAsUsage(t *testing.T) {
	w := newWorld(t)
	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)

	if text := mcpCall(t, w, "acme", "create_spec", `{"title":"Billing"}`); !strings.Contains(text, "acme-1") {
		t.Fatalf("create_spec over MCP: %s, want acme-1", text)
	}
	if b := ok(t, w, "", "GET", "/api/usage?project=acme", ""); !strings.Contains(b, `"create_spec"`) {
		t.Errorf("Usage: %s, want the create_spec call", b)
	}
}

// claimedWorld is Project acme with acme-2 claimed by session s1, whose
// Claim 1 files its Usage and Session Events.
func claimedWorld(t *testing.T) *world.World {
	t.Helper()
	w := newWorld(t)
	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)
	ok(t, w, "agent", "POST", "/api/p/acme/create_spec", `{"title":"S"}`)
	ok(t, w, "agent", "POST", "/api/p/acme/create_ticket", `{"spec":1,"title":"A","state":"Ready"}`)
	ok(t, w, "agent", "POST", "/api/p/acme/claim", `{"ticket":2,"session_id":"s1"}`)
	return w
}

// postSessionEvent posts one text Session Event for session s1, as a
// Factory does.
func postSessionEvent(t *testing.T, w *world.World, text string) {
	t.Helper()
	if status, b := send(t, w, "integration", "POST", "/api/session-events",
		`{"session_id":"s1","events":[{"time":"2026-11-14T09:00:00Z","kind":"text","text":"`+text+`"}]}`); status != http.StatusNoContent {
		t.Fatalf("POST /api/session-events: %d %s", status, b)
	}
}

func TestAFactorysUsageAndSessionEventsAreFiledUnderItsClaim(t *testing.T) {
	w := claimedWorld(t)

	if status, b := send(t, w, "integration", "POST", "/api/usage",
		`{"session_id":"s1","responses":[{"kind":"main","model":"claude-opus-5-5","input_tokens":1200,"output_tokens":34}]}`); status != http.StatusNoContent {
		t.Fatalf("POST /api/usage: %d %s", status, b)
	}
	postSessionEvent(t, w, "Reading the Spec")

	if b := ok(t, w, "", "GET", "/api/usage/session?project=acme&session=s1", ""); !strings.Contains(b, `"input_tokens":1200`) {
		t.Errorf("session Usage: %s, want the posted response", b)
	}
	if b := ok(t, w, "", "GET", "/api/projects/acme/tickets/2/claims/1/transcript", ""); !strings.Contains(b, "Reading the Spec") {
		t.Errorf("Transcript: %s, want the posted Session Event", b)
	}
}

func TestAdvanceMovesCoresClock(t *testing.T) {
	w := newWorld(t)
	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)

	w.Advance(90 * time.Minute)

	if got, want := w.Now(), start.Add(90*time.Minute); !got.Equal(want) {
		t.Errorf("Now: %v, want %v", got, want)
	}
	if b := ok(t, w, "agent", "POST", "/api/p/acme/create_spec", `{"title":"S","full":true}`); !strings.Contains(b, `"created":"2026-11-14T10:30:00Z"`) {
		t.Errorf("create_spec: %s, want it created at the advanced time", b)
	}
}

// checkCases turn on each setting whose check the world stands in for.
var checkCases = []struct {
	name  string
	set   func(*world.World, core.WatcherCheckResult)
	path  string
	body  string
	field string
}{
	{"CI", (*world.World).SetCICheck, "/api/p/acme/set_ci_tracking", `{"tracking":true,"repository":"acme/app"}`, "repository"},
	{"Issues", (*world.World).SetIssuesCheck, "/api/p/acme/set_github_issues", `{"on":true,"repositories":["acme/app"]}`, "repositories"},
	{"Azure Boards", (*world.World).SetAzureBoardsCheck, "/api/p/acme/set_azure_boards",
		`{"on":true,"query_url":"https://dev.azure.com/acme/app/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10"}`, "on"},
	{"Pull Request", (*world.World).SetPullRequestCheck, "/api/p/acme/set_landing_method", `{"method":"pull_request"}`, "method"},
}

// checkedWorld is Project acme with a GitHub Push remote, ready for
// each check to run.
func checkedWorld(t *testing.T) *world.World {
	t.Helper()
	w := newWorld(t)
	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)
	ok(t, w, "agent", "POST", "/api/p/acme/report_push_remote", `{"url":"https://github.com/acme/app.git"}`)
	return w
}

func TestEveryCheckPassesUntilSet(t *testing.T) {
	for _, c := range checkCases {
		t.Run(c.name, func(t *testing.T) {
			ok(t, checkedWorld(t), "", "POST", c.path, c.body)
		})
	}
}

func TestACheckRefusalReachesCoresCheckResult(t *testing.T) {
	for _, c := range checkCases {
		t.Run(c.name, func(t *testing.T) {
			w := checkedWorld(t)
			c.set(w, core.WatcherCheckResult{Problem: core.ProblemRepositoryUnreadable, Reason: "gh can't read acme/app", Fix: "Grant gh access"})

			status, b := send(t, w, "", "POST", c.path, c.body)
			var e core.Error
			json.Unmarshal([]byte(b), &e)
			if status != http.StatusUnprocessableEntity || e.Field != c.field || e.Message != "gh can't read acme/app" || e.Fix != "Grant gh access" {
				t.Fatalf("%d %s, want it refused on %s with the check's reason and fix", status, b, c.field)
			}

			c.set(w, core.WatcherCheckResult{})
			ok(t, w, "", "POST", c.path, c.body)
		})
	}
}

func TestAWatcherCrashIsReportedToCore(t *testing.T) {
	w := checkedWorld(t)
	ok(t, w, "", "POST", "/api/p/acme/set_ci_tracking", `{"tracking":true,"repository":"acme/app"}`)

	w.ReportWatcherCrash(core.WatcherCI, "acme", core.WatcherCrash{Restarts: 1, LastLine: "panic: no gh", RestartAt: start.Add(time.Second)})

	if b := ok(t, w, "", "GET", "/api/projects/acme", ""); !strings.Contains(b, `"problem":"crashed"`) || !strings.Contains(b, "panic: no gh") {
		t.Errorf("get_project: %s, want the CI Watcher crashed with its last line", b)
	}
}

// openStream opens the SSE stream at path and returns its body, read past
// the opening comment, so the stream is live when it returns.
func openStream(t *testing.T, w *world.World, path string) (*bufio.Reader, io.Closer) {
	t.Helper()
	req, _ := http.NewRequest("GET", "http://localhost"+path, nil)
	res, err := w.RoundTrip(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("GET %s: status %d, content type %q", path, res.StatusCode, res.Header.Get("Content-Type"))
	}
	r := bufio.NewReader(res.Body)
	if line := readLine(t, r); !strings.HasPrefix(line, ":") {
		t.Fatalf("GET %s: opening line %q", path, line)
	}
	readLine(t, r) // the blank line ending the comment
	return r, res.Body
}

// readLine reads one line of a stream, failing the test if none comes
// within a second.
func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := readWithin(r)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	return line
}

// ended fails the test unless the stream ends within a second.
func ended(t *testing.T, r *bufio.Reader) {
	t.Helper()
	for {
		_, err := readWithin(r)
		if errors.Is(err, errStillOpen) {
			t.Fatal("the stream is still open")
		}
		if err != nil {
			return
		}
	}
}

// errStillOpen is readWithin's error for a stream with nothing to read.
var errStillOpen = errors.New("nothing came within a second")

// readWithin reads one line of a stream, or answers errStillOpen if none
// comes within a second.
func readWithin(r *bufio.Reader) (string, error) {
	type read struct {
		line string
		err  error
	}
	got := make(chan read, 1)
	go func() {
		line, err := r.ReadString('\n')
		got <- read{strings.TrimRight(line, "\n"), err}
	}()
	select {
	case g := <-got:
		return g.line, g.err
	case <-time.After(time.Second):
		return "", errStillOpen
	}
}

// transcriptStreamPath follows claimedWorld's Claim's Transcript.
const transcriptStreamPath = "/api/projects/acme/tickets/2/claims/1/transcript/stream"

func TestTheChangeNoticeFeedStreamsAsCoreWrites(t *testing.T) {
	w := newWorld(t)
	notices, _ := openStream(t, w, "/api/events")

	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)

	if got := readLine(t, notices) + "\n" + readLine(t, notices); got != "event: project.changed\ndata: {\"project\":\"acme\"}" {
		t.Errorf("notice: %q, want project.changed for acme", got)
	}
}

func TestATranscriptStreamStreamsAndClosingItEndsIt(t *testing.T) {
	w := claimedWorld(t)
	transcript, body := openStream(t, w, transcriptStreamPath)
	if n := w.TranscriptStreams(); n != 1 {
		t.Fatalf("open Transcript streams: %d, want 1", n)
	}

	postSessionEvent(t, w, "Reading the Spec")
	readLine(t, transcript) // id
	readLine(t, transcript) // event
	if data := readLine(t, transcript); !strings.Contains(data, "Reading the Spec") {
		t.Errorf("Transcript stream: %q, want the posted Session Event", data)
	}

	body.Close()
	codecentraltest.Eventually(t, func() bool { return w.TranscriptStreams() == 0 }, "the closed stream's handler hasn't ended")
}

func TestCancellingARequestsContextEndsItsStream(t *testing.T) {
	w := newWorld(t)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://localhost/api/events", nil)
	res, err := w.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	notices := bufio.NewReader(res.Body)
	readLine(t, notices)

	cancel()

	ended(t, notices)
}

func TestDroppingStreamsEndsEveryOpenStreamAndTheWorldServesOn(t *testing.T) {
	w := claimedWorld(t)
	notices, _ := openStream(t, w, "/api/events")
	transcript, _ := openStream(t, w, transcriptStreamPath)

	w.DropStreams()

	ended(t, notices)
	ended(t, transcript)
	codecentraltest.Eventually(t, func() bool { return w.TranscriptStreams() == 0 }, "the dropped Transcript stream's handler hasn't ended")
	openStream(t, w, transcriptStreamPath)
	if n := w.TranscriptStreams(); n != 1 {
		t.Errorf("open Transcript streams after reconnecting: %d, want 1", n)
	}
}

func TestDroppingTheWorldEndsItsStreamsAndItServesNoMore(t *testing.T) {
	w := claimedWorld(t)
	notices, _ := openStream(t, w, "/api/events")
	transcript, _ := openStream(t, w, transcriptStreamPath)

	w.Drop()

	ended(t, notices)
	ended(t, transcript)
	req, _ := http.NewRequest("GET", "http://localhost/api/projects/acme", nil)
	if _, err := w.RoundTrip(req); !errors.Is(err, world.ErrDropped) {
		t.Errorf("a request after Drop: %v, want ErrDropped", err)
	}
}

// chatStreamPath follows Project acme's Conversation.
const chatStreamPath = "/api/p/acme/chat/stream"

func TestAFreshWorldIsSettled(t *testing.T) {
	w := newWorld(t)

	if busy := w.Settled(world.Subscribed{}); busy != (world.Busy{}) {
		t.Errorf("a fresh world: %+v, want it settled", busy)
	}
}

func TestAPlainRequestIsInFlightUntilItsBodyIsRead(t *testing.T) {
	w := newWorld(t)
	req, _ := http.NewRequest("GET", "http://localhost/api/workflow", nil)
	res, err := w.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if busy := w.Settled(world.Subscribed{}); busy != (world.Busy{Requests: 1}) {
		t.Errorf("before its body is read: %+v, want one request in flight", busy)
	}
	if _, err := io.ReadAll(res.Body); err != nil {
		t.Fatal(err)
	}
	if busy := w.Settled(world.Subscribed{}); busy != (world.Busy{}) {
		t.Errorf("once its body is read: %+v, want it settled", busy)
	}
}

func TestEachStreamKindCountsItsOpenStreamsAgainstItsSubscribedFeeds(t *testing.T) {
	w := claimedWorld(t)
	openStream(t, w, "/api/events")
	openStream(t, w, transcriptStreamPath)
	openStream(t, w, chatStreamPath)

	busy := w.Settled(world.Subscribed{Events: 1, Transcripts: 1, Chats: 1})
	if busy.Events != nil || busy.Transcripts != nil || busy.Chats != nil {
		t.Errorf("one feed of each kind subscribed: %+v, want every kind's streams open", busy)
	}
	busy = w.Settled(world.Subscribed{Events: 2, Transcripts: 0, Chats: 1})
	if want := (world.Streams{Open: 1, Subscribed: 2}); busy.Events == nil || *busy.Events != want {
		t.Errorf("events: %+v, want %+v", busy.Events, want)
	}
	if want := (world.Streams{Open: 1, Subscribed: 0}); busy.Transcripts == nil || *busy.Transcripts != want {
		t.Errorf("Transcripts: %+v, want %+v", busy.Transcripts, want)
	}
	if busy.Chats != nil {
		t.Errorf("Chats: %+v, want none busy", busy.Chats)
	}
}

func TestADroppedStreamsFeedIsBusyUntilItReconnects(t *testing.T) {
	w := newWorld(t)
	notices, _ := openStream(t, w, "/api/events")

	w.DropStreams()
	ended(t, notices)

	codecentraltest.Eventually(t, func() bool {
		busy := w.Settled(world.Subscribed{Events: 1})
		return busy.Events != nil && *busy.Events == world.Streams{Open: 0, Subscribed: 1}
	}, "the dropped feed isn't reported busy with no stream open")
	openStream(t, w, "/api/events")
	if busy := w.Settled(world.Subscribed{Events: 1}); busy != (world.Busy{}) {
		t.Errorf("once it reconnects: %+v, want it settled", busy)
	}
}

func TestAStreamsUnreadWriteKeepsTheWorldBusyUntilItIsRead(t *testing.T) {
	w := newWorld(t)
	notices, _ := openStream(t, w, "/api/events")

	ok(t, w, "", "POST", "/api/projects/create_project", `{"key":"acme"}`)

	codecentraltest.Eventually(t, func() bool {
		return w.Settled(world.Subscribed{Events: 1}) == (world.Busy{UnreadWrites: 1})
	}, "the notice's write isn't reported unread")
	readLine(t, notices) // event
	readLine(t, notices) // data
	readLine(t, notices) // the blank line ending it
	if busy := w.Settled(world.Subscribed{Events: 1}); busy != (world.Busy{}) {
		t.Errorf("once the notice is read: %+v, want it settled", busy)
	}
}
