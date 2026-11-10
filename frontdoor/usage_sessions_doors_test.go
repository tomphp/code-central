package frontdoor_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// Context Size (Spec code-central-201): a Factory posts each model
// response's token usage to POST /api/usage, filed under the newest Claim
// with its session ID, and GET /api/usage reads it back as each kind of
// session's peaks and the recent sessions.

// postUsage posts body to POST /api/usage, which always answers 204.
func postUsage(t *testing.T, srv *codecentraltest.Server, body string) {
	t.Helper()
	status, b := httpDo(t, srv, httpCall{method: "POST", path: "/api/usage", body: body})
	if status != http.StatusNoContent || len(b) != 0 {
		t.Fatalf("POST /api/usage %s: %d %q, want 204 with no body", body, status, b)
	}
}

// response is one posted model response: its Context Size is
// input + write + read, and its output, when left at 0, is a tenth of that.
// A response with no model is posted without one.
type response struct {
	time                       time.Time
	agent, kind, model         string
	input, write, read, output int
}

func postResponses(t *testing.T, srv *codecentraltest.Server, session string, rs ...response) {
	t.Helper()
	var items []string
	for _, r := range rs {
		if r.output == 0 {
			r.output = (r.input + r.write + r.read) / 10
		}
		agent := ""
		if r.agent != "" {
			agent = fmt.Sprintf(`"agent":%q,`, r.agent)
		}
		model := ""
		if r.model != "" {
			model = fmt.Sprintf(`"model":%q,`, r.model)
		}
		items = append(items, fmt.Sprintf(
			`{"time":%q,%s"kind":%q,%s"input_tokens":%d,"cache_write_tokens":%d,"cache_read_tokens":%d,"output_tokens":%d}`,
			r.time.Format(time.RFC3339Nano), agent, r.kind, model, r.input, r.write, r.read, r.output))
	}
	postUsage(t, srv, fmt.Sprintf(`{"session_id":%q,"responses":[%s]}`, session, strings.Join(items, ",")))
}

// claim claims key's next Ready Ticket for session.
func claim(t *testing.T, srv *codecentraltest.Server, key, session string) core.ClaimResult {
	t.Helper()
	var res core.ClaimResult
	must(t, func() error {
		var err error
		res, err = srv.Core.Claim(context.Background(), key, core.ClaimInput{SessionID: session})
		return err
	})
	return res
}

