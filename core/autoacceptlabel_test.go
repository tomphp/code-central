package core_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// The auto-accept Label (Spec code-central-536): the human's reserved
// Label that turns every Auto-accept kind on for a Ticket or its Spec,
// and accepts what it covers that is already In Review.

// addLabels adds labels to target as the human.
func (f fixture) addLabels(t *testing.T, target api.Ref, labels ...string) api.TargetView {
	t.Helper()
	v, err := f.core.AddLabels(human, "acme", api.LabelsInput{Target: target, Labels: labels})
	if err != nil {
		t.Fatalf("add_labels %s %v: %v", target, labels, err)
	}
	return v
}

func TestTheLabelOnATicketAutoAcceptsItsFinishWithEveryKindOff(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.addLabels(t, "acme-2", domain.AutoAcceptLabel)
	f.finishWork(t, "acme-2")
	autoAccepted(t, f.get(t, "acme-2"), domain.Accepted, domain.CauseTicketLabel)
}

func TestTheLabelOnALongLivedSpecAutoAcceptsEachOfItsTicketsFinishes(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	f.finishWork(t, "")
	autoAccepted(t, f.get(t, "acme-2"), domain.Done, domain.CauseSpecLabel)
}

func TestTheLabelOnTheTicketIsTheCauseBeforeTheSpecsAndTheProjects(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	f.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	f.addLabels(t, "acme-2", domain.AutoAcceptLabel)
	f.finishWork(t, "acme-2")
	autoAccepted(t, f.get(t, "acme-2"), domain.Accepted, domain.CauseTicketLabel)

	g := newFixture(t)
	g.work(t, domain.Ready)
	g.setAutoAccept(t, map[domain.AutoAcceptKind]bool{domain.AutoAcceptImplementation: true})
	g.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	g.finishWork(t, "acme-2")
	autoAccepted(t, g.get(t, "acme-2"), domain.Accepted, domain.CauseSpecLabel)
}

func TestTheLabelOnARequirementAutoAcceptsEveryStageWithEveryKindOff(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.addLabels(t, "acme-2", domain.AutoAcceptLabel)

	f.finishTicket(t, "acme-3")
	autoAccepted(t, f.get(t, "acme-3"), domain.Done, domain.CauseSpecLabel)
	res := f.claim(t, api.ClaimInput{Ticket: "acme-4"})
	if _, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "acme-4", ClaimID: res.ClaimID, Summary: "Spec written.", Recommendation: domain.RecommendSplit}); err != nil {
		t.Fatal(err)
	}
	autoAccepted(t, f.get(t, "acme-4"), domain.Done, domain.CauseSpecLabel)
	f.finishTicket(t, "acme-5")
	autoAccepted(t, f.get(t, "acme-5"), domain.Done, domain.CauseSpecLabel)
}

func TestTheLabelOnASpecMergeTicketAutoAcceptsIt(t *testing.T) {
	f := specMergeWorld(t)
	f.openSpecMerge(t, "README")
	f.addLabels(t, "acme-3", domain.AutoAcceptLabel)
	f.finishTicket(t, "acme-3")
	autoAccepted(t, f.get(t, "acme-3"), domain.Done, domain.CauseTicketLabel)
}

func TestPrototypesAndMapTicketsAreNeverAutoAcceptedUnderTheLabel(t *testing.T) {
	f := newFixture(t)
	f.project(t, "acme")
	f.spec(t, "acme", "Login")
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Try", State: domain.Ready, Labels: []string{domain.PrototypeLabel}})
	f.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	f.addLabels(t, "acme-2", domain.AutoAcceptLabel)
	f.finishTicket(t, "acme-2")
	if got := f.get(t, "acme-2"); got.State != domain.InReview {
		t.Fatalf("the Prototype is %s under the Label, want In Review", got.State)
	}

	g := newFixture(t)
	g.mapWith(t, api.CreateTicketInput{Title: "Look into caching", State: domain.Ready, Labels: []string{domain.WayfinderPrefix + "research"}},
		api.CreateTicketInput{Title: "Untyped", State: domain.Ready})
	g.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	g.addLabels(t, "acme-3", domain.AutoAcceptLabel)
	for _, ref := range []api.Ref{"acme-2", "acme-3"} {
		g.finishTicket(t, ref)
		if got := g.get(t, ref); got.State != domain.InReview {
			t.Fatalf("Map Ticket %s is %s under the Label, want In Review", ref, got.State)
		}
	}
}

