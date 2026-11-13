package core_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// Auto-accept (Spec code-central-536, ADR 0037): the Project's setting
// per Auto-accept kind, set by the human alone, and finish carrying out
// the human's accept when the finished Ticket is eligible.

// projectView reads the Project as the UI does.
func (f fixture) projectView(t *testing.T, key string) core.ProjectView {
	t.Helper()
	p, err := f.core.GetProject(ctx, key)
	if err != nil {
		t.Fatalf("get_project %s: %v", key, err)
	}
	return p
}

// setAutoAccept turns the given kinds on or off on acme as the human.
func (f fixture) setAutoAccept(t *testing.T, kinds map[store.AutoAcceptKind]bool) core.ProjectView {
	t.Helper()
	v, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: kinds, Full: true})
	if err != nil {
		t.Fatalf("set_auto_accept: %v", err)
	}
	return *v.Record
}

func TestSetAutoAcceptTurnsKindsOnAndOffAndTheProjectReadsThem(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	if p := f.projectView(t, "acme"); len(p.AutoAccept) != 5 || p.AutoAccept["implementation"] {
		t.Fatalf("a new Project's auto_accept = %v, want every kind off", p.AutoAccept)
	}

	p := f.setAutoAccept(t, map[store.AutoAcceptKind]bool{"implementation": true, "merge": true})
	if !p.AutoAccept["implementation"] || !p.AutoAccept["merge"] || p.AutoAccept["grilling"] || p.AutoAccept["spec"] || p.AutoAccept["tickets"] {
		t.Fatalf("auto_accept = %v, want implementation and merge on", p.AutoAccept)
	}

	// A kind left out keeps its value.
	p = f.setAutoAccept(t, map[store.AutoAcceptKind]bool{"merge": false, "grilling": true})
	if !p.AutoAccept["implementation"] || p.AutoAccept["merge"] || !p.AutoAccept["grilling"] {
		t.Fatalf("auto_accept = %v, want implementation and grilling on", p.AutoAccept)
	}
	if got := f.projectView(t, "acme").AutoAccept; got["implementation"] != true || got["grilling"] != true || got["merge"] {
		t.Fatalf("get_project auto_accept = %v", got)
	}
}

func TestSetAutoAcceptAcknowledgesTheSettingAndWritesNothingWhenUnchanged(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	v, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"tickets": true}})
	if err != nil || v.ID != "acme" || len(v.Changed) != 1 || v.Changed[0] != "auto_accept" {
		t.Fatalf("acknowledgement %+v, %v", v, err)
	}
	v, err = f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"tickets": true}})
	if err != nil || len(v.Changed) != 0 {
		t.Fatalf("acknowledgement of no change %+v, %v", v, err)
	}
}

func TestSetAutoAcceptIsTheHumansAloneAndRefusesAnUnknownKind(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, actor := range []core.Actor{core.Agent, core.Integration} {
		_, err := f.core.SetAutoAccept(core.WithActor(ctx, actor), "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"implementation": true}})
		var e *core.Error
		if !errors.As(err, &e) || e.Code != core.ActionNotAllowed {
			t.Fatalf("as %s: %v, want action_not_allowed", actor, err)
		}
	}
	_, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[store.AutoAcceptKind]bool{"prototype": true}})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid || e.Field != "kinds" {
		t.Fatalf("unknown kind: %v, want invalid on kinds", err)
	}
	if p := f.projectView(t, "acme"); p.AutoAccept["implementation"] {
		t.Fatalf("auto_accept = %v, want nothing on after refusals", p.AutoAccept)
	}
}

// finishWork claims and finishes acme-2, a plain implementation Ticket on
// a Long-lived Spec, naming a branch when branch isn't empty.
func (f fixture) finishWork(t *testing.T, branch string) core.FinishResult {
	t.Helper()
	f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	res, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-2", ClaimID: "claim-1", Summary: "Done on acme-2.", Branch: branch})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return res
}

