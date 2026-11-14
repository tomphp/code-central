package core_test

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// finishedSpec is Project acme with Spec acme-1, Login, finished: Stage
// Ticket acme-2 Done after a Claim with a session that finished, Prototype
// acme-3 Cancelled after a Claim with no session, and acme-4 Done after a
// released Claim and a second one, the first with no working folder.
func (f fixture) finishedSpec(t *testing.T) {
	t.Helper()
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Grill it", State: store.Ready, Labels: []string{"stage:grilling"}})
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Prototype: theme", State: store.Ready, Labels: []string{core.PrototypeLabel}})
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "1", Title: "Build it", State: store.Ready})

	grill := f.claim(t, core.ClaimInput{Ticket: "2", SessionID: "sess-1", WorkingDir: ptr("/w/one")})
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: grill.ClaimID, Summary: "Grilled."}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	f.move(t, "2", store.Done)
	f.claim(t, core.ClaimInput{Ticket: "3", WorkingDir: ptr("/w/two")})
	f.move(t, "3", store.Cancelled)
	f.claim(t, core.ClaimInput{Ticket: "4", SessionID: "sess-2"})
	if _, err := f.core.Release(human, "acme", core.ReleaseInput{Ticket: "4"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	f.claim(t, core.ClaimInput{Ticket: "4", SessionID: "sess-3", WorkingDir: ptr("/w/three")})
	f.move(t, "4", store.Done)
}

func (f fixture) submitRetro(t *testing.T, in core.SubmitRetroInput) core.TicketView {
	t.Helper()
	in.Full = true
	tk, err := record(f.core.SubmitRetro(ctx, "acme", in))
	if err != nil {
		t.Fatalf("submit_retro: %v", err)
	}
	return tk
}

// wantRetro checks tk is a Ready Retro of target on Spec spec, titled
// title, with one move to Ready and description.
func wantRetro(t *testing.T, tk core.TicketView, spec, title, target, description string) {
	t.Helper()
	if tk.Title != title || tk.State != store.Ready || tk.Spec.ID != spec || !tk.Claimable ||
		!slices.Equal(tk.Labels, []string{core.RetroLabel}) || !maps.Equal(tk.Tags, map[string]string{"retro-of": target}) {
		t.Fatalf("Retro %+v", tk)
	}
	if len(tk.Moves) != 1 || tk.Moves[0].Actor != "human" || tk.Moves[0].To != store.Ready || tk.Moves[0].Via != "submit_retro" {
		t.Fatalf("moves %+v", tk.Moves)
	}
	if tk.Description != description {
		t.Fatalf("description:\n%s\nwant:\n%s", tk.Description, description)
	}
}

func TestARetroOnAFinishedSpecListsEveryClaimOnItsTickets(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	tk := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-1"})
	wantRetro(t, tk, "acme-5", "Retro: Login", "acme-1", "Retro on Spec [acme-1](acme-1): Login.\n"+
		"\n## Claims to read\n\n"+
		"- acme-2 (Grill it), Claim 1: session_id `sess-1`, working_dir `/w/one`, ended by finish.\n"+
		"- acme-3 (Prototype: theme), Claim 1: no session to read, working_dir `/w/two`, ended by move.\n"+
		"- acme-4 (Build it), Claim 1: session_id `sess-2`, no working_dir, ended by release.\n"+
		"- acme-4 (Build it), Claim 2: session_id `sess-3`, working_dir `/w/three`, ended by move.\n")
}

func TestARetroOnADoneTicketListsItsClaims(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	tk := f.submitRetro(t, core.SubmitRetroInput{Target: "4"})
	wantRetro(t, tk, "acme-5", "Retro: Build it", "acme-4", "Retro on Ticket [acme-4](acme-4): Build it.\n"+
		"\n## Claims to read\n\n"+
		"- acme-4 (Build it), Claim 1: session_id `sess-2`, no working_dir, ended by release.\n"+
		"- acme-4 (Build it), Claim 2: session_id `sess-3`, working_dir `/w/three`, ended by move.\n")
}

func TestARetroOnACancelledTicketShowsTheNoteUnderItsOwnHeading(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	tk := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-3", Note: "  Why did we drop it?\n"})
	wantRetro(t, tk, "acme-5", "Retro: Prototype: theme", "acme-3", "Retro on Ticket [acme-3](acme-3): Prototype: theme.\n"+
		"\n## Note\n\nWhy did we drop it?\n"+
		"\n## Claims to read\n\n"+
		"- acme-3 (Prototype: theme), Claim 1: no session to read, working_dir `/w/two`, ended by move.\n")
}

func TestARetroOnWorkWithNoClaimsStillOpens(t *testing.T) {
	f := newFixture(t)
	f.finishedWork(t) // acme-2 Done and acme-3 Cancelled, never claimed
	tk := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-2", Note: " \n"})
	wantRetro(t, tk, "acme-4", "Retro: Ta", "acme-2", "Retro on Ticket [acme-2](acme-2): Ta.\n"+
		"\n## Claims to read\n\nNone: no agent ever claimed this work, so there is no session to read.\n")
}

func TestTheFirstRetroCreatesTheRetrosSpecAndALaterOneReusesItAfterARetitle(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	first := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-2"})
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: core.Ref(first.Spec.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if sp.ID != "acme-5" || sp.Title != "Retros" || !slices.Equal(sp.Labels, []string{core.LongLivedLabel}) || sp.Body == nil || *sp.Body == "" {
		t.Fatalf("Retros Spec %+v", sp)
	}
	var p store.Project
	f.store.View(func(r store.Reader) error { p, _ = r.Project("acme"); return nil })
	if p.RetrosSpec != 5 {
		t.Fatalf("the Project names Retros Spec %d", p.RetrosSpec)
	}
	if _, err := f.core.EditSpec(ctx, "acme", core.EditSpecInput{Spec: "acme-5", Title: ptr("Looking back")}); err != nil {
		t.Fatal(err)
	}
	if second := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-4"}); second.Spec.ID != "acme-5" || second.ID != "acme-7" {
		t.Fatalf("second Retro %+v", second)
	}
}

func TestARetroCanTargetARetroAndSeveralCanTargetTheSameWork(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	first := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-2"})
	second := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-2"})
	if first.ID == second.ID || second.Tags["retro-of"] != "acme-2" {
		t.Fatalf("Retros %s and %+v", first.ID, second)
	}
	f.move(t, core.Ref(first.ID), store.Done)
	onRetro := f.submitRetro(t, core.SubmitRetroInput{Target: core.Ref(first.ID)})
	if onRetro.Title != "Retro: Retro: Grill it" || onRetro.Tags["retro-of"] != first.ID || onRetro.Spec.ID != "acme-5" {
		t.Fatalf("Retro on a Retro %+v", onRetro)
	}
}

func TestSubmittingARetroAcknowledgesItsTicket(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	res, err := f.core.SubmitRetro(ctx, "acme", core.SubmitRetroInput{Target: "acme-2"})
	if err != nil {
		t.Fatalf("submit_retro: %v", err)
	}
	wantTicketAcknowledgement(t, res, "acme-6", t0, store.Ready,
		[]string{"title", "state", "spec", "labels", "tags", "description", "moves", "claimable"}, 0)
}

func TestARetroIsRefusedOnWorkThatIsMissingUnfinishedOrLongLived(t *testing.T) {
	for name, target := range map[string]core.Ref{
		"no target":         "",
		"not an ID":         "nope",
		"another Project's": "other-2",
		"not found":         "acme-99",
		"unfinished Ticket": "acme-4",
		"unfinished Spec":   "acme-3",
		"Long-lived Spec":   "acme-1",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.work(t, store.Draft) // Long-lived acme-1, finished: acme-2 Done
			f.move(t, "2", store.Done)
			f.spec(t, "acme", "Signup") // acme-3, unfinished: acme-4 In Review
			f.ticket(t, "acme", core.CreateTicketInput{Spec: "3", Title: "Form"})
			f.move(t, "4", store.InReview)
			_, err := f.core.SubmitRetro(ctx, "acme", core.SubmitRetroInput{Target: target})
			wantCode(t, err, core.Invalid, "target")
			if specs, _ := f.core.FindSpecs(ctx, "acme", core.FindSpecsInput{Finished: true}); len(specs) != 2 {
				t.Fatalf("a refused Retro wrote a Spec: %+v", specs)
			}
		})
	}
}

