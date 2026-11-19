package core_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
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
	v, err := f.core.EditSpec(ctx, "acme", api.EditSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"body", "edits"}, 1)

	v, err = f.core.EditSpec(ctx, "acme", api.EditSpecInput{Spec: "1", Title: ptr("Sign in"), Body: ptr("Newer.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"title", "body", "edits"}, 2)

	f.clock.Advance(time.Minute)
	v, err = f.core.EditSpec(ctx, "acme", api.EditSpecInput{Spec: "1", Body: ptr("Newer.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{}, 0)
}

func TestCreateSpecAcknowledgesWithTheNewID(t *testing.T) {
	f := defining(t)
	v, err := f.core.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Search", Body: "Find things.\n"})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-3", t0, []string{"title", "body"}, 0)
	v, err = f.core.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-4", t0, []string{"title"}, 0)
}

func TestLinkedSpecWritesAcknowledge(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	v, err := f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Login", Body: "Users log in.\n", Link: ghLink("10")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0, []string{"title", "body", "link"}, 0)

	f.clock.Advance(time.Minute)
	v, err = f.core.UpdateLinkedSpec(integration, "acme", api.UpdateLinkedSpecInput{Spec: "1", Body: ptr("New.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantAcknowledgement(t, v, "acme-1", t0.Add(time.Minute), []string{"body", "edits"}, 1)
	v, err = f.core.UpdateLinkedSpec(integration, "acme", api.UpdateLinkedSpecInput{Spec: "1", Body: ptr("New.\n")})
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
			return f.core.CreateSpec(ctx, "acme", api.CreateSpecInput{Title: "Search", Full: true})
		},
		"edit_spec": func() (any, error) {
			return f.core.EditSpec(ctx, "acme", api.EditSpecInput{Spec: "1", Body: ptr("New.\n"), Full: true})
		},
		"create_linked_spec": func() (any, error) {
			return f.core.CreateLinkedSpec(integration, "acme", api.CreateLinkedSpecInput{Title: "Epic", Link: ghLink("10"), Full: true})
		},
		"update_linked_spec": func() (any, error) {
			return f.core.UpdateLinkedSpec(integration, "acme", api.UpdateLinkedSpecInput{Spec: "4", Title: ptr("Epic 2"), Full: true})
		},
	}
	for _, name := range []string{"create_spec", "edit_spec", "create_linked_spec", "update_linked_spec"} {
		v, err := writes[name]()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := resultJSON(t, v)
		get, err := f.core.GetSpec(ctx, "acme", api.GetSpecInput{Spec: api.Ref(out["id"].(string))})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// get_spec's full read adds the Spec's Tickets and more to the
		// record, so only the record's keys are compared.
		want := resultJSON(t, get)
		for k := range want {
			if _, ok := resultJSON(t, api.SpecView{})[k]; !ok && k != "link" {
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
func wantTicketAcknowledgement(t *testing.T, v any, id string, updated time.Time, state domain.State, changed []string, edit int) {
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

	v, err := f.core.Comment(ctx, "acme", api.CommentInput{Ticket: "2", Body: "Looks good."})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", at(), domain.Draft, []string{"comments"}, 0)

	e, err := f.core.EditTicket(ctx, "acme", api.EditTicketInput{Ticket: "2", Description: ptr("Build it well.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, e, "acme-2", at(), domain.Draft, []string{"description", "edits"}, 1)
	f.clock.Advance(time.Minute)
	e, err = f.core.EditTicket(ctx, "acme", api.EditTicketInput{Ticket: "2", Description: ptr("Build it well.\n")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, e, "acme-2", at().Add(-time.Minute), domain.Draft, []string{}, 0)

	a, err := f.core.Approve(ctx, "acme", api.TicketInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, a, "acme-2", at(), domain.Ready, []string{"state", "moves", "claimable"}, 0)

	blocker := f.ticket(t, "acme", api.CreateTicketInput{Spec: "1", Title: "API", Full: true})
	b, err := f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "2", Blocker: api.Ref(blocker.ID)})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, b, "acme-2", at(), domain.Ready, []string{"blocked_by", "blockers", "blocked", "claimable"}, 0)
	b, err = f.core.RemoveBlocker(ctx, "acme", api.BlockerInput{Ticket: "2", Blocker: api.Ref(blocker.ID)})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, b, "acme-2", at(), domain.Ready, []string{"blocked_by", "blockers", "blocked", "claimable"}, 0)

	claimed := f.claim(t, api.ClaimInput{Ticket: "2"})
	f.clock.Advance(time.Minute)
	s, err := f.core.SetSession(ctx, "acme", api.SetSessionInput{Ticket: "2", ClaimID: claimed.ClaimID, SessionID: "s-1"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, s, "acme-2", at(), domain.InProgress, []string{"claims"}, 0)
}

func TestCreateTicketAcknowledgesTheFieldsItFilledIn(t *testing.T) {
	f := defining(t)
	v, err := f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "API", Description: "Serve it.\n", Labels: []string{"api"}})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-3", t0, domain.Draft, []string{"title", "state", "spec", "labels", "description", "spec_landed"}, 0)

	v, err = f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "Ready", State: domain.Ready, BlockedBy: []api.Ref{"3"}})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-4", t0, domain.Ready, []string{"title", "state", "spec", "blocked_by", "blockers", "spec_landed", "blocked"}, 0)
}

func TestLinkedTicketWritesAcknowledge(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	v, err := f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Form", Link: ghLink("11")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0, domain.Draft, []string{"title", "state", "spec", "spec_landed", "link"}, 0)

	f.clock.Advance(time.Minute)
	v, err = f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "2", Title: ptr("Login form")})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), domain.Draft, []string{"title", "edits"}, 1)
}

func TestHumanTicketActionsAcknowledgeWithTheNewState(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	v, err := f.core.Move(human, "acme", api.MoveInput{Ticket: "2", ToState: domain.Ready})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), domain.Ready, []string{"state", "moves", "claimable"}, 0)
	v, err = f.core.Defer(ctx, "acme", api.DeferInput{Ticket: "2", Reason: "Not yet."})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), domain.Draft, []string{"state", "comments", "moves", "claimable"}, 0)
	v, err = f.core.Cancel(human, "acme", api.CancelInput{Ticket: "2"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(time.Minute), domain.Cancelled, []string{"state", "moves"}, 0)
}

