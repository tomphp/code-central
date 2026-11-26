package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Stop and Resume of a Factory Claim (Spec code-central-1233).

var byFactory = api.ClaimInput{SessionID: "s1", Factory: "cc", Host: "box"}

func (f fixture) stop(t *testing.T, ref api.Ref) api.TicketView {
	t.Helper()
	v, err := record(f.core.Stop(human, "acme", api.StopInput{Ticket: ref, Full: true}))
	if err != nil {
		t.Fatalf("stop %s: %v", ref, err)
	}
	return v
}

func (f fixture) resume(t *testing.T, ref api.Ref) api.TicketView {
	t.Helper()
	v, err := record(f.core.Resume(human, "acme", api.ResumeInput{Ticket: ref, Full: true}))
	if err != nil {
		t.Fatalf("resume %s: %v", ref, err)
	}
	return v
}

// factoryAnswered is acme-2 Answered on a Factory Claim.
func (f fixture) factoryAnswered(t *testing.T) {
	t.Helper()
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	if _, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []api.QuestionInput{qDB}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
}

func wantRefused(t *testing.T, err error, why string) {
	t.Helper()
	wantCode(t, err, api.ActionNotAllowed, "")
	if !strings.Contains(err.Error(), why) {
		t.Fatalf("error %q, want it to say %q", err, why)
	}
}

func TestStopMarksTheFactoryClaimStoppedAndKeepsItLive(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.clock.Advance(time.Hour)
	stopped := f.clock.Now()

	v, err := record(f.core.Stop(human, "acme", api.StopInput{Ticket: "2", Note: "Wrong approach.", Full: true}))
	if err != nil {
		t.Fatal(err)
	}
	c := v.Claims[0]
	if v.State != domain.InProgress || c.Ended != nil || !c.Stopped || !c.FactoryClaim {
		t.Fatalf("state %s claim %+v, want In Progress with a live, Stopped Factory Claim", v.State, c)
	}
	if len(c.Stops) != 1 || !c.Stops[0].Stopped.Equal(stopped) || c.Stops[0].Resumed != nil {
		t.Fatalf("stops %+v, want one at %v not Resumed", c.Stops, stopped)
	}
	want := api.CommentView{Number: 1, Actor: "human", Time: stopped, Claim: 1, Body: "Wrong approach."}
	if len(v.Comments) != 1 || v.Comments[0] != want {
		t.Fatalf("comments %+v, want [%+v]", v.Comments, want)
	}
	if v.Stoppable || !v.Resumable {
		t.Fatalf("stoppable %v resumable %v, want only resumable", v.Stoppable, v.Resumable)
	}
}

func TestResumeEndsTheLatestStopAndTheClaimCanBeStoppedAgain(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.stop(t, "2")
	f.clock.Advance(time.Hour)
	resumed := f.clock.Now()

	v := f.resume(t, "2")
	c := v.Claims[0]
	if c.Stopped || len(c.Stops) != 1 || c.Stops[0].Resumed == nil || !c.Stops[0].Resumed.Equal(resumed) {
		t.Fatalf("claim %+v, want its one Stop Resumed at %v", c, resumed)
	}
	if !v.Stoppable || v.Resumable {
		t.Fatalf("stoppable %v resumable %v, want only stoppable", v.Stoppable, v.Resumable)
	}
	if c := f.stop(t, "2").Claims[0]; !c.Stopped || len(c.Stops) != 2 {
		t.Fatalf("claim %+v, want Stopped with two Stops", c)
	}
}

func TestAnAnsweredFactoryClaimCanBeStopped(t *testing.T) {
	f := newFixture(t)
	f.factoryAnswered(t)
	if v := f.get(t, "2"); !v.Stoppable {
		t.Fatalf("%s Answered: stoppable false, want true", v.ID)
	}
	if v := f.stop(t, "2"); v.State != domain.Answered || !v.Claims[0].Stopped {
		t.Fatalf("state %s claim %+v, want Answered and Stopped", v.State, v.Claims[0])
	}
}

