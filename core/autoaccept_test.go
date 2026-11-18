package core_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
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
func (f fixture) setAutoAccept(t *testing.T, kinds map[domain.AutoAcceptKind]bool) core.ProjectView {
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

	p := f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{"implementation": true, "merge": true})
	if !p.AutoAccept["implementation"] || !p.AutoAccept["merge"] || p.AutoAccept["grilling"] || p.AutoAccept["spec"] || p.AutoAccept["tickets"] {
		t.Fatalf("auto_accept = %v, want implementation and merge on", p.AutoAccept)
	}

	// A kind left out keeps its value.
	p = f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{"merge": false, "grilling": true})
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
	v, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[domain.AutoAcceptKind]bool{"tickets": true}})
	if err != nil || v.ID != "acme" || len(v.Changed) != 1 || v.Changed[0] != "auto_accept" {
		t.Fatalf("acknowledgement %+v, %v", v, err)
	}
	v, err = f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[domain.AutoAcceptKind]bool{"tickets": true}})
	if err != nil || len(v.Changed) != 0 {
		t.Fatalf("acknowledgement of no change %+v, %v", v, err)
	}
}

func TestSetAutoAcceptIsTheHumansAloneAndRefusesAnUnknownKind(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	for _, actor := range []core.Actor{core.Agent, core.Integration} {
		_, err := f.core.SetAutoAccept(core.WithActor(ctx, actor), "acme", core.SetAutoAcceptInput{Kinds: map[domain.AutoAcceptKind]bool{"implementation": true}})
		var e *core.Error
		if !errors.As(err, &e) || e.Code != core.ActionNotAllowed {
			t.Fatalf("as %s: %v, want action_not_allowed", actor, err)
		}
	}
	_, err := f.core.SetAutoAccept(human, "acme", core.SetAutoAcceptInput{Kinds: map[domain.AutoAcceptKind]bool{"prototype": true}})
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
func autoAccepted(t *testing.T, tk core.TicketView, to domain.State, cause domain.AutoAcceptCause) {
	t.Helper()
	if tk.State != to {
		t.Fatalf("%s is %s, want %s", tk.ID, tk.State, to)
	}
	n := len(tk.Moves)
	if n < 2 {
		t.Fatalf("moves %+v", tk.Moves)
	}
	if m := tk.Moves[n-2]; m.To != domain.InReview || m.Actor != "agent" || m.Cause != "" {
		t.Fatalf("finish's move %+v, want to In Review by the agent with no cause", m)
	}
	if m := tk.Moves[n-1]; m.From != domain.InReview || m.To != to || m.Actor != "human" || m.Cause != cause {
		t.Fatalf("accept's move %+v, want In Review to %s by human with cause %q", m, to, cause)
	}
}

func TestFinishAutoAcceptsAnImplementationTicketUnderTheProjectSetting(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	res := f.finishWork(t, "acme-2")

	autoAccepted(t, f.get(t, "acme-2"), domain.Accepted, domain.CauseProject)
	// finish's response is the agent's, as if nothing followed.
	if res.Acknowledgement == nil || res.Acknowledgement.State != domain.InReview || res.NextStep != "stop working this Ticket" {
		t.Fatalf("finish result %+v, want the In Review acknowledgement", res)
	}
	if a, _ := f.core.AttentionOf(ctx, "acme", "acme-2"); a.NeedsMe {
		t.Fatalf("an auto-accepted Ticket is in Needs Me")
	}
}

func TestFinishAutoAcceptsToDoneWhenNoBranchWasNamed(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	f.finishWork(t, "")
	autoAccepted(t, f.get(t, "acme-2"), domain.Done, domain.CauseProject)
}

func TestFinishLeavesTheTicketInReviewWhileItsKindIsOff(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptMerge: true, domain.AutoAcceptGrilling: true, domain.AutoAcceptSpec: true, domain.AutoAcceptTickets: true})
	res := f.finishWork(t, "acme-2")
	tk := f.get(t, "acme-2")
	if tk.State != domain.InReview || tk.Moves[len(tk.Moves)-1].Cause != "" {
		t.Fatalf("ticket %s with moves %+v, want In Review", tk.State, tk.Moves)
	}
	if res.Acknowledgement == nil || res.Acknowledgement.State != domain.InReview {
		t.Fatalf("finish result %+v", res)
	}
}