func TestTicketAndSpecReadsListTheirRetrosOldestFirstAndLeaveThemOutWhenThereAreNone(t *testing.T) {
	f := newFixture(t)
	f.finishedSpec(t)
	tk, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "acme-2"})
	if err != nil || tk.Retros != nil {
		t.Fatalf("Ticket without Retros: %+v, %v", tk.Retros, err)
	}
	sp, err := f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil || sp.Retros != nil {
		t.Fatalf("Spec without Retros: %+v, %v", sp.Retros, err)
	}
	if b, _ := json.Marshal(tk); strings.Contains(string(b), "retros") {
		t.Fatalf("retros not left out: %s", b)
	}

	first := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-2"})
	second := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-2"})
	onSpec := f.submitRetro(t, core.SubmitRetroInput{Target: "acme-1"})
	f.move(t, core.Ref(second.ID), store.Done)

	tk, err = f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "acme-2"})
	if err != nil {
		t.Fatal(err)
	}
	want := []core.RetroView{
		{ID: first.ID, Title: "Retro: Grill it", State: store.Ready},
		{ID: second.ID, Title: "Retro: Grill it", State: store.Done},
	}
	if !slices.Equal(tk.Retros, want) {
		t.Fatalf("Ticket Retros %+v, want %+v", tk.Retros, want)
	}
	sp, err = f.core.GetSpec(ctx, "acme", core.GetSpecInput{Spec: "acme-1"})
	if err != nil {
		t.Fatal(err)
	}
	if wantSpec := []core.RetroView{{ID: onSpec.ID, Title: "Retro: Login", State: store.Ready}}; !slices.Equal(sp.Retros, wantSpec) {
		t.Fatalf("Spec Retros %+v, want %+v", sp.Retros, wantSpec)
	}
	other, err := f.core.GetTicket(ctx, "acme", core.GetTicketInput{Ticket: "acme-4"})
	if err != nil || other.Retros != nil {
		t.Fatalf("another Ticket's Retros: %+v, %v", other.Retros, err)
	}
}