func TestStopIsRefusedWithWhy(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready, domain.Ready, domain.Ready)
	f.claim(t, api.ClaimInput{Ticket: "2"})
	f.claim(t, api.ClaimInput{Ticket: "3", Factory: "cc", Host: "box"})
	if _, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "3", ClaimID: "claim-2", Questions: []api.QuestionInput{qDB}}); err != nil {
		t.Fatal(err)
	}
	f.claim(t, api.ClaimInput{Ticket: "4", Factory: "cc", Host: "box"})
	f.stop(t, "4")

	for _, tc := range []struct {
		ref api.Ref
		why string
	}{
		{"2", "acme-2's Claim was not made by a Factory"},
		{"3", "acme-3 is Needs Answer: answer its Round instead"},
		{"4", "acme-4's Claim is already Stopped: resume or release it"},
		{"5", "acme-5 is Ready"},
	} {
		_, err := f.core.Stop(human, "acme", api.StopInput{Ticket: tc.ref})
		wantRefused(t, err, tc.why)
		if v := f.get(t, tc.ref); v.Stoppable {
			t.Errorf("%s stoppable, want not", v.ID)
		}
	}
}

func TestResumeIsRefusedUnlessTheClaimIsStopped(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready, domain.Ready)
	f.claim(t, api.ClaimInput{Ticket: "2", Factory: "cc", Host: "box"})
	_, err := f.core.Resume(human, "acme", api.ResumeInput{Ticket: "2"})
	wantRefused(t, err, "acme-2's Claim is not Stopped")
	_, err = f.core.Resume(human, "acme", api.ResumeInput{Ticket: "3"})
	wantRefused(t, err, "acme-3 has no live Claim")
	if f.get(t, "2").Resumable || f.get(t, "3").Resumable {
		t.Fatal("resumable without a Stopped Claim, want not")
	}
}

func TestStopAndResumeAreTheHumans(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	_, err := f.core.Stop(ctx, "acme", api.StopInput{Ticket: "2"})
	wantRefused(t, err, "stop is a human action")
	f.stop(t, "2")
	_, err = f.core.Resume(ctx, "acme", api.ResumeInput{Ticket: "2"})
	wantRefused(t, err, "resume is a human action")
}

func TestTheHolderWritesOnAStoppedClaimAndItStaysStoppedThroughItsRound(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.stop(t, "2")

	if _, err := f.core.SetSession(ctx, "acme", api.SetSessionInput{Ticket: "2", ClaimID: "claim-1", SessionID: "by-hand"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.PostRound(ctx, "acme", api.PostRoundInput{Ticket: "2", ClaimID: "claim-1", Questions: []api.QuestionInput{qDB}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Answer(human, "acme", api.AnswerInput{Ticket: "2", Question: 1, Text: "SQLite"}); err != nil {
		t.Fatal(err)
	}
	if v := f.get(t, "2"); v.State != domain.Answered || !v.Claims[0].Stopped || !v.Resumable {
		t.Fatalf("state %s claim %+v, want Answered and still Stopped", v.State, v.Claims[0])
	}
	if _, err := f.core.PickUpAnswers(ctx, "acme", api.PickUpAnswersInput{Ticket: "2", ClaimID: "claim-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.core.Finish(ctx, "acme", api.FinishInput{Ticket: "2", ClaimID: "claim-1", Summary: "Done by hand."}); err != nil {
		t.Fatal(err)
	}
	if v := f.get(t, "2"); v.State != domain.InReview || v.Claims[0].Stopped || v.Resumable {
		t.Fatalf("state %s claim %+v, want In Review with the Stopped mark ended", v.State, v.Claims[0])
	}
}

func TestReleaseCancelAndAMoveByHandEndAStoppedClaim(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(f fixture) error
	}{
		{"release", func(f fixture) error {
			_, err := f.core.Release(human, "acme", api.ReleaseInput{Ticket: "2"})
			return err
		}},
		{"cancel", func(f fixture) error {
			_, err := f.core.Cancel(human, "acme", api.CancelInput{Ticket: "2"})
			return err
		}},
		{"a move by hand", func(f fixture) error {
			_, err := f.core.Move(human, "acme", api.MoveInput{Ticket: "2", ToState: domain.Draft})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.work(t, domain.Ready)
			f.claim(t, byFactory)
			f.stop(t, "2")
			if err := tc.end(f); err != nil {
				t.Fatal(err)
			}
			v := f.get(t, "2")
			if c := v.Claims[0]; c.Ended == nil || c.Stopped || len(c.Stops) != 1 || v.Resumable || v.Stoppable {
				t.Fatalf("claim %+v, want it ended, its Stop kept and nothing to resume", c)
			}
		})
	}
}