func TestFinishsResponseIsTheSameWhetherOrNotItAutoAccepts(t *testing.T) {
	var results []core.FinishResult
	var fulls []core.FinishResult
	for _, on := range []bool{false, true} {
		f := newFixture(t)
		f.work(t, domain.Ready)
		f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: on})
		f.clock.Set(t0.Add(time.Hour))
		results = append(results, f.finishWork(t, "acme-2"))

		g := newFixture(t)
		g.work(t, domain.Ready)
		g.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: on})
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

// finishSpecStage claims the spec Stage acme-4 and finishes it with the
// recommendation.
func (f fixture) finishSpecStage(t *testing.T, rec domain.Recommendation) core.FinishResult {
	t.Helper()
	f.setState(t, 3, domain.Done)
	f.setState(t, 4, domain.Ready)
	f.claim(t, core.ClaimInput{Ticket: "acme-4"})
	res, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-4", ClaimID: "claim-1", Summary: "Spec written.", Recommendation: rec})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return res
}

func TestFinishAutoAcceptsASpecStageRecommendingImplementAsImplement(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptSpec: true})
	res := f.finishSpecStage(t, domain.RecommendImplement)
	if res.Acknowledgement == nil || res.Acknowledgement.State != domain.InReview || res.NextStep != "stop working this Ticket" {
		t.Fatalf("finish result %+v, want finish's own", res)
	}

	spec := f.get(t, "acme-4")
	autoAccepted(t, spec, domain.Done, domain.CauseProject)
	if m := spec.Moves[len(spec.Moves)-1]; m.Via != "implement" {
		t.Fatalf("accept's move %+v, want via implement", m)
	}
	build := f.get(t, "acme-6")
	if build.Title != "Implement: Add a dark mode" || build.State != domain.Ready || build.Spec.ID != "acme-2" {
		t.Fatalf("implementation Ticket %+v", build)
	}
	if len(build.Moves) != 1 || build.Moves[0].To != domain.Ready || build.Moves[0].Actor != "human" || build.Moves[0].Via != "implement" {
		t.Fatalf("implementation Ticket moves %+v", build.Moves)
	}
	if tickets := f.get(t, "acme-5"); tickets.State != domain.Cancelled || len(tickets.Replacements) != 1 || tickets.Replacements[0] != "acme-6" {
		t.Fatalf("tickets Stage %+v", tickets)
	}
}

func TestFinishAutoAcceptsASpecStageRecommendingSplitAsSplitIntoTickets(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptSpec: true})
	f.finishSpecStage(t, domain.RecommendSplit)

	spec := f.get(t, "acme-4")
	autoAccepted(t, spec, domain.Done, domain.CauseProject)
	if m := spec.Moves[len(spec.Moves)-1]; m.Via != "" {
		t.Fatalf("accept's move %+v, want no via", m)
	}
	if tickets := f.get(t, "acme-5"); tickets.State != domain.Ready {
		t.Fatalf("tickets Stage is %s, want Ready to run", tickets.State)
	}
	if _, err := f.core.GetTicket(ctx, "acme", whole("acme-6")); err == nil {
		t.Fatal("split created a Ticket")
	}
}

func TestAnAutoAcceptedImplementCreatesADraftTicketWhileTheTicketsStageIsDraft(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptSpec: true})
	f.setState(t, 5, domain.Draft)
	f.finishSpecStage(t, domain.RecommendImplement)

	autoAccepted(t, f.get(t, "acme-4"), domain.Done, domain.CauseProject)
	build := f.get(t, "acme-6")
	if build.State != domain.Draft || len(build.Moves) != 1 || build.Moves[0].To != domain.Draft || build.Moves[0].Via != "implement" {
		t.Fatalf("implementation Ticket %s with moves %+v, want Draft", build.State, build.Moves)
	}
	if tickets := f.get(t, "acme-5"); tickets.State != domain.Cancelled {
		t.Fatalf("tickets Stage is %s, want Cancelled", tickets.State)
	}
}

