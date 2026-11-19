package server_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
)

// Nested Subagents (Spec code-central-638): GET /api/usage lists main
// sessions only, each with its Subagents, oldest first, and each
// session's tokens, with a main session's total including its
// Subagents'.

// everySession is each main session followed by its Subagents.
func everySession(mains []api.UsageSession) []api.UsageSession {
	var out []api.UsageSession
	for _, m := range mains {
		out = append(append(out, m), m.Subagents...)
	}
	return out
}

// agents are the agents of sessions, in order.
func agents(sessions []api.UsageSession) []string {
	out := []string{}
	for _, s := range sessions {
		out = append(out, s.Agent)
	}
	return out
}

func TestSubagentsNestUnderTheirMainSessionOldestFirst(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "impl")
	ctx := api.WithActor(context.Background(), api.Human)
	must(t, func() error { _, err := srv.Core.Approve(ctx, "acme", api.TicketInput{Ticket: "2"}); return err })
	claim(t, srv, "acme", "spec")

	postResponses(t, srv, "impl",
		response{time: at(1), kind: "implementation", input: 10},
		response{time: at(3), agent: "toolu_a", kind: "review", input: 10},
		response{time: at(2), agent: "toolu_b", kind: "subagent", input: 10},
		response{time: at(4), agent: "toolu_c", kind: "subagent", input: 10},
		response{time: at(5), agent: "toolu_b", kind: "subagent", input: 10},
	)
	postResponses(t, srv, "spec", response{time: at(6), kind: "spec", input: 10})

	u := readUsage(t, srv, "")
	var top []string
	for _, s := range u.Sessions {
		top = append(top, s.SessionID+"/"+s.Agent)
	}
	if want := []string{"spec/", "impl/"}; !reflect.DeepEqual(top, want) {
		t.Fatalf("top-level sessions %v, want only the main sessions %v, newest first", top, want)
	}
	if got := agents(u.Sessions[0].Subagents); len(got) != 0 {
		t.Errorf("spec's Subagents %v, want none", got)
	}
	impl := u.Sessions[1]
	if got, want := agents(impl.Subagents), []string{"toolu_b", "toolu_a", "toolu_c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("impl's Subagents %v, want %v, oldest first", got, want)
	}
	for _, s := range impl.Subagents {
		if s.SessionID != "impl" || s.Ticket != "acme-3" || s.Subagents == nil || len(s.Subagents) != 0 || s.TotalTokens != nil {
			t.Errorf("Subagent %+v, want one of impl's on acme-3 with no Subagents or total of its own", s)
		}
	}
	if b := impl.Subagents[0]; b.Responses != 2 || b.Kind != "subagent" {
		t.Errorf("toolu_b %+v, want its 2 responses", b)
	}

	// Subagents is always there, and the total only on a main session.
	_, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/usage"})
	body := string(b)
	if n := strings.Count(body, `"subagents":[]`); n != 4 {
		t.Errorf("GET /api/usage has %d empty subagents, want 4 (spec and impl's three): %s", n, body)
	}
	if n := strings.Count(body, `"total_tokens":`); n != 2 {
		t.Errorf("GET /api/usage has %d total_tokens, want 2, one per main session: %s", n, body)
	}
}

func TestEachSessionHasItsTokensAndAMainSessionItsTotalWithItsSubagents(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "impl")
	ctx := api.WithActor(context.Background(), api.Human)
	must(t, func() error { _, err := srv.Core.Approve(ctx, "acme", api.TicketInput{Ticket: "2"}); return err })
	claim(t, srv, "acme", "spec")

	postResponses(t, srv, "impl",
		response{time: at(1), kind: "implementation", input: 1, write: 10, read: 100, output: 1_000},
		response{time: at(2), kind: "implementation", input: 2, write: 20, read: 200, output: 2_000},
		// A bigger context than its parent's, which the parent's peak doesn't take.
		response{time: at(3), agent: "toolu_a", kind: "review", input: 3, write: 30_000, read: 300, output: 3},
		response{time: at(4), agent: "toolu_b", kind: "subagent", input: 4, write: 40, read: 400, output: 4},
	)
	postResponses(t, srv, "spec", response{time: at(5), kind: "spec", input: 5, write: 50, read: 500, output: 5})

	u := readUsage(t, srv, "")
	spec, impl := u.Sessions[0], u.Sessions[1]
	if spec.Tokens != 560 || spec.TotalTokens == nil || *spec.TotalTokens != 560 {
		t.Errorf("spec has tokens %d and total %v, want 560 and the same 560 with no Subagents", spec.Tokens, spec.TotalTokens)
	}
	if impl.Tokens != 3_333 || impl.TotalTokens == nil || *impl.TotalTokens != 3_333+30_306+448 {
		t.Errorf("impl has tokens %d and total %v, want 3333 and %d with its Subagents", impl.Tokens, impl.TotalTokens, 3_333+30_306+448)
	}
	if got := []int{impl.Subagents[0].Tokens, impl.Subagents[1].Tokens}; !reflect.DeepEqual(got, []int{30_306, 448}) {
		t.Errorf("Subagents' tokens %v, want [30306 448]", got)
	}
	if impl.Peak != 222 || impl.Subagents[0].Peak != 30_303 {
		t.Errorf("peaks %d and %d, want impl's own 222 and toolu_a's own 30303", impl.Peak, impl.Subagents[0].Peak)
	}
}

