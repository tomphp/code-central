package core_test

import (
	"slices"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// To merge options (code-central-1067): what the Accepted panel offers on
// a Ticket the human merges, agreeing with what send_back and
// new_fix_ticket accept.

func wantToMergeOptions(t *testing.T, v api.TicketView, want ...api.ToMergeOption) {
	t.Helper()
	if v.ToMergeOptions == nil || !slices.Equal(v.ToMergeOptions, want) {
		t.Fatalf("%s to_merge_options %#v, want %v", v.ID, v.ToMergeOptions, want)
	}
}

func TestToMergeUnderBranchOffersSendBack(t *testing.T) {
	f := landWorld(t, false)
	f.landing(t, domain.Landing{Method: domain.LandingBranch})
	f.moveTo(t, 5, domain.Accepted)

	v := f.getTicket(t, "acme-5")
	wantReason(t, v, api.ReasonToMerge)
	wantToMergeOptions(t, v, "send_back")
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "acme-5", Note: "rename it"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
	if v := f.getTicket(t, "acme-5"); v.ToMergeOptions != nil {
		t.Fatalf("to_merge_options %v on a Ready Ticket, want left out", v.ToMergeOptions)
	}
}

func TestToMergeUnderPullRequestWithoutLandPerSpecOffersSendBack(t *testing.T) {
	f := newFixture(t)
	f.pullRequestAccepted(t)
	f.reportPR(t, api.ReportPullRequestInput{Branch: "acme-2", State: domain.PROpen})

	v := f.getTicket(t, "acme-2")
	wantReason(t, v, api.ReasonToMerge)
	wantToMergeOptions(t, v, "send_back")
	if _, err := f.core.NewFixTicket(human, "acme", api.NewFixTicketInput{Ticket: "acme-2", Note: "x"}); err == nil {
		t.Fatal("new_fix_ticket on a single Ticket's To merge succeeded")
	}
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "acme-2", Note: "rename it"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
}

func TestBranchAndPushOffersSendBackWhileItsCIItemListsItAlone(t *testing.T) {
	f := itemWorld(t)
	f.landing(t, domain.Landing{Method: domain.LandingBranch, Push: true, Remote: "origin"})
	f.moveTo(t, 8, domain.Done)
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)

	v := f.getTicket(t, "acme-5")
	if v.CIItem == nil || !slices.Equal(listed(v.CIItem), []string{"acme-5"}) {
		t.Fatalf("item %+v, want one listing acme-5 alone", v.CIItem)
	}
	wantToMergeOptions(t, v, "send_back")
	if _, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "acme-5", Note: "fix ci.yml"}); err != nil {
		t.Fatalf("send_back: %v", err)
	}
}

func TestBranchAndPushOffersNothingWhileItsCIItemListsOtherTickets(t *testing.T) {
	f := itemWorld(t)
	f.landing(t, domain.Landing{Method: domain.LandingBranch, Push: true, Remote: "origin"})
	f.ciRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICompleted, domain.CIFailure)

	for _, id := range []string{"acme-5", "acme-8"} {
		v := f.getTicket(t, id)
		if v.CIItem == nil || len(v.CIItem.Tickets) != 2 {
			t.Fatalf("%s item %+v, want one listing both", id, v.CIItem)
		}
		wantToMergeOptions(t, v)
		_, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: api.Ref(id), Note: "fix ci.yml"})
		notAllowed(t, err)
	}
}

func TestASpecsLeadUnderPullRequestOffersNewFixTicketNotSendBack(t *testing.T) {
	for _, state := range []domain.PullRequestState{domain.PROpen, domain.PRClosed} {
		t.Run(string(state), func(t *testing.T) {
			f := specPullRequestWorld(t)
			f.acceptedOnSpecBranch(t, 5, "one")
			f.acceptedOnSpecBranch(t, 6, "two")
			f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")
			f.reportPR(t, api.ReportPullRequestInput{Branch: specBranch, State: state})

			wantToMergeOptions(t, f.getTicket(t, "acme-5"), "new_fix_ticket")
			_, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "acme-5", Note: "x"})
			notAllowed(t, err)
			if _, err := f.core.NewFixTicket(human, "acme", api.NewFixTicketInput{Ticket: "acme-5", Note: "reviewers asked for a rename"}); err != nil {
				t.Fatalf("new_fix_ticket: %v", err)
			}
		})
	}
}

func TestASpecWaitingTicketThatIsNotItsLeadOffersNothing(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "one")
	f.acceptedOnSpecBranch(t, 6, "two")
	f.pushSpecBranch(t, "tip1", "acme-5", "acme-6")
	f.reportPR(t, api.ReportPullRequestInput{Branch: specBranch, State: domain.PROpen})

	wantToMergeOptions(t, f.getTicket(t, "acme-6"))
	_, err := f.core.SendBack(human, "acme", api.SendBackInput{Ticket: "acme-6", Note: "x"})
	notAllowed(t, err)
	if _, err := f.core.NewFixTicket(human, "acme", api.NewFixTicketInput{Ticket: "acme-6", Note: "x"}); err == nil {
		t.Fatal("new_fix_ticket on a Spec's non-lead succeeded")
	}
}

func TestASpecWaitingTicketOffersNothingBeforeItsSpecBranchIsPushed(t *testing.T) {
	f := specPullRequestWorld(t)
	f.acceptedOnSpecBranch(t, 5, "one")

	v := f.getTicket(t, "acme-5")
	if !v.WaitingForSpec || v.NeedsMeReason != "" {
		t.Fatalf("waiting %v, reason %q: want acme-5 waiting on its Spec with no item", v.WaitingForSpec, v.NeedsMeReason)
	}
	wantToMergeOptions(t, v)
}

func TestToMergeOptionsAreLeftOutUnderMerge(t *testing.T) {
	f := landWorld(t, false)
	f.moveTo(t, 5, domain.Accepted)
	if v := f.getTicket(t, "acme-5"); v.ToMergeOptions != nil {
		t.Fatalf("to_merge_options %v under Merge, want left out", v.ToMergeOptions)
	}
}