func TestASpecStageStaysInReviewWhenItsImplementIsNotAllowed(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptSpec: true})
	// The tickets Stage can no longer run, so implement is not allowed:
	// the finish still goes through, and the spec Stage waits for the human.
	f.setState(t, 5, domain.Cancelled)
	res := f.finishSpecStage(t, domain.RecommendImplement)
	if res.Acknowledgement == nil || res.Acknowledgement.State != domain.InReview {
		t.Fatalf("finish result %+v", res)
	}
	spec := f.get(t, "acme-4")
	if spec.State != domain.InReview || spec.Recommendation != domain.RecommendImplement || spec.Moves[len(spec.Moves)-1].Cause != "" {
		t.Fatalf("spec Stage %s with moves %+v, want In Review with no auto-accept", spec.State, spec.Moves)
	}
	if _, err := f.core.GetTicket(ctx, "acme", whole("acme-6")); err == nil {
		t.Fatal("a Ticket was created")
	}
}

func TestTheHumansImplementStillCreatesAReadyTicketWhileTheTicketsStageIsDraft(t *testing.T) {
	f := newFixture(t)
	f.specInReview(t)
	f.setState(t, 5, domain.Draft)
	f.implement(t, "acme-4")
	if build := f.get(t, "acme-6"); build.State != domain.Ready || build.Moves[0].Cause != "" {
		t.Fatalf("implementation Ticket %s with moves %+v, want Ready with no cause", build.State, build.Moves)
	}
}

func TestFinishingASpecStageStillNeedsARecommendationUnderAutoAccept(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptSpec: true})
	f.setState(t, 3, domain.Done)
	f.setState(t, 4, domain.Ready)
	f.claim(t, core.ClaimInput{Ticket: "acme-4"})
	_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-4", ClaimID: "claim-1", Summary: "Spec written."})
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid {
		t.Fatalf("finish without a recommendation: %v, want invalid", err)
	}
	if spec := f.get(t, "acme-4"); spec.State != domain.InProgress {
		t.Fatalf("spec Stage is %s, want still In Progress", spec.State)
	}
}

// allKinds turns every Auto-accept kind on.
func (f fixture) allKinds(t *testing.T) {
	t.Helper()
	kinds := map[domain.AutoAcceptKind]bool{}
	for _, k := range domain.AutoAcceptKinds {
		kinds[k] = true
	}
	f.setAutoAccept(t, kinds)
}

// finishTicket claims ref and finishes it with no branch.
func (f fixture) finishTicket(t *testing.T, ref core.Ref) {
	t.Helper()
	res := f.claim(t, core.ClaimInput{Ticket: ref})
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: ref, ClaimID: res.ClaimID, Summary: "Finished " + string(ref) + "."}); err != nil {
		t.Fatalf("finish %s: %v", ref, err)
	}
}

func TestGrillingOutcomesAutoAcceptsAFinishedGrillingStage(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptGrilling: true})
	f.finishTicket(t, "acme-3")
	autoAccepted(t, f.get(t, "acme-3"), domain.Done, domain.CauseProject)
	if spec := f.get(t, "acme-4"); spec.State != domain.Ready {
		t.Fatalf("spec Stage is %s, want Ready once the grilling Stage is Done", spec.State)
	}
}

func TestTicketsStageAutoAcceptsAFinishedTicketsStage(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptTickets: true})
	f.setState(t, 3, domain.Done)
	f.setState(t, 4, domain.Done)
	f.finishTicket(t, "acme-5")
	autoAccepted(t, f.get(t, "acme-5"), domain.Done, domain.CauseProject)
}

