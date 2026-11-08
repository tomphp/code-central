package core_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// resultJSON is a write's result as a client sees it.
func resultJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// wantAcknowledgement checks a write's result is the Acknowledgement of
// id, changing exactly changed, with edit number edit (0: none).
func wantAcknowledgement(t *testing.T, v any, id string, updated time.Time, changed []string, edit int) {
	t.Helper()
	out := resultJSON(t, v)
	got := []string{}
	for _, c := range out["changed"].([]any) {
		got = append(got, c.(string))
	}
	if out["id"] != id || out["updated"] != updated.Format(time.RFC3339) || !slices.Equal(got, changed) {
		t.Fatalf("acknowledgement %v, want id %s, updated %s, changed %v", out, id, updated, changed)
	}
	if e, ok := out["edit"]; edit == 0 && ok || edit != 0 && e != float64(edit) {
		t.Fatalf("acknowledgement %v, want edit %d", out, edit)
	}
	for _, k := range []string{"title", "body", "edits"} {
		if _, ok := out[k]; ok {
			t.Fatalf("acknowledgement %v has the record's %s", out, k)
		}
	}
}

func TestEditSpecAcknowledgesTheFieldsItChanged(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"body", "edits"}, 1)

	v, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Title: ptr("Sign in"), Body: ptr("Newer.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"title", "body", "edits"}, 2)

	f.clock.Advance(time.Minute)
	v, err = f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: ptr("Newer.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{}, 0)
}

func TestCreateSpecAcknowledgesWithTheNewID(t *testing.T) {
	f := defining(t)
	v, err := f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Search", Body: "Find things.\n"})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-3", t0, []string{"title", "body"}, 0)
	v, err = f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-4", t0, []string{"title"}, 0)
}

func TestLinkedSpecWritesAcknowledge(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	v, err := f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Login", Body: "Users log in.\n", Link: ghLink("10")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0, []string{"title", "body", "link"}, 0)

	f.clock.Advance(time.Minute)
	v, err = f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"body", "edits"}, 1)
	v, err = f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{}, 0)
}

func TestSpecWritesWithFullReturnTheSpec(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	writes := map[string]func() (any, error){
		"create_spec": func() (any, error) {
			return f.core.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Search", Full: true})
		},
		"edit_spec": func() (any, error) {
			return f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "1", Body: ptr("New.\n"), Full: true})
		},
		"create_linked_spec": func() (any, error) {
			return f.core.CreateLinkedSpec(integration, "acme", core.CreateLinkedSpecInput{Title: "Epic", Link: ghLink("10"), Full: true})
		},
		"update_linked_spec": func() (any, error) {
			return f.core.UpdateLinkedSpec(integration, "acme", core.UpdateLinkedSpecInput{Spec: "4", Title: ptr("Epic 2"), Full: true})
		},
	}
	for _, name := range []string{"create_spec", "edit_spec", "create_linked_spec", "update_linked_spec"} {
		v, err := writes[name]()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := resultJSON(t, v)
		get, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: core.Ref(out["id"].(string))})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// get_spec's full read adds the Spec's Tickets and more to the
		// record, so only the record's keys are compared.
		want := resultJSON(t, get)
		for k := range want {
			if _, ok := resultJSON(t, core.SpecView{})[k]; !ok && k != "link" {
				delete(want, k)
			}
		}
		if !reflect.DeepEqual(out, want) {
			t.Fatalf("%s with full: %v, want the Spec %v", name, out, want)
		}
	}
}

// wantTicketAcknowledgement is wantAcknowledgement for a Ticket write,
// which also gives the Ticket's State after it and none of its record.
func wantTicketAcknowledgement(t *testing.T, v any, id string, updated time.Time, state store.State, changed []string, edit int) {
	t.Helper()
	wantAcknowledgement(t, v, id, updated, changed, edit)
	out := resultJSON(t, v)
	if out["state"] != string(state) {
		t.Fatalf("acknowledgement %v, want state %s", out, state)
	}
	for _, k := range []string{"description", "comments", "moves", "claims", "rounds"} {
		if _, ok := out[k]; ok {
			t.Fatalf("acknowledgement %v has the record's %s", out, k)
		}
	}
}