func TestAMainSessionWhoseOnlyResponsesInRangeAreItsSubagentsIsListed(t *testing.T) {
	srv := newServer(t)
	first := claim(t, srv, "acme", "s1") // acme-3
	// The turn started twenty days before: out of the 7 day range.
	postResponses(t, srv, "s1",
		response{time: at(-20 * 24 * 60), kind: "implementation", model: "claude-opus-5-5", input: 100},
		response{time: at(2), agent: "toolu_a", kind: "subagent", input: 10},
	)
	resume(t, srv, "s1", first) // acme-2
	postResponses(t, srv, "s1", response{time: at(3), agent: "toolu_b", kind: "review", input: 20})

	u := readUsage(t, srv, "")
	if len(u.Sessions) != 1 {
		t.Fatalf("sessions %+v, want the one main session", u.Sessions)
	}
	got := u.Sessions[0]
	got.Subagents = nil
	want := api.UsageSession{
		Project: "acme", Ticket: "acme-2", Claim: 1, SessionID: "s1",
		Started: at(2), Models: []string{}, TotalTokens: new(11 + 22),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("main session %+v, want %+v: no responses or kind of its own, and the Ticket of its Subagents' latest", got, want)
	}
	if a := agents(u.Sessions[0].Subagents); !reflect.DeepEqual(a, []string{"toolu_a", "toolu_b"}) {
		t.Errorf("Subagents %v, want toolu_a and toolu_b", a)
	}
	// The kinds count only sessions with responses.
	wantKinds := []api.UsageKind{
		{Kind: "review", Sessions: 1, AveragePeak: 20, LargestPeak: 20},
		{Kind: "subagent", Sessions: 1, AveragePeak: 10, LargestPeak: 10},
	}
	if !reflect.DeepEqual(u.Kinds, wantKinds) {
		t.Errorf("kinds %+v, want %+v", u.Kinds, wantKinds)
	}

	// Over 30 days its own response is in range.
	main := readUsage(t, srv, "?days=30").Sessions[0]
	if main.Responses != 1 || main.Kind != "implementation" || !main.Started.Equal(at(-20*24*60)) || main.Ticket != "acme-3" {
		t.Errorf("over 30 days, main session %+v, want its own response, kind, start and Claim", main)
	}
}

func TestTheRecentSessionsLimitCountsMainSessionsOnly(t *testing.T) {
	srv := newServer(t)
	ctx := context.Background()
	// 55 Claims of acme-3, each giving up for the next.
	for i := range 55 {
		session := fmt.Sprintf("s%02d", i)
		c := claim(t, srv, "acme", session)
		postResponses(t, srv, session,
			response{time: at(i), kind: "implementation", input: 1},
			response{time: at(i), agent: "toolu_a", kind: "subagent", input: 1},
			response{time: at(i), agent: "toolu_b", kind: "subagent", input: 1},
		)
		must(t, func() error {
			_, err := srv.Core.GiveUp(ctx, "acme", api.GiveUpInput{Ticket: "3", ClaimID: c.ClaimID, Note: "next"})
			return err
		})
	}

	u := readUsage(t, srv, "")
	if len(u.Sessions) != 50 || u.Sessions[0].SessionID != "s54" || u.Sessions[49].SessionID != "s05" {
		t.Fatalf("%d sessions from %+v, want s54 back to s05", len(u.Sessions), u.Sessions[0])
	}
	for _, s := range u.Sessions {
		if got := agents(s.Subagents); !reflect.DeepEqual(got, []string{"toolu_a", "toolu_b"}) {
			t.Errorf("%s's Subagents %v, want both", s.SessionID, got)
		}
	}
	want := []api.UsageKind{
		{Kind: "implementation", Sessions: 55, AveragePeak: 1, LargestPeak: 1},
		{Kind: "subagent", Sessions: 110, AveragePeak: 1, LargestPeak: 1},
	}
	if !reflect.DeepEqual(u.Kinds, want) {
		t.Errorf("kinds %+v, want %+v, counting every session", u.Kinds, want)
	}
}

func TestASubagentHasTheLatestAgentTypeAndDescriptionItWasPostedWith(t *testing.T) {
	srv := newServer(t)
	claim(t, srv, "acme", "s1")
	postResponses(t, srv, "s1",
		// A main session's response has neither, even if posted with them.
		response{time: at(1), agentType: "Plan", description: "Not a Subagent", kind: "implementation", input: 10},
		response{time: at(2), agent: "toolu_a", agentType: "Explore", description: "Map it", kind: "subagent", input: 10},
		response{time: at(3), agent: "toolu_a", description: "Map it better", kind: "subagent", input: 10},
		response{time: at(4), agent: "toolu_a", kind: "subagent", input: 10},
		// As every response recorded before Agent Types were posted.
		response{time: at(5), agent: "toolu_old", kind: "subagent", input: 10},
	)

	u := readUsage(t, srv, "")
	main := u.Sessions[0]
	if main.AgentType != "" || main.Description != "" {
		t.Errorf("main session has %q and %q, want neither", main.AgentType, main.Description)
	}
	if a := main.Subagents[0]; a.AgentType != "Explore" || a.Description != "Map it better" {
		t.Errorf("toolu_a has %q and %q, want Explore and Map it better", a.AgentType, a.Description)
	}
	status, b := httpDo(t, srv, httpCall{method: "GET", path: "/api/usage"})
	if status != http.StatusOK {
		t.Fatalf("GET /api/usage: %d", status)
	}
	if n := strings.Count(string(b), `"agent_type":`); n != 1 {
		t.Errorf("GET /api/usage has %d agent_type, want only toolu_a's: %s", n, b)
	}
	if n := strings.Count(string(b), `"description":`); n != 1 {
		t.Errorf("GET /api/usage has %d description, want only toolu_a's: %s", n, b)
	}
}
