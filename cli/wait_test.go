package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

var human = core.WithActor(context.Background(), core.Human)

// asking starts a server with Ticket acme-2 claimed as claim-1 and asking
// one Question.
func asking(t *testing.T) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t)
	c, ctx := srv.Core, context.Background()
	for _, err := range []error{
		second(c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})),
		second(c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"})),
		second(c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: store.Ready})),
		second(c.Claim(ctx, "acme", core.ClaimInput{})),
		second(c.PostRound(ctx, "acme", core.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []core.QuestionInput{{Title: "DB?", Body: "Which?"}}})),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	return srv
}

func second[T any](_ T, err error) error { return err }

func answer(t *testing.T, srv *codecentraltest.Server) {
	t.Helper()
	if _, err := srv.Core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
}

// startWait runs `code-central wait <args>` in the background against
// url, with clk as the CLI's clock for retries.
func startWait(url string, clk clock.Clock, args ...string) <-chan result {
	out := make(chan result, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		env := map[string]string{"CODE_CENTRAL_URL": url, "CODE_CENTRAL_PROJECT": "acme"}
		code := cli.Run(context.Background(), append([]string{"wait"}, args...), cli.Env{
			Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
			Getenv: func(k string) string { return env[k] }, Clock: clk,
		})
		out <- result{code, stdout.String(), stderr.String()}
	}()
	return out
}

type waitOutput struct {
	Ticket   string         `json:"ticket"`
	State    store.State    `json:"state"`
	NextStep string         `json:"next_step"`
	LastMove *core.MoveView `json:"last_move"`
}

func TestWaitExitsZeroWhenTheTicketIsAnswered(t *testing.T) {
	srv := asking(t)
	done := startWait(srv.URL, clock.NewFake(codecentraltest.Start), "acme-2")
	srv.Clock.BlockUntil(1) // the long-poll is holding
	answer(t, srv)
	r := <-done
	out := decode[waitOutput](t, r)
	if out.Ticket != "acme-2" || out.State != store.Answered || !strings.Contains(out.NextStep, "pick_up_answers") {
		t.Fatalf("output %+v", out)
	}

	// Already Answered: at once.
	if r := <-startWait(srv.URL, clock.NewFake(codecentraltest.Start), "2"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
}

func TestWaitKeepsHoldingAcrossLongPollTimeouts(t *testing.T) {
	srv := asking(t)
	done := startWait(srv.URL, clock.NewFake(codecentraltest.Start), "acme-2")
	for range 3 {
		srv.Clock.BlockUntil(1)
		srv.Clock.Advance(core.WaitTimeout)
	}
	srv.Clock.BlockUntil(1)
	select {
	case r := <-done:
		t.Fatalf("wait exited early: %+v", r)
	default:
	}
	answer(t, srv)
	if r := <-done; r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
}

func TestWaitExitsThreeWhenTheTicketLeftForAnotherState(t *testing.T) {
	srv := asking(t)
	answer(t, srv)
	ctx := context.Background()
	if _, err := srv.Core.PickUpAnswers(ctx, "acme", core.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done."}); err != nil {
		t.Fatal(err)
	}
	r := <-startWait(srv.URL, clock.NewFake(codecentraltest.Start), "acme-2")
	if r.code != 3 {
		t.Fatalf("exit %d, want 3: %s %s", r.code, r.stdout, r.stderr)
	}
	var out waitOutput
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.stdout)
	}
	if out.State != store.InReview || out.LastMove == nil || out.LastMove.From != store.InProgress || out.LastMove.To != store.InReview {
		t.Fatalf("output %+v", out)
	}
}

// flaky proxies to the server but drops the first n connections, as a
// service that is restarting would.
func flaky(t *testing.T, target string, n int32) (string, *atomic.Int32) {
	t.Helper()
	u, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(u)
	var seen atomic.Int32
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen.Add(1) <= n {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.(*net.TCPConn).SetLinger(0)
				conn.Close()
			}
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(hs.Close)
	return hs.URL, &seen
}

func TestWaitRetriesThroughConnectionErrors(t *testing.T) {
	srv := asking(t)
	front, seen := flaky(t, srv.URL, 2)
	cliClock := clock.NewFake(codecentraltest.Start)
	done := startWait(front, cliClock, "acme-2")
	for range 2 {
		cliClock.BlockUntil(1) // backing off after a dropped connection
		cliClock.Advance(time.Minute)
	}
	srv.Clock.BlockUntil(1)
	answer(t, srv)
	if r := <-done; r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if n := seen.Load(); n != 3 {
		t.Fatalf("%d requests, want 2 dropped and 1 that held", n)
	}
}

func TestWaitUsageAndErrors(t *testing.T) {
	srv := asking(t)
	env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
	if r := run(t, env, "", "wait"); r.code != 1 {
		t.Fatalf("no ID: exit %d", r.code)
	}
	if r := run(t, env, "", "wait", "2", "3"); r.code != 1 {
		t.Fatalf("two IDs: exit %d", r.code)
	}
	if r := run(t, env, "", "wait", "2", "--claim", "claim-1"); r.code != 1 {
		t.Fatalf("wait takes no Claim ID: exit %d", r.code)
	}
	wantError(t, run(t, env, "", "wait", "9"), core.NotFound)
}