// resume finishes first, acme-3, makes acme-2 Ready and claims it a
// minute later in session, as a session resumed under a newer Claim.
func resume(t *testing.T, srv *codecentraltest.Server, session string, first core.ClaimResult) {
	t.Helper()
	ctx := context.Background()
	must(t, func() error {
		_, err := srv.Core.Finish(ctx, "acme", core.FinishInput{Ticket: "3", ClaimID: first.ClaimID, Summary: "done"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.Approve(core.WithActor(ctx, core.Human), "acme", core.TicketInput{Ticket: "2"})
		return err
	})
	srv.Clock.Advance(time.Minute)
	claim(t, srv, "acme", session)
}

// at is a time on the test server's first day, minutes after it starts.
func at(minutes int) time.Time {
	return codecentraltest.Start.Add(time.Duration(minutes) * time.Minute)
}

func TestPostingUsageAlwaysAnswers204AndDropsWhatItCannotFile(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	for _, body := range []string{
		`{"session_id":"unknown","responses":[{"time":"2026-01-01T00:00:00Z","kind":"implementation","input_tokens":5}]}`,
		`{"session_id":"s1","responses":[]}`,
		`{"session_id":"s1"}`,
		`{}`,
		`{`,
		`not json`,
		`{"session_id":"s1","responses":[{"kind":7}]}`,
	} {
		postUsage(t, srv, body)
	}
	if u := readUsage(t, srv, ""); len(u.Kinds)+len(u.Sessions) != 0 {
		t.Fatalf("kinds %+v and sessions %+v, want none", u.Kinds, u.Sessions)
	}
}

func TestPostedUsageGoesToTheNewestClaimWithThatSession(t *testing.T) {
	srv := newServer(t)
	first := claim(t, srv, "acme", "s1") // acme-3
	postResponses(t, srv, "s1", response{time: at(1), kind: "implementation", input: 10, write: 1000, read: 0})
	resume(t, srv, "s1", first) // acme-2
	postResponses(t, srv, "s1", response{time: at(2), kind: "implementation", input: 10, write: 500, read: 3000})

	got := readUsage(t, srv, "").Sessions
	want := []core.UsageSession{{
		Project: "acme", Ticket: "acme-2", Claim: 1, SessionID: "s1", Kind: "implementation",
		Started: at(1), Peak: 3510, Responses: 2, Models: []string{},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessions %+v, want %+v", got, want)
	}
}

func TestPostingUsageCountsAsAPing(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	srv.Clock.Advance(10 * time.Minute)
	posted := srv.Clock.Now()
	postUsage(t, srv, `{"session_id":"s1","responses":[]}`)

	var v core.TicketView
	_, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/p/acme/tickets/3"})
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if got := v.Claims[0].LastActivity; !got.Equal(posted) {
		t.Fatalf("last_activity %v, want the post at %v", got, posted)
	}
}

func TestContextSizeByKindWithSubagentsAsSessionsOfTheirOwn(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "impl")
	ctx := core.WithActor(context.Background(), core.Human)
	must(t, func() error { _, err := srv.Core.Approve(ctx, "acme", core.TicketInput{Ticket: "2"}); return err })
	claim(t, srv, "acme", "spec")

	postResponses(t, srv, "impl",
		response{time: at(1), kind: "implementation", input: 5, write: 20_000, read: 0},
		response{time: at(2), kind: "implementation", input: 5, write: 1_000, read: 60_000}, // peak 61,005
		response{time: at(3), agent: "toolu_a", kind: "review", input: 3, write: 9_000, read: 0},
		response{time: at(4), agent: "toolu_b", kind: "review", input: 3, write: 4_000, read: 2_000},
		response{time: at(5), agent: "toolu_c", kind: "subagent", input: 1, write: 7_000, read: 0},
		response{time: at(6), agent: "toolu_d", kind: "custom", input: 1, write: 100, read: 0},
		response{time: at(7), agent: "toolu_e", kind: "another", input: 1, write: 200, read: 0},
	)
	postResponses(t, srv, "spec", response{time: at(8), kind: "spec", input: 2, write: 30_000, read: 0})

	u := readUsage(t, srv, "")
	want := []core.UsageKind{
		{Kind: "spec", Sessions: 1, AveragePeak: 30_002, LargestPeak: 30_002},
		{Kind: "implementation", Sessions: 1, AveragePeak: 61_005, LargestPeak: 61_005},
		{Kind: "review", Sessions: 2, AveragePeak: 7_503, LargestPeak: 9_003},
		{Kind: "subagent", Sessions: 1, AveragePeak: 7_001, LargestPeak: 7_001},
		{Kind: "another", Sessions: 1, AveragePeak: 201, LargestPeak: 201},
		{Kind: "custom", Sessions: 1, AveragePeak: 101, LargestPeak: 101},
	}
	if !reflect.DeepEqual(u.Kinds, want) {
		t.Fatalf("kinds %+v, want %+v", u.Kinds, want)
	}

	type row struct {
		ticket, agent, kind string
		peak, responses     int
	}
	var got []row
	for _, s := range u.Sessions {
		if s.SessionID != map[string]string{"acme-3": "impl", "acme-2": "spec"}[s.Ticket] || s.Project != "acme" || s.Claim != 1 {
			t.Errorf("session %+v filed under the wrong Claim", s)
		}
		got = append(got, row{s.Ticket, s.Agent, s.Kind, s.Peak, s.Responses})
	}
	wantRows := []row{
		{"acme-2", "", "spec", 30_002, 1},
		{"acme-3", "toolu_e", "another", 201, 1},
		{"acme-3", "toolu_d", "custom", 101, 1},
		{"acme-3", "toolu_c", "subagent", 7_001, 1},
		{"acme-3", "toolu_b", "review", 6_003, 1},
		{"acme-3", "toolu_a", "review", 9_003, 1},
		{"acme-3", "", "implementation", 61_005, 2},
	}
	if !reflect.DeepEqual(got, wantRows) {
		t.Fatalf("sessions, newest first:\n%+v\nwant\n%+v", got, wantRows)
	}
}

func TestRecentSessionsAreTheNewest50(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	var rs []response
	for i := range 60 {
		rs = append(rs, response{time: at(i), agent: fmt.Sprintf("agent-%02d", i), kind: "subagent", input: i + 1})
	}
	postResponses(t, srv, "s1", rs...)

	u := readUsage(t, srv, "")
	if len(u.Sessions) != 50 || u.Sessions[0].Agent != "agent-59" || u.Sessions[49].Agent != "agent-10" {
		t.Fatalf("%d sessions from %+v, want agent-59 back to agent-10", len(u.Sessions), u.Sessions[0])
	}
	// The kinds still count every session, not only the 50 listed.
	if want := []core.UsageKind{{Kind: "subagent", Sessions: 60, AveragePeak: 31, LargestPeak: 60}}; !reflect.DeepEqual(u.Kinds, want) {
		t.Fatalf("kinds %+v, want %+v", u.Kinds, want)
	}
}

func TestContextSizeForOneProjectOrAllOfThemOverTheRange(t *testing.T) {
	srv := newServer(t)
	ctx := context.Background()
	must(t, func() error {
		_, err := srv.Core.CreateSpec(ctx, "other", core.CreateSpecInput{Title: "S"})
		return err
	})
	must(t, func() error {
		_, err := srv.Core.CreateTicket(ctx, "other", core.CreateTicketInput{Spec: "1", Title: "T", State: "Ready"})
		return err
	})
	claim(t, srv, "acme", "a")
	claim(t, srv, "other", "o")
	postResponses(t, srv, "a", response{time: at(1), kind: "implementation", input: 100})
	postResponses(t, srv, "o", response{time: at(2), kind: "implementation", input: 300})
	// Twenty days before: only in the 30 and 90 day ranges.
	postResponses(t, srv, "o", response{time: at(-20 * 24 * 60), agent: "old", kind: "review", input: 50})

	sessions := func(query string) []string {
		var out []string
		for _, s := range readUsage(t, srv, query).Sessions {
			out = append(out, s.Ticket+"/"+s.Kind)
		}
		return out
	}
	if got := sessions("?project=acme"); !reflect.DeepEqual(got, []string{"acme-3/implementation"}) {
		t.Errorf("acme: %v", got)
	}
	if got := sessions(""); !reflect.DeepEqual(got, []string{"other-2/implementation", "acme-3/implementation"}) {
		t.Errorf("all: %v", got)
	}
	if got := sessions("?project=other&days=30"); !reflect.DeepEqual(got, []string{"other-2/implementation", "other-2/review"}) {
		t.Errorf("other over 30 days: %v", got)
	}
	want := []core.UsageKind{{Kind: "implementation", Sessions: 2, AveragePeak: 200, LargestPeak: 300}}
	if got := readUsage(t, srv, "").Kinds; !reflect.DeepEqual(got, want) {
		t.Errorf("all kinds %+v, want %+v", got, want)
	}
}

func TestPostedUsageSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first := newServerWith(t, []codecentraltest.Option{codecentraltest.WithUsageDir(dir)})
	claim(t, first, "acme", "s1")
	postResponses(t, first, "s1",
		response{time: at(1), kind: "implementation", model: "claude-opus-4-7", input: 42},
		// As every line written before responses named their model.
		response{time: at(2), agent: "toolu_a", kind: "review", input: 7},
	)

	second := newServerWith(t, []codecentraltest.Option{codecentraltest.WithUsageDir(dir)})
	got := readUsage(t, second, "?project=acme").Sessions
	if len(got) != 2 || got[1].Peak != 42 || !reflect.DeepEqual(got[1].Models, []string{"claude-opus-4-7"}) || !reflect.DeepEqual(got[0].Models, []string{}) {
		t.Fatalf("sessions %+v after a restart, want the one with peak 42 on claude-opus-4-7 and a subagent with no model", got)
	}
}

// Model column (Spec code-central-266): each posted response may name its
// model, and each recent session lists its models in order of first use.

func TestASessionListsItsModelsInOrderOfFirstUse(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	// Posted out of order: the Sonnet response came first.
	postResponses(t, srv, "s1",
		response{time: at(3), kind: "implementation", model: "claude-opus-4-7", input: 10},
		response{time: at(4), kind: "implementation", model: "claude-sonnet-4-6", input: 10},
		response{time: at(5), kind: "implementation", input: 10},
	)
	postResponses(t, srv, "s1",
		response{time: at(1), kind: "implementation", model: "claude-sonnet-4-6", input: 10},
		response{time: at(2), kind: "implementation", model: "claude-opus-4-7", input: 10},
		response{time: at(3), agent: "toolu_a", kind: "review", model: "claude-haiku-4-5", input: 10},
		response{time: at(4), agent: "toolu_b", kind: "subagent", input: 10},
	)

	got := map[string][]string{}
	for _, s := range readUsage(t, srv, "").Sessions {
		got[s.Agent] = s.Models
	}
	want := map[string][]string{
		"":        {"claude-sonnet-4-6", "claude-opus-4-7"},
		"toolu_a": {"claude-haiku-4-5"},
		"toolu_b": {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models by agent %v, want %v", got, want)
	}
}

func TestAResumedSessionListsTheModelsOfEveryClaim(t *testing.T) {
	srv := newServer(t)
	resume(t, srv, "s1", claim(t, srv, "acme", "s1")) // acme-3, then acme-2
	postResponses(t, srv, "s1", response{time: at(2), kind: "implementation", model: "claude-sonnet-4-6", input: 10})
	// Filed under the newest Claim too, but used before it.
	postResponses(t, srv, "s1", response{time: at(1), kind: "implementation", model: "claude-opus-4-7", input: 10})

	got := readUsage(t, srv, "").Sessions
	if len(got) != 1 || !reflect.DeepEqual(got[0].Models, []string{"claude-opus-4-7", "claude-sonnet-4-6"}) {
		t.Fatalf("sessions %+v, want one on claude-opus-4-7 then claude-sonnet-4-6", got)
	}
}

func TestASessionWithNoModelsListsNone(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	postResponses(t, srv, "s1", response{time: at(1), kind: "implementation", input: 10})

	_, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/usage"})
	if !strings.Contains(string(b), `"models":[]`) {
		t.Fatalf("GET /api/usage %s, want \"models\":[]", b)
	}
}