func TestAStageIsNotAutoAcceptedUnderTheOtherKinds(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptSpec: true, domain.AutoAcceptTickets: true, domain.AutoAcceptImplementation: true, domain.AutoAcceptMerge: true})
	f.finishTicket(t, "acme-3")
	if g := f.get(t, "acme-3"); g.State != domain.InReview {
		t.Fatalf("grilling Stage is %s, want In Review", g.State)
	}
}

func TestMergeResultsAutoAcceptsOnlyASpecMergeTicket(t *testing.T) {
	f := specMergeWorld(t)
	f.openSpecMerge(t, "README")
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptMerge: true})
	f.finishTicket(t, "acme-3")
	autoAccepted(t, f.get(t, "acme-3"), domain.Done, domain.CauseProject)

	// An ordinary Ticket is Implementation results, whatever its title.
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Merge the base into the Spec Branch of acme-1", State: domain.Ready})
	f.finishTicket(t, "acme-4")
	if got := f.get(t, "acme-4"); got.State != domain.InReview {
		t.Fatalf("a Ticket titled like a Spec merge Ticket is %s under Merge results, want In Review", got.State)
	}
	f.finishTicket(t, "acme-2")
	if got := f.get(t, "acme-2"); got.State != domain.InReview {
		t.Fatalf("an implementation Ticket is %s under Merge results, want In Review", got.State)
	}
}

func TestImplementationResultsNeverAcceptsASpecMergeTicket(t *testing.T) {
	f := specMergeWorld(t)
	f.openSpecMerge(t, "README")
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	f.finishTicket(t, "acme-3")
	if got := f.get(t, "acme-3"); got.State != domain.InReview {
		t.Fatalf("the Spec merge Ticket is %s under Implementation results, want In Review", got.State)
	}
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Merge the base into the Spec Branch of acme-1", State: domain.Ready})
	f.finishTicket(t, "acme-4")
	autoAccepted(t, f.get(t, "acme-4"), domain.Done, domain.CauseProject)
}

func TestPrototypesAndMapTicketsAreNeverAutoAccepted(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", core.CreateTicketInput{Spec: "acme-1", Title: "Try", State: domain.Ready, Labels: []string{core.PrototypeLabel}})
	f.allKinds(t)
	f.finishTicket(t, "acme-2")
	if got := f.get(t, "acme-2"); got.State != domain.InReview {
		t.Fatalf("the Prototype is %s with every kind on, want In Review", got.State)
	}

	g := newFixture(t)
	g.mapWith(t, core.CreateTicketInput{Title: "Look into caching", State: domain.Ready, Labels: []string{core.WayfinderPrefix + "research"}},
		core.CreateTicketInput{Title: "Untyped", State: domain.Ready})
	g.allKinds(t)
	for _, ref := range []core.Ref{"acme-2", "acme-3"} {
		g.finishTicket(t, ref)
		if got := g.get(t, ref); got.State != domain.InReview {
			t.Fatalf("Map Ticket %s is %s with every kind on, want In Review", ref, got.State)
		}
	}
}

func TestARedoIsAutoAcceptedLikeAnyOtherFinish(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.finishWork(t, "acme-2")
	if _, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "acme-2", Comment: "Missing tests."}); err != nil {
		t.Fatal(err)
	}
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	res := f.claim(t, core.ClaimInput{Ticket: "acme-2"})
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-2", ClaimID: res.ClaimID, Summary: "Tests added.", Branch: "acme-2"}); err != nil {
		t.Fatal(err)
	}
	autoAccepted(t, f.get(t, "acme-2"), domain.Accepted, domain.CauseProject)
}

func TestTurningAKindOnAcceptsNothingAlreadyInReview(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.finishWork(t, "acme-2")
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	if got := f.get(t, "acme-2"); got.State != domain.InReview {
		t.Fatalf("ticket is %s after turning the kind on, want still In Review", got.State)
	}
	if a, _ := f.core.AttentionOf(ctx, "acme", "acme-2"); !a.NeedsMe {
		t.Fatal("a Ticket In Review before the setting should still need me")
	}
}