func TestTicketWritesWithFullReturnTheTicket(t *testing.T) {
	f := defining(t)
	f.clock.Advance(time.Minute)
	writes := []struct {
		name  string
		write func() (any, error)
	}{
		{"create_ticket", func() (any, error) {
			return f.core.CreateTicket(ctx, "acme", api.CreateTicketInput{Spec: "1", Title: "API", Full: true})
		}},
		{"edit_ticket", func() (any, error) {
			return f.core.EditTicket(ctx, "acme", api.EditTicketInput{Ticket: "2", Title: ptr("Login form"), Full: true})
		}},
		{"comment", func() (any, error) {
			return f.core.Comment(ctx, "acme", api.CommentInput{Ticket: "2", Body: "Hi.", Full: true})
		}},
		{"approve", func() (any, error) {
			return f.core.Approve(ctx, "acme", api.TicketInput{Ticket: "2", Full: true})
		}},
		{"add_blocker", func() (any, error) {
			return f.core.AddBlocker(ctx, "acme", api.BlockerInput{Ticket: "2", Blocker: "3", Full: true})
		}},
		{"move", func() (any, error) {
			return f.core.Move(human, "acme", api.MoveInput{Ticket: "3", ToState: domain.Done, Full: true})
		}},
		{"create_linked_ticket", func() (any, error) {
			return f.core.CreateLinkedTicket(integration, "acme", api.CreateLinkedTicketInput{Spec: "1", Title: "Linked", Link: ghLink("11"), Full: true})
		}},
		{"update_linked_ticket", func() (any, error) {
			return f.core.UpdateLinkedTicket(integration, "acme", api.UpdateLinkedTicketInput{Ticket: "4", Title: ptr("Linked 2"), Full: true})
		}},
	}
	for _, w := range writes {
		v, err := w.write()
		if err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}
		out := resultJSON(t, v)
		get, err := f.core.GetTicket(ctx, "acme", whole(api.Ref(out["id"].(string))))
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
		state   domain.State
		changed []string
	}{
		{"accept", fixture.inReview, func(f fixture) (any, error) {
			return f.core.Accept(human, "acme", api.TicketInput{Ticket: "2"})
		}, domain.Done, []string{"state", "moves", "needs_me_reason"}},
		{"reject", fixture.inReview, func(f fixture) (any, error) {
			return f.core.Reject(human, "acme", api.RejectInput{Ticket: "2", Comment: "Not yet."})
		}, domain.Ready, []string{"state", "comments", "moves", "claimable", "needs_me_reason"}},
		{"land", fixture.inReview, func(f fixture) (any, error) {
			return f.core.Land(ctx, "acme", api.LandInput{Ticket: "2", Commit: "def456"})
		}, domain.Done, []string{"state", "moves", "needs_me_reason"}},
		{"send_back", fixture.failed, func(f fixture) (any, error) {
			return f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "2", Note: "Fixed."})
		}, domain.Ready, []string{"state", "comments", "moves", "claimable", "needs_me_reason"}},
		{"to_draft", fixture.failed, func(f fixture) (any, error) {
			return f.core.ToDraft(human, "acme", api.TicketInput{Ticket: "2"})
		}, domain.Draft, []string{"state", "moves", "needs_me_reason"}},
		{"release", func(f fixture, t *testing.T) {
			f.work(t, domain.Ready)
			f.claim(t, api.ClaimInput{})
		}, func(f fixture) (any, error) {
			return f.core.Release(human, "acme", api.ReleaseInput{Ticket: "2"})
		}, domain.Ready, []string{"state", "claims", "moves", "claimable", "silent_since", "needs_me_reason"}}, // a Silent Claim, two hours quiet
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
	v, err := f.core.AcceptRecommended(human, "acme", api.AcceptRecommendedInput{Ticket: "2", Round: 1})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(2*time.Hour), domain.NeedsAnswer, []string{"rounds"}, 0)
	v, err = f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "Postgres"})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(2*time.Hour), domain.NeedsAnswer, []string{"rounds"}, 0)
	v, err = f.core.DismissQuestion(human, "acme", api.DismissQuestionInput{Ticket: "2", Question: 2})
	if err != nil {
		t.Fatal(err)
	}
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(2*time.Hour), domain.Answered, []string{"state", "rounds", "moves", "needs_me_reason"}, 0)
}