// lastMove is tk's latest Move, or none.
func lastMove(tk api.TicketView) api.MoveView {
	if len(tk.Moves) == 0 {
		return api.MoveView{}
	}
	return tk.Moves[len(tk.Moves)-1]
}

// sweptBy checks tk was accepted from In Review to state by the human,
// with cause, by a Label added after its finish.
func sweptBy(t *testing.T, tk api.TicketView, to domain.State, cause domain.AutoAcceptCause) {
	t.Helper()
	if m := lastMove(tk); tk.State != to || m.From != domain.InReview || m.To != to || m.Actor != "human" || m.Cause != cause {
		t.Fatalf("%s is %s with last move %+v, want accepted to %s by human with cause %q", tk.ID, tk.State, m, to, cause)
	}
}

func TestAddingTheLabelToATicketInReviewAcceptsItInTheSameWrite(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.finishWork(t, "acme-2")
	f.addLabels(t, "acme-2", domain.AutoAcceptLabel)
	sweptBy(t, f.get(t, "acme-2"), domain.Accepted, domain.CauseTicketLabel)
	if a, _ := f.core.AttentionOf(ctx, "acme", "acme-2"); a.NeedsMe {
		t.Fatal("a swept Ticket is still in Needs Me")
	}
}

func TestAddingTheLabelToASpecAcceptsEveryEligibleTicketInReviewOnIt(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready, domain.Ready)
	f.finishTicket(t, "acme-2")
	f.finishTicket(t, "acme-3")
	f.ticket(t, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Try", State: domain.Ready, Labels: []string{domain.PrototypeLabel}})
	f.finishTicket(t, "acme-6") // a Prototype, never auto-accepted
	f.claim(t, api.ClaimInput{Ticket: "acme-4"})
	// acme-5 stays Ready.

	f.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	sweptBy(t, f.get(t, "acme-2"), domain.Done, domain.CauseSpecLabel)
	sweptBy(t, f.get(t, "acme-3"), domain.Done, domain.CauseSpecLabel)
	for ref, want := range map[api.Ref]domain.State{"acme-4": domain.InProgress, "acme-5": domain.Ready, "acme-6": domain.InReview} {
		if got := f.get(t, ref); got.State != want || lastMove(got).Cause != "" {
			t.Errorf("%s is %s with last move %+v, want %s untouched", ref, got.State, lastMove(got), want)
		}
	}
}

func TestTheSweepFollowsASpecStagesRecommendationAndLeavesOneWithoutIt(t *testing.T) {
	f := newFixture(t)
	f.requirement(t)
	f.finishSpecStage(t, domain.RecommendImplement)
	f.addLabels(t, "acme-2", domain.AutoAcceptLabel)
	spec := f.get(t, "acme-4")
	sweptBy(t, spec, domain.Done, domain.CauseSpecLabel)
	if lastMove(spec).Via != "implement" {
		t.Fatalf("the sweep's move %+v, want via implement", lastMove(spec))
	}
	if build := f.get(t, "acme-6"); build.Title != "Implement: Add a dark mode" || build.State != domain.Ready {
		t.Fatalf("implementation Ticket %+v", build)
	}

	// A spec Stage finished before Spec recommendations has none, so the
	// sweep leaves it for the human.
	g := newFixture(t)
	g.specInReview(t)
	g.addLabels(t, "acme-2", domain.AutoAcceptLabel)
	if got := g.get(t, "acme-4"); got.State != domain.InReview {
		t.Fatalf("a spec Stage with no recommendation is %s after the sweep, want In Review", got.State)
	}
}

func TestAddingTheLabelAgainOrAnotherLabelSweepsNothing(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.finishWork(t, "acme-2")
	f.addLabels(t, "acme-2", "ui")
	if got := f.get(t, "acme-2"); got.State != domain.InReview {
		t.Fatalf("another Label moved the Ticket to %s", got.State)
	}
}

func TestRemovingTheLabelChangesNothingAlreadyAccepted(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	f.addLabels(t, "acme-1", domain.AutoAcceptLabel)
	f.finishWork(t, "acme-2")
	if _, err := f.core.RemoveLabels(human, "acme", api.LabelsInput{Target: "acme-1", Labels: []string{domain.AutoAcceptLabel}}); err != nil {
		t.Fatalf("remove_labels: %v", err)
	}
	autoAccepted(t, f.get(t, "acme-2"), domain.Accepted, domain.CauseSpecLabel)
	f.finishTicket(t, "acme-3")
	if got := f.get(t, "acme-3"); got.State != domain.InReview {
		t.Fatalf("a finish after the Label was removed is %s, want In Review", got.State)
	}
}