// autoAccepted checks the Ticket's last Move is an auto-accept by the
// human from In Review to state, with cause, after the agent's move to
// In Review.
func autoAccepted(t *testing.T, tk core.TicketView, to store.State, cause store.AutoAcceptCause) {
	t.Helper()
	if tk.State != to {
		t.Fatalf("%s is %s, want %s", tk.ID, tk.State, to)
	}
	n := len(tk.Moves)
	if n < 2 {
		t.Fatalf("moves %+v", tk.Moves)
	}
	if m := tk.Moves[n-2]; m.To != store.InReview || m.Actor != "agent" || m.Cause != "" {
		t.Fatalf("finish's move %+v, want to In Review by the agent with no cause", m)
	}
	if m := tk.Moves[n-1]; m.From != store.InReview || m.To != to || m.Actor != "human" || m.Cause != cause {
		t.Fatalf("accept's move %+v, want In Review to %s by human with cause %q", m, to, cause)
	}
}

func TestFinishAutoAcceptsAnImplementationTicketUnderTheProjectSetting(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptImplementation: true})
	res := f.finishWork(t, "acme-2")

	autoAccepted(t, f.get(t, "acme-2"), store.Accepted, store.CauseProject)
	// finish's response is the agent's, as if nothing followed.
	if res.Acknowledgement == nil || res.Acknowledgement.State != store.InReview || res.NextStep != "stop working this Ticket" {
		t.Fatalf("finish result %+v, want the In Review acknowledgement", res)
	}
	if a, _ := f.core.AttentionOf(ctx, "acme", "acme-2"); a.NeedsMe {
		t.Fatalf("an auto-accepted Ticket is in Needs Me")
	}
}

func TestFinishAutoAcceptsToDoneWhenNoBranchWasNamed(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptImplementation: true})
	f.finishWork(t, "")
	autoAccepted(t, f.get(t, "acme-2"), store.Done, store.CauseProject)
}

func TestFinishLeavesTheTicketInReviewWhileItsKindIsOff(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptMerge: true, store.AutoAcceptGrilling: true, store.AutoAcceptSpec: true, store.AutoAcceptTickets: true})
	res := f.finishWork(t, "acme-2")
	tk := f.get(t, "acme-2")
	if tk.State != store.InReview || tk.Moves[len(tk.Moves)-1].Cause != "" {
		t.Fatalf("ticket %s with moves %+v, want In Review", tk.State, tk.Moves)
	}
	if res.Acknowledgement == nil || res.Acknowledgement.State != store.InReview {
		t.Fatalf("finish result %+v", res)
	}
}

func TestFinishsResponseIsTheSameWhetherOrNotItAutoAccepts(t *testing.T) {
	var results []core.FinishResult
	var fulls []core.FinishResult
	for _, on := range []bool{false, true} {
		f := newFixture(t)
		f.work(t, store.Ready)
		f.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptImplementation: on})
		f.clock.Set(t0.Add(time.Hour))
		results = append(results, f.finishWork(t, "acme-2"))

		g := newFixture(t)
		g.work(t, store.Ready)
		g.setAutoAccept(t, map[store.AutoAcceptKind]bool{store.AutoAcceptImplementation: on})
		g.clock.Set(t0.Add(time.Hour))
		g.claim(t, core.ClaimInput{Ticket: "acme-2"})
		full, err := g.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-2", ClaimID: "claim-1", Summary: "Done on acme-2.", Branch: "acme-2", Full: true})
		if err != nil {
			t.Fatal(err)
		}
		fulls = append(fulls, full)
	}
	if !reflect.DeepEqual(results[0], results[1]) {
		t.Errorf("finish's acknowledgement differs under auto-accept:\n%+v\n%+v", *results[0].Acknowledgement, *results[1].Acknowledgement)
	}
	if !reflect.DeepEqual(fulls[0], fulls[1]) {
		t.Errorf("finish's full result differs under auto-accept:\n%+v\n%+v", *fulls[0].Ticket, *fulls[1].Ticket)
	}
}
