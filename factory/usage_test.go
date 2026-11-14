package factory_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// usageSession is a Usage session without its times, for comparing.
type usageSession struct {
	Ticket, Agent, Kind string
	Claim, Peak         int
	Responses           int
	Models              []string
}

// usageSessions are the Usage sessions of session, by agent.
func usageSessions(u core.UsageView, session string) []usageSession {
	var out []usageSession
	for _, s := range u.Sessions {
		if s.SessionID == session {
			out = append(out, usageSession{s.Ticket, s.Agent, s.Kind, s.Claim, s.Peak, s.Responses, s.Models})
		}
	}
	slices.SortFunc(out, func(a, b usageSession) int { return strings.Compare(a.Agent, b.Agent) })
	return out
}

// wantUsageSessions checks that u's sessions of session are want.
func wantUsageSessions(t *testing.T, u core.UsageView, session string, want []usageSession) {
	t.Helper()
	if got := usageSessions(u, session); !reflect.DeepEqual(got, want) {
		t.Errorf("Usage sessions\n got %+v\nwant %+v", got, want)
	}
}

// waitForUsage waits for Usage to hold n responses in all, and returns it.
func waitForUsage(h *factorytest.Harness, n int) core.UsageView {
	var u core.UsageView
	h.WaitFor("Usage to hold the responses", func() bool {
		u = h.Usage()
		got := 0
		for _, s := range u.Sessions {
			got += s.Responses
		}
		return got >= n
	})
	return u
}

func TestEachModelResponsesTokenUsageIsPostedOnceUnderItsKind(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.StageTicket("spec", "Spec: Paint the shed")
	h.Script(id,
		// One response in two content blocks, each line carrying its usage.
		factorytest.Response("msg_1", "", "", 100, 200, 300, 10),
		factorytest.Response("msg_1", "", "", 100, 200, 300, 10),
		factorytest.AgentCall("toolu_review", "general-purpose", "Spec review of the branch"),
		factorytest.AgentCall("toolu_explore", "Explore", "Find the paint"),
		factorytest.Response("msg_r1", "toolu_review", "", 1000, 0, 0, 5),
		factorytest.Response("msg_e1", "toolu_explore", "", 50, 0, 0, 5),
		factorytest.Response("msg_e2", "toolu_explore", "", 60, 0, 0, 5),
		factorytest.Response("msg_2", "", "", 100, 0, 700, 20),
		factorytest.FinishSpec("Done.", "split"),
		factorytest.Result("success"),
	)
	h.Start()
	h.WaitForState(id, store.InReview)
	session := h.Ticket(id).Claims[0].SessionID

	u := waitForUsage(h, 5)
	want := []usageSession{
		{Ticket: id, Kind: "spec", Claim: 1, Peak: 800, Responses: 2, Models: []string{}},
		{Ticket: id, Agent: "toolu_explore", Kind: "subagent", Claim: 1, Peak: 60, Responses: 2, Models: []string{}},
		{Ticket: id, Agent: "toolu_review", Kind: "review", Claim: 1, Peak: 1000, Responses: 1, Models: []string{}},
	}
	wantUsageSessions(t, u, session, want)
	var kinds []string
	for _, k := range u.Kinds {
		kinds = append(kinds, k.Kind)
	}
	if want := []string{"spec", "review", "subagent"}; !slices.Equal(kinds, want) {
		t.Errorf("Usage kinds are %q; want %q", kinds, want)
	}
}

func TestEachSubagentsResponsesCarryItsAgentTypeAndDescriptionAndTheMainSessionsCarryNeither(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Paint with helpers")
	h.Script(id,
		factorytest.Response("msg_1", "", "", 100, 0, 0, 10),
		factorytest.AgentCall("toolu_explore", "Explore", "Find the paint"),
		factorytest.AgentCall("toolu_review", "general-purpose", "Spec review of the branch"),
		factorytest.AgentCall("toolu_typeless", "", "Stir the paint"),
		factorytest.AgentCall("toolu_descless", "Plan", ""),
		factorytest.Response("msg_e1", "toolu_explore", "", 50, 0, 0, 5),
		factorytest.Response("msg_e2", "toolu_explore", "", 60, 0, 0, 5),
		factorytest.Response("msg_r1", "toolu_review", "", 70, 0, 0, 5),
		factorytest.Response("msg_t1", "toolu_typeless", "", 80, 0, 0, 5),
		factorytest.Response("msg_d1", "toolu_descless", "", 90, 0, 0, 5),
		factorytest.Finish("Done."),
		factorytest.Result("success"),
	)
	h.Start()
	h.WaitForState(id, store.InReview)
	waitForUsage(h, 6)
	session := h.Ticket(id).Claims[0].SessionID

	for _, c := range []struct{ agent, kind, agentType, description string }{
		{"", "implementation", "", ""},
		{"toolu_explore", "subagent", "Explore", "Find the paint"},
		{"toolu_review", "review", "general-purpose", "Spec review of the branch"},
		{"toolu_typeless", "subagent", "", "Stir the paint"},
		{"toolu_descless", "subagent", "Plan", ""},
	} {
		v := h.SessionUsage(session, c.agent)
		if v.AgentType != c.agentType || v.Description != c.description {
			t.Errorf("session %q has agent type %q and description %q; want %q and %q", c.agent, v.AgentType, v.Description, c.agentType, c.description)
		}
	}
	kinds := map[string]string{}
	for _, s := range h.Usage().Sessions {
		kinds[s.Agent] = s.Kind
	}
	if kinds["toolu_review"] != "review" || kinds["toolu_explore"] != "subagent" {
		t.Errorf("session kinds are %v; want the review posted as review", kinds)
	}
}

