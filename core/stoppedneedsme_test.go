package core_test

import (
	"slices"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// Needs Me, Silent and Long Claims and the Stage strip for a Stopped Claim
// (Spec code-central-1233).

func TestAStoppedClaimNeedsMeSinceItsStop(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.clock.Advance(5 * time.Minute)
	stopped := f.clock.Now()
	f.stop(t, "2")

	wantReason(t, f.get(t, "2"), api.ReasonStopped)
	row, ok := longRow(t, f, "acme-2")
	if !ok || row.Reason != api.ReasonStopped || !row.Since.Equal(stopped) {
		t.Fatalf("Needs Me row %+v, want stopped since %s", row, stopped)
	}
	v, _ := f.core.NeedsMe(human)
	if v.Count != 1 {
		t.Fatalf("Needs Me count %d, want 1", v.Count)
	}
}

func TestStoppedWinsOverAnsweredAndEndsWithTheResume(t *testing.T) {
	f := newFixture(t)
	f.factoryAnswered(t)
	wantReason(t, f.get(t, "2"), api.ReasonAnswered)

	f.stop(t, "2")
	wantReason(t, f.get(t, "2"), api.ReasonStopped)

	f.resume(t, "2")
	wantReason(t, f.get(t, "2"), api.ReasonAnswered)
}

func TestStoppedSitsBetweenAnsweredAndSilentClaim(t *testing.T) {
	var order []api.NeedsMeReason
	for _, v := range api.NeedsMeReasonViews {
		order = append(order, v.Reason)
	}
	answered, stopped, silent := slices.Index(order, api.ReasonAnswered), slices.Index(order, api.ReasonStopped), slices.Index(order, api.ReasonSilentClaim)
	if answered+1 != stopped || stopped+1 != silent {
		t.Fatalf("order %v, want stopped right after answered and before silent_claim", order)
	}
}

func TestAStoppedClaimIsNeverSilentOrLong(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.stop(t, "2")

	f.clock.Advance(core.LongAfter + core.SilentAfter)
	wantReason(t, f.get(t, "2"), api.ReasonStopped)
}

func TestQuietTimeCountsFromTheLatestResume(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.clock.Advance(core.LongAfter + core.SilentAfter)
	f.stop(t, "2")
	f.clock.Advance(time.Minute)
	f.resume(t, "2")
	wantReason(t, f.get(t, "2"), "")

	f.clock.Advance(core.SilentAfter)
	wantReason(t, f.get(t, "2"), api.ReasonSilentClaim)
}

func TestLongTimeCountsFromTheLatestResume(t *testing.T) {
	f := newFixture(t)
	f.work(t, domain.Ready)
	f.claim(t, byFactory)
	f.keepActive(core.LongAfter)
	f.stop(t, "2")
	f.resume(t, "2")

	f.keepActive(core.LongAfter)
	wantReason(t, f.get(t, "2"), "")
	f.clock.Advance(time.Second)
	wantReason(t, f.get(t, "2"), api.ReasonLongClaim)
}

func TestAStageWithAStoppedClaimNeedsTheHuman(t *testing.T) {
	f := newFixture(t)
	f.stagesDone(t, domain.Ready)
	f.claim(t, api.ClaimInput{Ticket: "acme-6", SessionID: "s1", Factory: "cc", Host: "box"})
	f.stop(t, "acme-6")

	r := f.stripOf(t, "2")
	if got, want := needLines(r), []string{"acme-6 stopped"}; !slices.Equal(got, want) {
		t.Fatalf("needs %v, want %v", got, want)
	}
	if got := segmentLines(r)[3]; got != "build needs stopped" {
		t.Fatalf("Build segment %q, want %q", got, "build needs stopped")
	}
}