func TestTicketWritesAcknowledgeTheFieldsTheyChangedAndTheState(t *testing.T) {
	f := defining(t) // acme-2, a Draft Ticket on Spec acme-1
	at := func() time.Time { return f.clock.Now() }
	f.clock.Advance(time.Minute)

	v, err := f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Looks good."})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", at(), store.Draft, []string{"comments"}, 0)

	e, err := f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Description: ptr("Build it well.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, e, "acme-2", at(), store.Draft, []string{"description", "edits"}, 1)
	f.clock.Advance(time.Minute)
	e, err = f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Description: ptr("Build it well.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, e, "acme-2", at().Add(-time.Minute), store.Draft, []string{}, 0)

	a, err := f.core.Approve(ctx, "acme", core.TicketInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, a, "acme-2", at(), store.Ready, []string{"state", "moves", "claimable"}, 0)

	blocker := f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "API", Full: true})
	b, err := f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "2", Blocker: core.Ref(blocker.ID)})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, b, "acme-2", at(), store.Ready, []string{"blocked_by", "blockers", "blocked", "claimable"}, 0)
	b, err = f.core.RemoveBlocker(ctx, "acme", core.BlockerInput{Ticket: "2", Blocker: core.Ref(blocker.ID)})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, b, "acme-2", at(), store.Ready, []string{"blocked_by", "blockers", "blocked", "claimable"}, 0)

	claimed := f.claim(t, core.ClaimInput{Ticket: "2"})
	f.clock.Advance(time.Minute)
	s, err := f.core.SetSession(ctx, "acme", core.SetSessionInput{Ticket: "2", ClaimID: claimed.ClaimID, SessionID: "s-1"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, s, "acme-2", at(), store.InProgress, []string{"claims"}, 0)
}

func TestCreateTicketAcknowledgesTheFieldsItFilledIn(t *testing.T) {
	f := defining(t)
	v, err := f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "API", Description: "Serve it.\n", Labels: []string{"api"}})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-3", t0, store.Draft, []string{"title", "state", "spec", "labels", "description"}, 0)

	v, err = f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Ready", State: store.Ready, BlockedBy: []core.Ref{"3"}})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-4", t0, store.Ready, []string{"title", "state", "spec", "blocked_by", "blockers", "blocked"}, 0)
}

func TestLinkedTicketWritesAcknowledge(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	v, err := f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Form", Link: ghLink("11")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0, store.Draft, []string{"title", "state", "spec", "link"}, 0)

	f.clock.Advance(time.Minute)
	v, err = f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "2", Title: ptr("Login form")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), store.Draft, []string{"title", "edits"}, 1)
}

func TestHumanTicketActionsAcknowledgeWithTheNewState(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.Move(human, "acme", core.MoveInput{Ticket: "2", ToState: store.Ready})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), store.Ready, []string{"state", "moves", "claimable"}, 0)
	v, err = f.core.Defer(ctx, "acme", core.DeferInput{Ticket: "2", Reason: "Not yet."})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), store.Draft, []string{"state", "comments", "moves", "claimable"}, 0)
	v, err = f.core.Cancel(human, "acme", core.CancelInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), store.Cancelled, []string{"state", "moves"}, 0)
}

func TestTicketWritesWithFullReturnTheTicket(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	writes := []struct {
		name  string
		write func() (any, error)
	}{
		{"create_ticket", func() (any, error) {
			return f.core.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "API", Full: true})
		}},
		{"edit_ticket", func() (any, error) {
			return f.core.EditTicket(ctx, "acme", core.EditTicketInput{Ticket: "2", Title: ptr("Login form"), Full: true})
		}},
		{"comment", func() (any, error) {
			return f.core.Comment(ctx, "acme", core.CommentInput{Ticket: "2", Body: "Hi.", Full: true})
		}},
		{"approve", func() (any, error) {
			return f.core.Approve(ctx, "acme", core.TicketInput{Ticket: "2", Full: true})
		}},
		{"add_blocker", func() (any, error) {
			return f.core.AddBlocker(ctx, "acme", core.BlockerInput{Ticket: "2", Blocker: "3", Full: true})
		}},
		{"move", func() (any, error) {
			return f.core.Move(human, "acme", core.MoveInput{Ticket: "3", ToState: store.Done, Full: true})
		}},
		{"create_linked_ticket", func() (any, error) {
			return f.core.CreateLinkedTicket(integration, "acme", core.CreateLinkedTicketInput{Spec: "1", Title: "Linked", Link: ghLink("11"), Full: true})
		}},
		{"update_linked_ticket", func() (any, error) {
			return f.core.UpdateLinkedTicket(integration, "acme", core.UpdateLinkedTicketInput{Ticket: "4", Title: ptr("Linked 2"), Full: true})
		}},
	}
	for _, w := range writes {
		v, err := w.write()
		if err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}
		out := resultJSON(t, v)
		get, err := f.core.GetTicket(ctx, "acme", core.Ref(out["id"].(string)))
		if err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}
		if want := resultJSON(t, get); !reflect.DeepEqual(out, want) {
			t.Fatalf("%s with full: %v, want the Ticket %v", w.name, out, want)
		}
	}
}

