package factory_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/factory/factorytest"
	"github.com/tomoram/code-central/store"
)

// newMap creates a Map, a Spec labelled wayfinder:map, with these Tags,
// and returns its ID.
func newMap(t *testing.T, h *factorytest.Rig, title string, tags map[string]string) string {
	t.Helper()
	ctx := context.Background()
	c := h.Server.Client
	s, err := c.CreateSpec(ctx, h.Project, api.CreateSpecInput{Title: title})
	mustNot(t, err)
	_, err = c.AddLabels(ctx, h.Project, api.LabelsInput{Target: api.Ref(s.ID), Labels: []string{domain.MapLabel}})
	mustNot(t, err)
	if len(tags) > 0 {
		_, err = h.Human().SetTags(ctx, h.Project, api.SetTagsInput{Target: api.Ref(s.ID), Tags: tags})
		mustNot(t, err)
	}
	return s.ID
}

// guidedMap is newMap with its guided Tag on.
func guidedMap(t *testing.T, h *factorytest.Rig, title string) string {
	t.Helper()
	return newMap(t, h, title, map[string]string{domain.GuidedTag: domain.GuidedOn})
}

// decision creates a Draft Ticket carrying label on Map m, as Charting
// creates its decision Tickets, and returns its ID.
func decision(t *testing.T, h *factorytest.Rig, m, label, title string) string {
	t.Helper()
	tk, err := h.Server.Client.CreateTicket(context.Background(), h.Project, api.CreateTicketInput{Spec: api.Ref(m), Title: title, Labels: []string{label}})
	mustNot(t, err)
	return tk.ID
}

// putState puts Ticket id in state directly in the store, with no Claim,
// as if it had got there on its own.
func putState(t *testing.T, h *factorytest.Rig, id string, state domain.State) {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimPrefix(id, h.Project+"-"))
	mustNot(t, err)
	err = h.Server.Store.Update(func(w store.Writer) error {
		tk, _ := w.Ticket(h.Project, n)
		tk.State = state
		return w.PutTicket(h.Project, tk)
	})
	mustNot(t, err)
}

// mustNot fails the test on err.
func mustNot(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// wantState checks Ticket id is in want, saying why it should be.
func wantState(t *testing.T, h *factorytest.Rig, id string, want domain.State, why string) {
	t.Helper()
	if got := h.Ticket(id).State; got != want {
		t.Errorf("%s is %s, want %s: %s", id, got, want, why)
	}
}

func TestTheFactoryStartsOneDecisionPerIdleGuidedMapInCreatedOrder(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	m := guidedMap(t, h, "Offline mode")
	first := decision(t, h, m, "wayfinder:task", "Ask legal")
	second := decision(t, h, m, "wayfinder:task", "Ask ops")
	other := guidedMap(t, h, "Dark mode")
	blocked := decision(t, h, other, "wayfinder:task", "Pick colours")
	unblocked := decision(t, h, other, "wayfinder:task", "Pick a toggle")
	_, err := h.Server.Client.AddBlocker(context.Background(), h.Project, api.BlockerInput{Ticket: api.Ref(blocked), Blocker: api.Ref(unblocked)})
	mustNot(t, err)
	h.Script(first, factorytest.Wait(), factorytest.Finish("Legal says yes."), factorytest.Result("success"))

	r := h.Start()
	h.WantState(first, domain.InProgress)
	r.WantOutput(first + ` approved (guided Map ` + m + `): "Ask legal"`)
	wantState(t, h, second, domain.Draft, "one Ticket per Map per poll")
	if s := h.Ticket(unblocked).State; s == domain.Draft {
		t.Errorf("%s is still Draft: each idle Map starts its first frontier Ticket", unblocked)
	}
	wantState(t, h, blocked, domain.Draft, "a blocked Ticket isn't on the frontier")

	h.Continue(first)
	r.Settle()
	h.WantState(first, domain.InReview)
	r.Poll()
	wantState(t, h, second, domain.Draft, "a Ticket In Review holds its Map")

	h.Accept(first)
	wantState(t, h, first, domain.Done, "a Map Ticket finished with no branch is Done when accepted")
	r.Poll()
	if s := h.Ticket(second).State; s == domain.Draft {
		t.Errorf("%s is still Draft once its Map is idle again", second)
	}
}

func TestAGuidedMapIsHeldByAnyNonResearchTicketThatWaitsOnTheHumanOrAnAgent(t *testing.T) {
	t.Parallel()
	for _, state := range []domain.State{domain.Ready, domain.InProgress, domain.NeedsAnswer, domain.Answered, domain.Failed, domain.InReview, domain.Accepted} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t, factorytest.FakeGit())
			m := guidedMap(t, h, "Offline mode")
			holding := decision(t, h, m, "wayfinder:grilling", "Which store")
			next := decision(t, h, m, "wayfinder:task", "Ask legal")
			putState(t, h, holding, state)
			h.Script(holding, factorytest.Wait())

			r := h.Start()
			r.Poll()
			wantState(t, h, next, domain.Draft, "a "+string(state)+" Ticket holds its Map")
		})
	}
}

func TestAGuidedMapIsNotHeldByResearch(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	m := guidedMap(t, h, "Offline mode")
	research := decision(t, h, m, "wayfinder:research", "Sync libraries")
	next := decision(t, h, m, "wayfinder:grilling", "Which store")
	putState(t, h, research, domain.InProgress)
	h.Script(next, factorytest.Wait())

	h.Start()
	h.WantState(next, domain.InProgress)
}

func TestAMapIsGuidedOnlyWhenItsGuidedTagIsOn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	var drafts []string
	for _, tags := range []map[string]string{nil, {domain.GuidedTag: "off"}, {domain.GuidedTag: "true"}, {domain.GuidedTag: "On"}} {
		m := newMap(t, h, "Offline mode", tags)
		drafts = append(drafts, decision(t, h, m, "wayfinder:task", "Ask legal"))
	}

	r := h.Start()
	r.Poll()
	for _, id := range drafts {
		wantState(t, h, id, domain.Draft, "a Map is guided only by guided: on")
	}
}

func TestAPausedFactoryStartsNoDecision(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t, factorytest.FakeGit())
	cut := h.ReadyTicket("Cut off")
	h.Script(cut, factorytest.APIError("rate_limit"), factorytest.Result("success"), factorytest.Exit(1))
	r := h.Start()
	r.WantOutput(cut + " turn ended")

	m := guidedMap(t, h, "Offline mode")
	next := decision(t, h, m, "wayfinder:task", "Ask legal")
	r.Poll()
	wantState(t, h, next, domain.Draft, "nothing is started during an outage")
}