func TestClaimEndingActionsAcknowledgeWithTheirNextStep(t *testing.T) {
	at := t0.Add(10 * time.Minute) // before the Claim falls silent
	for _, c := range []struct {
		name    string
		write   func(fixture) (any, error)
		state   domain.State
		changed []string
	}{
		{"finish", func(f fixture) (any, error) {
			return f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done."})
		}, domain.InReview, []string{"state", "claims", "comments", "moves", "needs_me_reason"}},
		{"finish on a branch", func(f fixture) (any, error) {
			return f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done.", Branch: "acme-2", Commit: "abc123"})
		}, domain.InReview, []string{"state", "branch", "commit", "claims", "comments", "moves", "needs_me_reason"}},
		{"report_failure", func(f fixture) (any, error) {
			return f.core.ReportFailure(ctx, "acme", report("2", "claim-1"))
		}, domain.Failed, []string{"state", "claims", "failure_reports", "moves", "needs_me_reason"}},
		{"give_up", func(f fixture) (any, error) {
			return f.core.GiveUp(ctx, "acme", api.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: "Out of context."})
		}, domain.Ready, []string{"state", "claims", "comments", "moves", "claimable"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.work(t, domain.Ready)
			f.claim(t, api.ClaimInput{})
			f.clock.Set(at)
			v, err := c.write(f)
			if err != nil {
				t.Fatal(err)
			}
			wantTicketAcknowledgement(t, v, "acme-2", at, c.state, c.changed, 0)
			if out := resultJSON(t, v); out["next_step"] != "stop working this Ticket" || out["ticket"] != nil {
				t.Fatalf("acknowledgement %v, want the next step and no ticket", out)
			}
		})
	}
}

func TestPostRoundAcknowledgesWithTheRoundAndTheNextStep(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, api.ClaimInput{})
	f.clock.Set(t0.Add(10 * time.Minute)) // before the Claim falls silent
	v, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []api.QuestionInput{qDB}})
	if err != nil {
		t.Fatal(err)
	}
	// claims: posting a Round is the Claim's latest activity.
	wantTicketAcknowledgement(t, v, "acme-2", t0.Add(10*time.Minute), domain.NeedsAnswer, []string{"state", "claims", "rounds", "moves", "needs_me_reason"}, 0)
	out := resultJSON(t, v)
	if out["round"] != float64(1) || out["next_step"] != "End your turn now, after running `code-central wait acme-2` in the background if you can run background commands." {
		t.Fatalf("acknowledgement %v, want round 1 and the next step", out)
	}
}

func TestClaimEndingActionsAndPostRoundWithFullReturnTheirFormerResult(t *testing.T) {
	for _, c := range []struct {
		name  string
		write func(fixture) (any, error)
		step  string
	}{
		{"finish", func(f fixture) (any, error) {
			return f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done.", Full: true})
		}, "stop working this Ticket"},
		{"report_failure", func(f fixture) (any, error) {
			return f.core.ReportFailure(ctx, "acme", fullReport("2", "claim-1"))
		}, "stop working this Ticket"},
		{"give_up", func(f fixture) (any, error) {
			return f.core.GiveUp(ctx, "acme", api.GiveUpInput{Ticket: "2", ClaimID: "claim-1", Note: "Out of context.", Full: true})
		}, "stop working this Ticket"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			f.work(t, domain.Ready)
			f.claim(t, api.ClaimInput{})
			v, err := c.write(f)
			if err != nil {
				t.Fatal(err)
			}
			get, err := f.core.GetTicket(ctx, "acme", whole("2"))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"ticket": resultJSON(t, get), "next_step": c.step}
			if out := resultJSON(t, v); !reflect.DeepEqual(out, want) {
				t.Fatalf("with full: %v, want %v", out, want)
			}
		})
	}
	t.Run("post_round", func(t *testing.T) {
		f := newFixture(t)
		f.work(t, domain.Ready)
		f.claim(t, api.ClaimInput{})
		v, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []api.QuestionInput{qName}, Full: true})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"round": float64(1), "next_step": "End your turn now, after running `code-central wait acme-2` in the background if you can run background commands."}
		if out := resultJSON(t, v); !reflect.DeepEqual(out, want) {
			t.Fatalf("with full: %v, want %v", out, want)
		}
	})
}