func TestReviewAndFailureActionsAcknowledgeWithTheNewState(t *testing.T) {
	at := t0.Add(2 * time.Hour)
	for _, c := range []struct {
		name    string
		setup   func(fixture, *testing.T)
		write   func(fixture) (any, error)
		state   store.State
		changed []string
	}{
		{"accept", fixture.inReview, func(f fixture) (any, error) {
			return f.core.Accept(human, "acme", core.TicketInput{Ticket: "2"})
		}, store.Done, []string{"state", "moves", "needs_me_reason"}},
		{"reject", fixture.inReview, func(f fixture) (any, error) {
			return f.core.Reject(human, "acme", core.RejectInput{Ticket: "2", Comment: "Not yet."})
		}, store.Ready, []string{"state", "comments", "moves", "claimable", "needs_me_reason"}},
		{"land", fixture.inReview, func(f fixture) (any, error) {
			return f.core.Land(ctx, "acme", core.LandInput{Ticket: "2", Commit: "def456"})
		}, store.Done, []string{"state", "moves", "needs_me_reason"}},
		{"send_back", fixture.failed, func(f fixture) (any, error) {
			return f.core.SendBack(human, "acme", core.SendBackInput{Ticket: "2", Note: "Fixed."})
		}, store.Ready, []string{"state", "comments", "moves", "claimable", "needs_me_reason"}},
		{"to_draft", fixture.failed, func(f fixture) (any, error) {
			return f.core.ToDraft(human, "acme", core.TicketInput{Ticket: "2"})
		}, store.Draft, []string{"state", "moves", "needs_me_reason"}},
		{"release", func(f fixture, t *testing.T) {
			f.work(t, store.Ready)
			f.claim(t, core.ClaimInput{})
		}, func(f fixture) (any, error) {
			return f.core.Release(human, "acme", core.ReleaseInput{Ticket: "2"})
		}, store.Ready, []string{"state", "claims", "moves", "claimable", "silent_since", "needs_me_reason"}}, // a Silent Claim, two hours quiet
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			c.setup(f, t)
			f.clock.Set(at)
			v, err := c.write(f)
			if err != nil {
				t.Fatal(err)
			}
			wantTicketAcknowledgement(t, v, "acme-2", at, c.state, c.changed, 0)
		})
	}
}

func TestRoundActionsAcknowledgeTheRoundsAndAnsweredOnTheLastOne(t *testing.T) {
	f := newFixture(t)
	f.asking(t, qDB, qName)
	f.clock.Set(t0.Add(2 * time.Hour))
	v, err := f.core.AcceptRecommended(human, "acme", core.AcceptRecommendedInput{Ticket: "2", Round: 1})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(2*time.Hour), store.NeedsAnswer, []string{"rounds"}, 0)
	v, err = f.core.Answer(human, "acme", core.AnswerInput{Ticket: "2", Question: 1, Text: "Postgres"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(2*time.Hour), store.NeedsAnswer, []string{"rounds"}, 0)
	v, err = f.core.DismissQuestion(human, "acme", core.DismissQuestionInput{Ticket: "2", Question: 2})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(2*time.Hour), store.Answered, []string{"state", "rounds", "moves", "needs_me_reason"}, 0)
}