// reservedForTheHuman checks err refuses the auto-accept Label as the
// human's.
func reservedForTheHuman(t *testing.T, door string, err error) {
	t.Helper()
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.Invalid || !strings.Contains(e.Message, `"auto-accept"`) || !strings.Contains(e.Message, "reserved for the human") {
		t.Fatalf("%s naming the Label: %v, want invalid naming it as reserved for the human", door, err)
	}
}

func TestOnlyTheHumanNamesTheLabelOnAnyDoor(t *testing.T) {
	for _, actor := range []api.Actor{api.Agent, api.Integration} {
		as := api.WithActor(ctx, actor)
		f := newFixture(t)
		f.work(t, domain.Ready)
		f.finishWork(t, "acme-2")
		label := []string{"ui", domain.AutoAcceptLabel}

		for _, target := range []api.Ref{"acme-1", "acme-2"} {
			_, err := f.core.AddLabels(as, "acme", api.LabelsInput{Target: target, Labels: label})
			reservedForTheHuman(t, string(actor)+" add_labels on "+string(target), err)
		}
		if got := f.get(t, "acme-2"); got.State != domain.InReview || len(got.Labels) != 0 {
			t.Fatalf("a refused add_labels changed the Ticket: %s %v", got.State, got.Labels)
		}

		f.addLabels(t, "acme-2", domain.AutoAcceptLabel)
		_, err := f.core.RemoveLabels(as, "acme", api.LabelsInput{Target: "acme-2", Labels: label})
		reservedForTheHuman(t, string(actor)+" remove_labels", err)

		_, err = f.core.CreateTicket(as, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Mine", Labels: label})
		reservedForTheHuman(t, string(actor)+" create_ticket", err)

		_, err = f.core.Import(as, "acme", api.ImportInput{Payload: api.ImportPayload{
			Specs: []api.ImportSpec{{Source: ".scratch/a/spec.md", Title: "A", Labels: []string{domain.AutoAcceptLabel}}}}})
		reservedForTheHuman(t, string(actor)+" import of a Spec", err)
		_, err = f.core.Import(as, "acme", api.ImportInput{DryRun: true, Payload: api.ImportPayload{
			Specs:   []api.ImportSpec{{Source: ".scratch/a/spec.md", Title: "A"}},
			Tickets: []api.ImportTicket{{Source: ".scratch/a/1.md", Spec: ".scratch/a/spec.md", Title: "One", Labels: []string{domain.AutoAcceptLabel}}}}})
		reservedForTheHuman(t, string(actor)+" import of a Ticket", err)
	}
}

func TestTheHumanGivesAndTakesTheLabelOnEveryDoorThatTakesLabels(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	if v := f.addLabels(t, "acme-1", domain.AutoAcceptLabel, domain.LongLivedLabel); len(v.Labels) != 2 {
		t.Fatalf("spec labels %v", v.Labels)
	}
	// Not a type Label, so it goes beside a Stage or prototype Label.
	tk, err := f.core.CreateTicket(human, "acme", api.CreateTicketInput{Spec: "acme-1", Title: "Try", Labels: []string{domain.PrototypeLabel, domain.AutoAcceptLabel}})
	if err != nil || tk.ID != "acme-3" {
		t.Fatalf("create_ticket as the human: %+v, %v", tk, err)
	}
	if _, err := f.core.RemoveLabels(human, "acme", api.LabelsInput{Target: "acme-3", Labels: []string{domain.AutoAcceptLabel}}); err != nil {
		t.Fatalf("remove_labels as the human: %v", err)
	}
	if _, err := f.core.Import(human, "acme", api.ImportInput{DryRun: true, Payload: api.ImportPayload{
		Specs:   []api.ImportSpec{{Source: ".scratch/a/spec.md", Title: "A", Labels: []string{domain.AutoAcceptLabel}}},
		Tickets: []api.ImportTicket{{Source: ".scratch/a/1.md", Spec: ".scratch/a/spec.md", Title: "One", Labels: []string{"stage:spec", domain.AutoAcceptLabel}}}}}); err != nil {
		t.Fatalf("import as the human: %v", err)
	}
}
