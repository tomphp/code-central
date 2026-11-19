package server_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/usage"
)

// One session's Context Size (Spec code-central-201):
// GET /api/usage/session reads one session's responses back in order,
// for its line chart.

// readSession reads GET /api/usage/session with query.
func readSession(t *testing.T, srv *codecentraltest.Server, query string) api.SessionUsageView {
	t.Helper()
	status, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/usage/session" + query})
	if status != http.StatusOK {
		t.Fatalf("GET /api/usage/session%s: status %d: %s", query, status, b)
	}
	var v api.SessionUsageView
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not the session shape: %v: %s", err, b)
	}
	return v
}

// sizes is each response's Context Size, in order.
func sizes(v api.SessionUsageView) []int {
	out := []int{}
	for _, r := range v.Responses {
		out = append(out, r.ContextSize)
	}
	return out
}

func TestASessionsResponsesReadBackInOrderWithTheirContextSize(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	// Posted out of order, and a day apart, as later turns of one session.
	postResponses(t, srv, "s1",
		response{time: at(3), kind: "implementation", input: 5, write: 1_000, read: 60_000},
		response{time: at(-24 * 60), kind: "implementation", input: 5, write: 20_000, read: 0},
		response{time: at(2), agent: "toolu_a", kind: "review", input: 3, write: 9_000, read: 0},
	)
	postResponses(t, srv, "s1",
		response{time: at(4), agent: "toolu_a", kind: "review", input: 3, write: 500, read: 9_000},
		response{time: at(5), kind: "implementation", input: 7, write: 300, read: 61_000},
	)

	main := readSession(t, srv, "?project=acme&session=s1")
	want := []api.SessionUsageResponse{
		{Time: at(-24 * 60), ContextSize: 20_005, Tokens: usage.Tokens{InputTokens: 5, CacheWriteTokens: 20_000, OutputTokens: 2_000}},
		{Time: at(3), ContextSize: 61_005, Tokens: usage.Tokens{InputTokens: 5, CacheWriteTokens: 1_000, CacheReadTokens: 60_000, OutputTokens: 6_100}},
		{Time: at(5), ContextSize: 61_307, Tokens: usage.Tokens{InputTokens: 7, CacheWriteTokens: 300, CacheReadTokens: 61_000, OutputTokens: 6_130}},
	}
	if len(main.Responses) != len(want) {
		t.Fatalf("main session %+v, want %+v", main.Responses, want)
	}
	for i := range want {
		if got := main.Responses[i]; !got.Time.Equal(want[i].Time) || got.ContextSize != want[i].ContextSize || got.Tokens != want[i].Tokens {
			t.Errorf("response %d: %+v, want %+v", i, got, want[i])
		}
	}

	if got := sizes(readSession(t, srv, "?project=acme&session=s1&agent=toolu_a")); !reflect.DeepEqual(got, []int{9_003, 9_503}) {
		t.Errorf("subagent toolu_a: %v, want [9003 9503]", got)
	}
}

func TestASubagentsSessionReadNamesItByTheLatestNonEmptyAgentTypeAndDescription(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	postResponses(t, srv, "s1",
		response{time: at(1), kind: "implementation", input: 5},
		response{time: at(2), agent: "toolu_a", kind: "subagent", agentType: "Explore", description: "Old name", input: 5},
		response{time: at(3), agent: "toolu_a", kind: "subagent", agentType: "Plan", description: "Map Usage", input: 5},
		response{time: at(4), agent: "toolu_a", kind: "subagent", input: 5},
		response{time: at(2), agent: "toolu_old", kind: "subagent", input: 5},
	)

	a := readSession(t, srv, "?project=acme&session=s1&agent=toolu_a")
	if a.AgentType != "Plan" || a.Description != "Map Usage" {
		t.Errorf("toolu_a is %q %q; want the latest non-empty, Plan and Map Usage", a.AgentType, a.Description)
	}
	for _, q := range []string{"&agent=toolu_old", ""} {
		if v := readSession(t, srv, "?project=acme&session=s1"+q); v.AgentType != "" || v.Description != "" {
			t.Errorf("session%s is %q %q; want neither", q, v.AgentType, v.Description)
		}
	}
	_, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/usage/session?project=acme&session=s1"})
	if strings.Contains(string(b), "agent_type") || strings.Contains(string(b), "description") {
		t.Errorf("main session read has the fields: %s", b)
	}
}

func TestASessionReadsOnlyItsOwnProjectAndAllTheKeptDays(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	postResponses(t, srv, "s1",
		response{time: at(-60 * 24 * 60), kind: "implementation", input: 10},
		response{time: at(1), kind: "implementation", input: 20},
	)
	if got := sizes(readSession(t, srv, "?project=acme&session=s1")); !reflect.DeepEqual(got, []int{10, 20}) {
		t.Errorf("acme: %v, want both responses, 60 days apart", got)
	}
	if got := readSession(t, srv, "?project=other&session=s1"); got.Responses == nil || len(got.Responses) != 0 {
		t.Errorf("other: %+v, want no responses", got)
	}
	if got := readSession(t, srv, "?project=acme&session=nobody"); got.Responses == nil || len(got.Responses) != 0 {
		t.Errorf("an unknown session: %+v, want no responses", got)
	}
}

func TestASessionReadRejectsAnUnknownProjectAndBadParameters(t *testing.T) {
	srv := newServer(t)
	httpRejected(t, srv, httpCall{method: "GET", path: "/api/usage/session?project=nope&session=s1"}, api.NotFound)
	for query, field := range map[string]string{
		"?session=s1":                       "project",
		"?project=acme":                     "session",
		"?project=acme&session=s1&colour=1": "colour",
	} {
		if e := httpRejected(t, srv, httpCall{method: "GET", path: "/api/usage/session" + query}, api.Invalid); e.Field != field {
			t.Errorf("%s: field %q, want %s", query, e.Field, field)
		}
	}
}

func TestASessionSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	first := newServerWith(t, []codecentraltest.Option{codecentraltest.WithUsageDir(dir)})
	claim(t, first, "acme", "s1")
	postResponses(t, first, "s1", response{time: at(1), kind: "implementation", input: 42})

	second := newServerWith(t, []codecentraltest.Option{codecentraltest.WithUsageDir(dir)})
	if got := sizes(readSession(t, second, "?project=acme&session=s1")); !reflect.DeepEqual(got, []int{42}) {
		t.Fatalf("%v after a restart, want [42]", got)
	}
}
