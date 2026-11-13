package core_test

import (
	"testing"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// specClaimed is requirement with its spec Stage, acme-4, claimed as
// claim-1.
func (f fixture) specClaimed(t *testing.T) {
	t.Helper()
	f.requirement(t)
	f.setState(t, 3, store.Done)
	f.setState(t, 4, store.Ready)
	f.claim(t, core.ClaimInput{Ticket: "acme-4"})
}

func TestFinishingASpecStageStoresItsRecommendation(t *testing.T) {
	f := newFixture(t)
	f.specClaimed(t)
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-4", ClaimID: "claim-1", Summary: "Small Spec: implement.", Recommendation: "implement"}); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, "acme-4"); got.State != store.InReview || got.Recommendation != "implement" {
		t.Fatalf("spec Stage %s with recommendation %q, want In Review with implement", got.State, got.Recommendation)
	}
}

func TestALaterFinishReplacesTheRecommendation(t *testing.T) {
	f := newFixture(t)
	f.specClaimed(t)
	f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-4", ClaimID: "claim-1", Summary: "Small.", Recommendation: "implement"})
	if _, err := f.core.Reject(human, "acme", core.RejectInput{Ticket: "acme-4", Comment: "It is bigger than that."}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	f.claim(t, core.ClaimInput{Ticket: "acme-4"})
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-4", ClaimID: "claim-2", Summary: "Large: split.", Recommendation: "split"}); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, "acme-4").Recommendation; got != "split" {
		t.Fatalf("recommendation %q, want split", got)
	}
}

func TestASpecStageCannotFinishWithoutAValidRecommendation(t *testing.T) {
	for _, rec := range []store.Recommendation{"", "  ", "maybe"} {
		f := newFixture(t)
		f.specClaimed(t)
		_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-4", ClaimID: "claim-1", Summary: "Done.", Recommendation: rec})
		wantCode(t, err, core.Invalid, "recommendation")
		if got := f.get(t, "acme-4"); got.State != store.InProgress || got.Recommendation != "" {
			t.Fatalf("recommendation %q: spec Stage %s with %q, want still In Progress with none", rec, got.State, got.Recommendation)
		}
	}
}

func TestOnlyASpecStageTakesARecommendation(t *testing.T) {
	f := newFixture(t)
	f.work(t, store.Ready)
	f.claim(t, core.ClaimInput{})
	_, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-2", ClaimID: "claim-1", Summary: "Done.", Recommendation: "split"})
	wantCode(t, err, core.Invalid, "recommendation")
	if got := f.get(t, "acme-2"); got.State != store.InProgress || got.Recommendation != "" {
		t.Fatalf("Ticket %s with %q, want still In Progress with no recommendation", got.State, got.Recommendation)
	}
	// Without one, it finishes and reads no recommendation.
	if _, err := f.core.Finish(ctx, "acme", core.FinishInput{Ticket: "acme-2", ClaimID: "claim-1", Summary: "Done."}); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, "acme-2").Recommendation; got != "" {
		t.Fatalf("recommendation %q, want none", got)
	}
}