func TestEachUsageSessionListsTheModelsOfItsResponsesInOrderOfFirstUse(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Paint with two brushes")
	h.Script(id,
		// One response in two content blocks counts its model once.
		factorytest.Response("msg_1", "", "claude-opus-5-5", 100, 0, 0, 10),
		factorytest.Response("msg_1", "", "claude-opus-5-5", 100, 0, 0, 10),
		factorytest.AgentCall("toolu_explore", "Explore", "Find the paint"),
		factorytest.Response("msg_e1", "toolu_explore", "claude-haiku-5-5", 50, 0, 0, 5),
		factorytest.Response("msg_2", "", "claude-sonnet-5-5", 200, 0, 0, 10),
		factorytest.Response("msg_3", "", "claude-opus-5-5", 300, 0, 0, 10),
		factorytest.Finish("Done."),
		factorytest.Result("success"),
	)
	h.Start()
	h.WaitForState(id, store.InReview)

	u := waitForUsage(h, 4)
	want := []usageSession{
		{Ticket: id, Kind: "implementation", Claim: 1, Peak: 300, Responses: 3, Models: []string{"claude-opus-5-5", "claude-sonnet-5-5"}},
		{Ticket: id, Agent: "toolu_explore", Kind: "subagent", Claim: 1, Peak: 50, Responses: 1, Models: []string{"claude-haiku-5-5"}},
	}
	wantUsageSessions(t, u, h.Ticket(id).Claims[0].SessionID, want)
}

func TestASyntheticResponseIsPostedWithItsTokensButNoModel(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Paint after a hiccup")
	h.Script(id,
		factorytest.Response("msg_1", "", "claude-opus-5-5", 100, 0, 0, 10),
		factorytest.Response("msg_synthetic", "", "<synthetic>", 900, 0, 0, 0),
		factorytest.AgentCall("toolu_explore", "Explore", "Find the paint"),
		factorytest.Response("msg_e1", "toolu_explore", "<synthetic>", 50, 0, 0, 0),
		factorytest.AgentCall("toolu_plain", "", "Stir the paint"),
		factorytest.Response("msg_p1", "toolu_plain", "", 60, 0, 0, 5),
		factorytest.Finish("Done."),
		factorytest.Result("success"),
	)
	h.Start()
	h.WaitForState(id, store.InReview)

	u := waitForUsage(h, 4)
	want := []usageSession{
		{Ticket: id, Kind: "implementation", Claim: 1, Peak: 900, Responses: 2, Models: []string{"claude-opus-5-5"}},
		{Ticket: id, Agent: "toolu_explore", Kind: "subagent", Claim: 1, Peak: 50, Responses: 1, Models: []string{}},
		{Ticket: id, Agent: "toolu_plain", Kind: "subagent", Claim: 1, Peak: 60, Responses: 1, Models: []string{}},
	}
	wantUsageSessions(t, u, h.Ticket(id).Claims[0].SessionID, want)
}

func TestAResumedSessionAddsToTheSameUsageSession(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Ask first")
	h.Script(id,
		factorytest.Response("msg_first", "", "", 100, 0, 0, 10),
		factorytest.PostRound("Which colour?"),
		factorytest.Result("success"),
	)
	r := h.Start()
	h.WaitForState(id, store.NeedsAnswer)
	r.WaitForOutput(id + " turn ended")
	waitForUsage(h, 1)

	h.Script(id,
		factorytest.Response("msg_second", "", "", 300, 0, 0, 10),
		factorytest.Finish("Done."),
		factorytest.Result("success"),
	)
	h.Answer(id, "Green")
	r.Poll()
	h.WaitForState(id, store.InReview)

	u := waitForUsage(h, 2)
	want := []usageSession{{Ticket: id, Kind: "implementation", Claim: 1, Peak: 300, Responses: 2, Models: []string{}}}
	wantUsageSessions(t, u, h.Ticket(id).Claims[0].SessionID, want)
}

func TestWithCodeCentralDownMidTurnTheTurnStillFinishesAndDroppedUsageHoldsNothingUp(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Paint through an outage")
	h.Script(id,
		factorytest.Response("msg_before", "", "", 100, 0, 0, 1), factorytest.Wait(),
		factorytest.Response("msg_during", "", "", 200, 0, 0, 1), factorytest.Wait(),
		factorytest.Response("msg_after", "", "", 300, 0, 0, 1), factorytest.Finish("Done."), factorytest.Result("success"))
	r := h.Start()
	waitForUsage(h, 1)

	h.SetDown(true)
	h.Continue(id)
	h.WaitFor("a post of usage to be refused", func() bool { return h.Refused("/api/usage") > 0 })
	h.SetDown(false)
	h.Continue(id)
	h.WaitForState(id, store.InReview)
	r.WaitForOutput(id + " turn ended")

	// The batch posted while down is dropped, never retried.
	u := waitForUsage(h, 2)
	want := []usageSession{{Ticket: id, Kind: "implementation", Claim: 1, Peak: 300, Responses: 2, Models: []string{}}}
	wantUsageSessions(t, u, h.Ticket(id).Claims[0].SessionID, want)
	if strings.Contains(r.Stdout()+r.Stderr(), "/api/usage") {
		t.Errorf("a failed post was reported; want it dropped quietly:\n%s%s", r.Stdout(), r.Stderr())
	}
}
